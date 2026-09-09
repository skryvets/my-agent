package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestClient(handler http.HandlerFunc) (*Client, func()) {
	server := httptest.NewServer(handler)
	client := New("test-key")
	client.apiURL = server.URL
	return client, server.Close
}

func TestChatSendsHistoryAndStreamsTheReply(t *testing.T) {
	var request map[string]any
	var authorization string

	client, done := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decoding request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, `data: {"choices":[{"delta":{"content":"hello"}}]}`+"\n"+"data: [DONE]\n")
	})
	defer done()

	var out strings.Builder
	msg, err := client.Chat(context.Background(), History(nil).WithUser("hi"), &out)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if msg.Content != "hello" {
		t.Errorf("content = %q", msg.Content)
	}
	if !strings.Contains(out.String(), "hello") {
		t.Errorf("reply not streamed: %q", out.String())
	}
	if authorization != "Bearer test-key" {
		t.Errorf("Authorization = %q", authorization)
	}
	if request["model"] != Model || request["stream"] != true {
		t.Errorf("request = %#v", request)
	}
	messages, _ := request["messages"].([]any)
	if len(messages) != 1 {
		t.Fatalf("messages = %#v", request["messages"])
	}
	if turn, _ := messages[0].(map[string]any); turn["content"] != "hi" || turn["role"] != "user" {
		t.Errorf("user turn = %#v", messages[0])
	}
}

func TestChatReportsHTTPErrors(t *testing.T) {
	client, done := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		io.WriteString(w, "slow down")
	})
	defer done()

	_, err := client.Chat(context.Background(), History(nil).WithUser("hi"), io.Discard)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "slow down") {
		t.Errorf("err = %v", err)
	}
}

func TestChatStopsOnCancelledContext(t *testing.T) {
	client, done := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		t.Error("request should not reach the server")
	})
	defer done()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.Chat(ctx, History(nil).WithUser("hi"), io.Discard); err == nil {
		t.Fatal("expected an error for a cancelled context")
	}
}

func TestModelReportsTheSlug(t *testing.T) {
	if got := New("key").Model(); got != Model {
		t.Errorf("Model() = %q", got)
	}
}
