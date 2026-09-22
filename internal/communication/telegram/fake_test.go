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
	"github.com/skryvets/my-agent/internal/task"
)

type apiCall struct {
	Method string
	Form   map[string]string
}

// fakeTelegram answers the Bot API calls of one test and records them. The
// library posts a multipart form, so the fields arrive as strings.
type fakeTelegram struct {
	server *httptest.Server

	mu    sync.Mutex
	calls []apiCall
	sent  chan string
	// refuseActions answers sendChatAction with an error.
	refuseActions bool
}

func newFakeTelegram(t *testing.T) *fakeTelegram {
	t.Helper()
	fake := &fakeTelegram{sent: make(chan string, 16)}
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
		fake.mu.Unlock()

		reply := `{"ok":true,"result":true}`
		if method == "sendChatAction" && fake.refuseActions {
			reply = `{"ok":false,"description":"no typing today"}`
		}
		if method == "sendMessage" {
			fake.sent <- form["text"]
			reply = `{"ok":true,"result":{"message_id":1}}`
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, reply)
	}))
	t.Cleanup(fake.server.Close)
	return fake
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
	answer func(agent.History) (string, error)
}

func (f fakeAgent) Model() string { return "test-model" }

func (f fakeAgent) Chat(_ context.Context, history agent.History, _ io.Writer) (string, error) {
	return f.answer(history)
}

// fakeTasks records the run the bot asked for and reports back.
type fakeTasks struct {
	mu          sync.Mutex
	chat        string
	repository  string
	instruction string
	lines       []string
	lost        []task.Run
	// started, when it is set, is closed when a run begins, and the run then
	// lasts until it is stopped.
	started chan struct{}
}

func (f *fakeTasks) Start(ctx context.Context, chat, repository, instruction string, report task.Report) error {
	f.mu.Lock()
	f.chat, f.repository, f.instruction = chat, repository, instruction
	f.mu.Unlock()

	for _, line := range f.lines {
		report(line)
	}
	if f.started != nil {
		close(f.started)
		<-ctx.Done()
		return ctx.Err()
	}
	return nil
}

func (f *fakeTasks) Interrupted(ctx context.Context) []task.Run { return f.lost }

func (f *fakeTasks) seen() (chat, repository, instruction string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.chat, f.repository, f.instruction
}

func newTestBot(fake *fakeTelegram, answer func(agent.History) (string, error)) *Bot {
	api, err := bot.New("test-token",
		bot.WithServerURL(fake.server.URL),
		bot.WithSkipGetMe(),
		bot.WithNotAsyncHandlers(),
	)
	if err != nil {
		panic(err)
	}
	b := newBot(fakeAgent{answer: answer}, nil, nil)
	b.api = api
	return b
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
