package agent

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

// assistant is one answer of the model.
func assistant(content string) *schema.Message {
	return schema.AssistantMessage(content, nil)
}

var errNoReply = errors.New("the fake model has no reply left")

// lastToolResult is the newest tool result in what the model was given.
func lastToolResult(history History) string {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == schema.Tool {
			return history[i].Content
		}
	}
	return ""
}

func TestChatStreamsTheAnswer(t *testing.T) {
	fake := &fakeModel{replies: []reply{{message: assistant("the answer is here")}}}
	client := testClient(t, fake)

	var out strings.Builder
	answer, err := client.Chat(context.Background(), History(nil).WithUser("ask"), &out)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if answer != "the answer is here" {
		t.Errorf("answer = %q", answer)
	}
	if !strings.Contains(out.String(), "the answer is here") {
		t.Errorf("stream = %q", out.String())
	}
	if client.Model() != "test-model" {
		t.Errorf("model = %q", client.Model())
	}
}

func TestChatRunsAToolAndAsksAgain(t *testing.T) {
	asked := assistant("")
	asked.ToolCalls = append(asked.ToolCalls, toolCall("call_1", "echo", `{"text":"pong"}`))
	fake := &fakeModel{replies: []reply{{message: asked}, {message: assistant("done")}}}
	client := testClient(t, fake, echoTool(t, nil))

	var out strings.Builder
	answer, err := client.Chat(context.Background(), History(nil).WithUser("say pong"), &out)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if answer != "done" {
		t.Errorf("answer = %q", answer)
	}
	if !strings.Contains(out.String(), "--- tool: echo") {
		t.Errorf("stream does not name the tool: %q", out.String())
	}
	if fake.times() != 2 {
		t.Errorf("the model was asked %d times, want 2", fake.times())
	}
	// The second call carried the tool call and its result.
	if got := lastToolResult(fake.history(1)); got != "pong" {
		t.Errorf("the model was told %q, want the result of the tool", got)
	}
}

func TestChatReportsAFailedToolToTheModel(t *testing.T) {
	asked := assistant("")
	asked.ToolCalls = append(asked.ToolCalls, toolCall("call_1", "echo", `{"text":"pong"}`))
	fake := &fakeModel{replies: []reply{{message: asked}, {message: assistant("I cannot")}}}
	client := testClient(t, fake, echoTool(t, errors.New("the workspace is gone")))

	answer, err := client.Chat(context.Background(), History(nil).WithUser("say pong"), io.Discard)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if answer != "I cannot" {
		t.Errorf("answer = %q", answer)
	}
	if got := lastToolResult(fake.history(1)); !strings.Contains(got, "the workspace is gone") {
		t.Errorf("the model was told %q, want the error of the tool", got)
	}
}

func TestChatReportsAToolTheModelInvented(t *testing.T) {
	asked := assistant("")
	asked.ToolCalls = append(asked.ToolCalls, toolCall("call_1", "fly", `{}`))
	fake := &fakeModel{replies: []reply{{message: asked}, {message: assistant("I cannot")}}}
	client := testClient(t, fake, echoTool(t, nil))

	if _, err := client.Chat(context.Background(), History(nil).WithUser("fly"), io.Discard); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if got := lastToolResult(fake.history(1)); !strings.Contains(got, `no tool named "fly"`) {
		t.Errorf("the model was told %q", got)
	}
}

func TestChatTriesAFailedCallAgain(t *testing.T) {
	fake := &fakeModel{replies: []reply{
		{err: errors.New("rate limited")},
		{message: assistant("second time")},
	}}
	client := testClient(t, fake)

	answer, err := client.Chat(context.Background(), History(nil).WithUser("ask"), io.Discard)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if answer != "second time" {
		t.Errorf("answer = %q", answer)
	}
	if fake.times() != 2 {
		t.Errorf("the model was asked %d times, want 2", fake.times())
	}
}

func TestChatGivesUpAfterTheRetries(t *testing.T) {
	var replies []reply
	for range maxRetries + 1 {
		replies = append(replies, reply{err: errors.New("rate limited")})
	}
	client := testClient(t, &fakeModel{replies: replies})

	if _, err := client.Chat(context.Background(), History(nil).WithUser("ask"), io.Discard); err == nil {
		t.Fatal("expected the failure to reach the caller")
	}
}

func TestChatStopsWhenTheCallerStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := testClient(t, &fakeModel{replies: []reply{{message: assistant("never")}}})
	if _, err := client.Chat(ctx, History(nil).WithUser("ask"), io.Discard); err == nil {
		t.Fatal("expected the stopped run to fail")
	}
}

func TestChatCarriesTheWholeHistoryToTheModel(t *testing.T) {
	fake := &fakeModel{replies: []reply{{message: assistant("two")}}}
	client := testClient(t, fake)

	history := History(nil).WithUser("one").WithAssistant("answer").WithUser("two")
	if _, err := client.Chat(context.Background(), history, io.Discard); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	seen := fake.history(0)
	if len(seen) != 3 || seen[0].Content != "one" || seen[2].Content != "two" {
		t.Errorf("the model saw %#v", seen)
	}
}

func TestChatReportsAnEmptyStream(t *testing.T) {
	client := testClient(t, &fakeModel{replies: []reply{{}}})

	if _, err := client.Chat(context.Background(), History(nil).WithUser("ask"), io.Discard); err == nil {
		t.Fatal("expected an error for an answer with no message")
	}
}

func TestChatReportsAStreamThatBreaks(t *testing.T) {
	broken := errors.New("the connection dropped")
	client := testClient(t, &fakeModel{replies: []reply{{streamErr: broken}}})

	_, err := client.Chat(context.Background(), History(nil).WithUser("ask"), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "the connection dropped") {
		t.Fatalf("err = %v", err)
	}
}

func TestChatReportsChunksThatDoNotJoin(t *testing.T) {
	mixed := []*schema.Message{assistant("half"), schema.UserMessage("other")}
	client := testClient(t, &fakeModel{replies: []reply{{chunks: mixed}}})

	if _, err := client.Chat(context.Background(), History(nil).WithUser("ask"), io.Discard); err == nil {
		t.Fatal("expected an error for chunks of two roles")
	}
}

func TestNewBuildsAClientForOpenRouter(t *testing.T) {
	client, err := New(context.Background(), "test-key")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if client.Model() != slug {
		t.Errorf("model = %q, want %q", client.Model(), slug)
	}
}
