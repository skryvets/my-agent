package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/skryvets/my-agent/internal/agent"
)

type apiCall struct {
	Method  string
	Payload map[string]any
}

type fakeTelegram struct {
	server *httptest.Server

	mu      sync.Mutex
	calls   []apiCall
	replies map[string]string
	sent    chan string
}

func newFakeTelegram(t *testing.T) *fakeTelegram {
	t.Helper()
	fake := &fakeTelegram{
		replies: map[string]string{},
		sent:    make(chan string, 16),
	}
	fake.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decoding %s payload: %v", method, err)
		}

		fake.mu.Lock()
		fake.calls = append(fake.calls, apiCall{Method: method, Payload: payload})
		reply, ok := fake.replies[method]
		fake.mu.Unlock()

		if method == "sendMessage" {
			text, _ := payload["text"].(string)
			fake.sent <- text
		}
		if !ok {
			reply = `{"ok":true,"result":true}`
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, reply)
	}))
	t.Cleanup(fake.server.Close)
	return fake
}

func (f *fakeTelegram) reply(method, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.replies[method] = body
}

func (f *fakeTelegram) methods() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var methods []string
	for _, call := range f.calls {
		methods = append(methods, call.Method)
	}
	return methods
}

func (f *fakeTelegram) client() *client {
	c := newClient("test-token")
	c.baseURL = f.server.URL
	return c
}

func (f *fakeTelegram) nextSent(t *testing.T) string {
	t.Helper()
	select {
	case text := <-f.sent:
		return text
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for sendMessage")
		return ""
	}
}

func (f *fakeTelegram) expectNoSend(t *testing.T) {
	t.Helper()
	select {
	case text := <-f.sent:
		t.Fatalf("unexpected sendMessage: %q", text)
	case <-time.After(100 * time.Millisecond):
	}
}

type fakeAgent struct {
	answer func(agent.History) (agent.Message, error)
}

func (f fakeAgent) Model() string { return "test-model" }

func (f fakeAgent) Chat(_ context.Context, history agent.History, _ io.Writer) (agent.Message, error) {
	return f.answer(history)
}

func newTestBot(fake *fakeTelegram, answer func(agent.History) (agent.Message, error)) *Bot {
	return &Bot{
		client:    fake.client(),
		agent:     fakeAgent{answer: answer},
		retryBase: time.Millisecond,
		sessions:  map[int64]chan string{},
		working:   map[int64]context.CancelFunc{},
	}
}

func textUpdate(id, userID, chatID int64, text string) update {
	msg := &message{Text: text}
	msg.From.ID = userID
	msg.Chat.ID = chatID
	return update{UpdateID: id, Message: msg}
}
