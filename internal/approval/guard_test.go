package approval

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/skryvets/my-agent/internal/conversation"
)

// countingTool records how often it really ran.
type countingTool struct {
	name string
	runs *int
	err  error
}

func (c countingTool) Name() string        { return c.name }
func (c countingTool) Description() string { return "does something" }
func (c countingTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}

func (c countingTool) Call(ctx context.Context, args json.RawMessage) (string, error) {
	*c.runs++
	return "ran", c.err
}

func TestGuardRunsAnAllowedCallWithoutAsking(t *testing.T) {
	runs, asks := 0, 0
	broker := &Broker{}
	broker.Handle(func(context.Context, string, Request) (bool, error) {
		asks++
		return true, nil
	})
	guard := Guard{
		Tool:   countingTool{name: "read_file", runs: &runs},
		Policy: Default(),
		Broker: broker,
	}

	out, err := guard.Call(context.Background(), json.RawMessage(`{"path":"a.txt"}`))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if out != "ran" || runs != 1 {
		t.Errorf("out = %q, runs = %d", out, runs)
	}
	if asks != 0 {
		t.Errorf("an allowed call asked %d times", asks)
	}
	if guard.Name() != "read_file" || guard.Description() == "" || guard.Parameters() == nil {
		t.Error("the guard does not look like the tool it wraps")
	}
}

func TestGuardAsksBeforeAGatedCall(t *testing.T) {
	runs := 0
	var seen Request
	var seenKey string
	broker := &Broker{}
	broker.Handle(func(_ context.Context, key string, request Request) (bool, error) {
		seen, seenKey = request, key
		return true, nil
	})
	guard := Guard{
		Tool:   countingTool{name: "fetch", runs: &runs},
		Policy: Default(),
		Broker: broker,
	}

	ctx := conversation.WithKey(context.Background(), "chat-1")
	if _, err := guard.Call(ctx, json.RawMessage(`{"url":"https://example.com"}`)); err != nil {
		t.Fatalf("Call: %v", err)
	}
	if runs != 1 {
		t.Errorf("an approved call ran %d times", runs)
	}
	if seen.Tool != "fetch" || seen.Details != "https://example.com" {
		t.Errorf("request = %#v", seen)
	}
	if seenKey != "chat-1" {
		t.Errorf("the question went to %q", seenKey)
	}
}

func TestGuardTellsTheModelWhenAPersonSaysNo(t *testing.T) {
	runs := 0
	broker := &Broker{}
	broker.Handle(func(context.Context, string, Request) (bool, error) { return false, nil })
	guard := Guard{
		Tool:   countingTool{name: "shell", runs: &runs},
		Policy: Default(),
		Broker: broker,
	}

	out, err := guard.Call(context.Background(), json.RawMessage(`{"command":"rm -rf /"}`))
	if err != nil {
		t.Fatalf("a refusal must not be an error: %v", err)
	}
	if runs != 0 {
		t.Error("a refused call ran")
	}
	if !strings.Contains(out, "refused") || !strings.Contains(out, "Do not repeat it") {
		t.Errorf("out = %q", out)
	}
}

func TestGuardReportsAQuestionThatCouldNotBePut(t *testing.T) {
	runs := 0
	guard := Guard{
		Tool:   countingTool{name: "shell", runs: &runs},
		Policy: Default(),
		Broker: &Broker{},
	}

	// Nobody registered a connector, so there is nobody to ask.
	if _, err := guard.Call(context.Background(), json.RawMessage(`{"command":"curl x"}`)); err == nil {
		t.Fatal("expected an error")
	}
	if runs != 0 {
		t.Error("the call ran with nobody watching")
	}
}

func TestBrokerTreatsSilenceAsNo(t *testing.T) {
	broker := &Broker{}
	broker.Handle(func(ctx context.Context, _ string, _ Request) (bool, error) {
		<-ctx.Done()
		return false, ctx.Err()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	allowed, err := broker.Allowed(ctx, Request{Tool: "fetch"})
	if err != nil {
		t.Fatalf("a question nobody answered must not be an error: %v", err)
	}
	if allowed {
		t.Error("silence was taken for a yes")
	}
}

func TestBrokerPassesOnAConnectorFailure(t *testing.T) {
	broker := &Broker{}
	broker.Handle(func(context.Context, string, Request) (bool, error) {
		return false, errors.New("telegram is down")
	})

	if _, err := broker.Allowed(context.Background(), Request{Tool: "fetch"}); err == nil {
		t.Fatal("expected the failure of the connector")
	}
}

func TestGuardedWrapsEveryTool(t *testing.T) {
	runs := 0
	guarded := Guarded(&Broker{}, Default(),
		countingTool{name: "one", runs: &runs},
		countingTool{name: "two", runs: &runs},
	)

	if len(guarded) != 2 {
		t.Fatalf("guarded = %#v", guarded)
	}
	if guarded[0].Name() != "one" || guarded[1].Name() != "two" {
		t.Errorf("names = %q %q", guarded[0].Name(), guarded[1].Name())
	}
}

func TestDetailsReadsTheInterestingArgument(t *testing.T) {
	cases := map[string]string{
		`{"command":"ls -la"}`:            "ls -la",
		`{"url":"https://example.com"}`:   "https://example.com",
		`{"path":"a.txt","content":"hi"}`: "a.txt",
		`{"other":1}`:                     `{"other":1}`,
		`{oops`:                           `{oops`,
	}
	for args, want := range cases {
		if got := details(json.RawMessage(args)); got != want {
			t.Errorf("details(%s) = %q, want %q", args, got, want)
		}
	}

	long := `{"command":"` + strings.Repeat("a", detailLimit*2) + `"}`
	if got := details(json.RawMessage(long)); len(got) != detailLimit+3 {
		t.Errorf("a long line was not cut: %d characters", len(got))
	}
}
