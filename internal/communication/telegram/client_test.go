package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
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

func TestGetUpdatesParsesMessagesAndSetsPollParameters(t *testing.T) {
	fake := newFakeTelegram(t)
	fake.reply("getUpdates", `{"ok":true,"result":[{"update_id":7,"message":{"message_id":1,"from":{"id":42,"username":"sergey"},"chat":{"id":99,"type":"private"},"text":"hi"}}]}`)

	updates, err := fake.client().getUpdates(context.Background(), 5)
	if err != nil {
		t.Fatalf("getUpdates: %v", err)
	}
	if len(updates) != 1 || updates[0].UpdateID != 7 || updates[0].Message.Text != "hi" {
		t.Fatalf("updates = %#v", updates)
	}
	if updates[0].Message.From.ID != 42 || updates[0].Message.Chat.ID != 99 {
		t.Errorf("message = %#v", updates[0].Message)
	}

	payload := fake.calls[0].Payload
	if payload["offset"] != float64(5) || payload["timeout"] != float64(pollSeconds) {
		t.Errorf("payload = %#v", payload)
	}
	if !reflect.DeepEqual(payload["allowed_updates"], []any{"message"}) {
		t.Errorf("allowed_updates = %#v", payload["allowed_updates"])
	}
}

func TestGetUpdatesOmitsZeroOffset(t *testing.T) {
	fake := newFakeTelegram(t)
	fake.reply("getUpdates", `{"ok":true,"result":[]}`)

	if _, err := fake.client().getUpdates(context.Background(), 0); err != nil {
		t.Fatalf("getUpdates: %v", err)
	}
	if _, ok := fake.calls[0].Payload["offset"]; ok {
		t.Errorf("offset should be omitted: %#v", fake.calls[0].Payload)
	}
}

func TestCallReturnsAPIError(t *testing.T) {
	fake := newFakeTelegram(t)
	fake.reply("getUpdates", `{"ok":false,"error_code":429,"description":"Too Many Requests","parameters":{"retry_after":12}}`)

	_, err := fake.client().getUpdates(context.Background(), 1)
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want *Error", err)
	}
	if apiErr.Code != 429 || apiErr.RetryAfter != 12 {
		t.Errorf("apiErr = %#v", apiErr)
	}
	if !strings.Contains(apiErr.Error(), "Too Many Requests") {
		t.Errorf("Error() = %q", apiErr.Error())
	}
}

func TestSendMessageSplitsLongText(t *testing.T) {
	fake := newFakeTelegram(t)
	long := strings.Repeat("a", maxMessage) + "\ntail"

	if err := fake.client().sendMessage(context.Background(), 99, long); err != nil {
		t.Fatalf("sendMessage: %v", err)
	}
	if first := fake.nextSent(t); len([]rune(first)) != maxMessage {
		t.Errorf("first part length = %d", len([]rune(first)))
	}
	if second := fake.nextSent(t); second != "tail" {
		t.Errorf("second part = %q", second)
	}
	if got := fake.calls[0].Payload["chat_id"]; got != float64(99) {
		t.Errorf("chat_id = %#v", got)
	}
}

func TestSplitMessage(t *testing.T) {
	cases := map[string]struct {
		text  string
		limit int
		want  []string
	}{
		"short":                {"hello", 10, []string{"hello"}},
		"empty":                {"", 10, []string{""}},
		"splits on blank":      {"one\n\ntwo", 5, []string{"one", "two"}},
		"splits on space":      {"one two three", 8, []string{"one two", "three"}},
		"no separator":         {"abcdefgh", 4, []string{"abcd", "efgh"}},
		"keeps runes together": {strings.Repeat("é", 5), 2, []string{"éé", "éé", "é"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := splitMessage(tc.text, tc.limit)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %q, want %q", got, tc.want)
			}
			for _, part := range got {
				if len([]rune(part)) > tc.limit {
					t.Errorf("part %q exceeds limit %d", part, tc.limit)
				}
			}
		})
	}
}
