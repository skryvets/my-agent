package telegram

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/skryvets/my-agent/internal/agent"
)

type apiCall struct {
	Method string
	Form   map[string]string
}

// fakeTelegram answers the Bot API calls of one test and records them. The
// library posts a multipart form, so the fields arrive as strings.
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

		form := map[string]string{}
		if err := r.ParseMultipartForm(1 << 20); err == nil {
			for name, values := range r.MultipartForm.Value {
				form[name] = values[0]
			}
		}

		fake.mu.Lock()
		fake.calls = append(fake.calls, apiCall{Method: method, Form: form})
		reply, ok := fake.replies[method]
		fake.mu.Unlock()

		if method == "sendMessage" {
			fake.sent <- form["text"]
			if !ok {
				reply, ok = `{"ok":true,"result":{"message_id":1}}`, true
			}
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
	api, err := bot.New("test-token",
		bot.WithServerURL(fake.server.URL),
		bot.WithSkipGetMe(),
		bot.WithNotAsyncHandlers(),
	)
	if err != nil {
		panic(err)
	}
	return &Bot{
		api:      api,
		agent:    fakeAgent{answer: answer},
		sessions: map[int64]chan string{},
		working:  map[int64]context.CancelFunc{},
	}
}

func textUpdate(id, userID, chatID int64, text string) *models.Update {
	return &models.Update{
		ID: id,
		Message: &models.Message{
			From: &models.User{ID: userID},
			Chat: models.Chat{ID: chatID},
			Text: text,
		},
	}
}
