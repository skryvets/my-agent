package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

// sse writes one streamed reply made of the given delta objects.
func sse(w http.ResponseWriter, deltas ...string) {
	w.Header().Set("Content-Type", "text/event-stream")
	for _, delta := range deltas {
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":%s}]}\n", delta)
	}
	io.WriteString(w, "data: [DONE]\n")
}

const shellCallDelta = `{"tool_calls":[{"index":0,"id":"call_1","type":"function",` +
	`"function":{"name":"one","arguments":"{}"}}]}`

func TestChatRunsToolsAndAsksTheModelAgain(t *testing.T) {
	var requests []map[string]any

	client, done := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decoding request: %v", err)
		}
		requests = append(requests, request)
		if len(requests) == 1 {
			sse(w, shellCallDelta)
			return
		}
		sse(w, `{"content":"the file is there"}`)
	})
	defer done()
	client.tools = NewRegistry(stubTool{name: "one"})

	var out strings.Builder
	history := History(nil).WithUser("is the file there?")
	msg, err := client.Chat(context.Background(), history, &out)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}

	if msg.Content != "the file is there" {
		t.Errorf("content = %q", msg.Content)
	}
	if len(requests) != 2 {
		t.Fatalf("the model was asked %d times, want 2", len(requests))
	}
	if requests[0]["tools"] == nil {
		t.Error("the first request carried no tools")
	}

	// The second request must replay the call and its result, in that order.
	messages, _ := requests[1]["messages"].([]any)
	if len(messages) != 3 {
		t.Fatalf("second request messages = %#v", messages)
	}
	asked, _ := messages[1].(map[string]any)
	if asked["role"] != "assistant" || asked["tool_calls"] == nil {
		t.Errorf("second message = %#v", asked)
	}
	answered, _ := messages[2].(map[string]any)
	if answered["role"] != "tool" || answered["content"] != "one ran" {
		t.Errorf("third message = %#v", answered)
	}
	if answered["tool_call_id"] != "call_1" {
		t.Errorf("third message = %#v", answered)
	}

	// The caller keeps the steps, so the next turn sees the same conversation.
	if len(msg.Steps) != 2 {
		t.Fatalf("steps = %#v", msg.Steps)
	}
	if got := history.WithAssistant(msg); len(got) != 4 || got[3]["content"] != msg.Content {
		t.Errorf("history = %#v", got)
	}
	if !strings.Contains(out.String(), "--- tool: one {} ---") {
		t.Errorf("the tool was not announced: %q", out.String())
	}
}

func TestChatStopsAfterTheRoundCap(t *testing.T) {
	rounds := 0
	client, done := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		rounds++
		sse(w, shellCallDelta)
	})
	defer done()
	client.tools = NewRegistry(stubTool{name: "one"})

	_, err := client.Chat(context.Background(), History(nil).WithUser("loop"), io.Discard)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "stopped after") {
		t.Errorf("err = %v", err)
	}
	if rounds != maxToolRounds+1 {
		t.Errorf("the model was asked %d times, want %d", rounds, maxToolRounds+1)
	}
}

func TestChatDoesNotWriteIntoTheCallersHistory(t *testing.T) {
	first := true
	client, done := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		if first {
			first = false
			sse(w, shellCallDelta)
			return
		}
		sse(w, `{"content":"done"}`)
	})
	defer done()
	client.tools = NewRegistry(stubTool{name: "one"})

	// A history with spare capacity is the case where a careless append would
	// overwrite the turn after the one it was given.
	history := make(History, 1, 8)
	history[0] = map[string]any{"role": "user", "content": "hi"}

	if _, err := client.Chat(context.Background(), history, io.Discard); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(history) != 1 || history[0]["content"] != "hi" {
		t.Errorf("history = %#v", history)
	}
}
