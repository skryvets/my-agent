package main

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

type telegramCall struct {
	Method  string
	Payload map[string]any
}

type fakeTelegram struct {
	server *httptest.Server

	mu      sync.Mutex
	calls   []telegramCall
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
		fake.calls = append(fake.calls, telegramCall{Method: method, Payload: payload})
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

func (f *fakeTelegram) client() *telegramClient {
	client := newTelegramClient("test-token")
	client.baseURL = f.server.URL
	return client
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

func newTestBot(fake *fakeTelegram, answer func([]map[string]any) (assistantMessage, error)) *telegramBot {
	return &telegramBot{
		client:    fake.client(),
		answer:    answer,
		retryBase: time.Millisecond,
		sessions:  map[int64]chan string{},
	}
}

func textUpdate(id, userID, chatID int64, text string) telegramUpdate {
	message := &telegramMessage{Text: text}
	message.MessageID = id
	message.From.ID = userID
	message.Chat.ID = chatID
	message.Chat.Type = "private"
	return telegramUpdate{UpdateID: id, Message: message}
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
	if payload["offset"] != float64(5) || payload["timeout"] != float64(telegramPollSeconds) {
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
	var apiErr *telegramError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want *telegramError", err)
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
	long := strings.Repeat("a", telegramMaxMessage) + "\ntail"

	if err := fake.client().sendMessage(context.Background(), 99, long); err != nil {
		t.Fatalf("sendMessage: %v", err)
	}
	if first := fake.nextSent(t); len([]rune(first)) != telegramMaxMessage {
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

func TestServeAnswersWithHistoryAndTypingAction(t *testing.T) {
	fake := newFakeTelegram(t)
	var seen [][]map[string]any
	bot := newTestBot(fake, func(messages []map[string]any) (assistantMessage, error) {
		snapshot := append([]map[string]any(nil), messages...)
		seen = append(seen, snapshot)
		return assistantMessage{Content: fmt.Sprintf("answer %d", len(seen))}, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bot.dispatch(ctx, textUpdate(1, 42, 99, "first"))
	if got := fake.nextSent(t); got != "answer 1" {
		t.Fatalf("reply = %q", got)
	}
	bot.dispatch(ctx, textUpdate(2, 42, 99, "second"))
	if got := fake.nextSent(t); got != "answer 2" {
		t.Fatalf("reply = %q", got)
	}

	if len(seen) != 2 || len(seen[1]) != 3 {
		t.Fatalf("history = %#v", seen)
	}
	if seen[1][0]["content"] != "first" || seen[1][1]["content"] != "answer 1" || seen[1][2]["content"] != "second" {
		t.Errorf("history = %#v", seen[1])
	}
	if seen[1][1]["role"] != "assistant" {
		t.Errorf("assistant turn = %#v", seen[1][1])
	}

	methods := fake.methods()
	if methods[0] != "sendChatAction" {
		t.Errorf("methods = %v", methods)
	}
	if action := fake.calls[0].Payload["action"]; action != "typing" {
		t.Errorf("action = %#v", action)
	}
}

func TestServeReportsFailedTurnAndDropsIt(t *testing.T) {
	fake := newFakeTelegram(t)
	var seen [][]map[string]any
	bot := newTestBot(fake, func(messages []map[string]any) (assistantMessage, error) {
		seen = append(seen, append([]map[string]any(nil), messages...))
		if len(seen) == 1 {
			return assistantMessage{}, errors.New("rate limited")
		}
		return assistantMessage{Content: "ok"}, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bot.dispatch(ctx, textUpdate(1, 42, 99, "boom"))
	if got := fake.nextSent(t); !strings.Contains(got, "rate limited") {
		t.Fatalf("reply = %q", got)
	}
	bot.dispatch(ctx, textUpdate(2, 42, 99, "retry"))
	if got := fake.nextSent(t); got != "ok" {
		t.Fatalf("reply = %q", got)
	}
	if len(seen[1]) != 1 || seen[1][0]["content"] != "retry" {
		t.Errorf("failed turn was kept: %#v", seen[1])
	}
}

func TestServeHandlesCommands(t *testing.T) {
	fake := newFakeTelegram(t)
	var seen [][]map[string]any
	bot := newTestBot(fake, func(messages []map[string]any) (assistantMessage, error) {
		seen = append(seen, append([]map[string]any(nil), messages...))
		return assistantMessage{Content: "answer"}, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bot.dispatch(ctx, textUpdate(1, 42, 99, "/start"))
	if got := fake.nextSent(t); !strings.Contains(got, "/reset") {
		t.Fatalf("start reply = %q", got)
	}
	bot.dispatch(ctx, textUpdate(2, 42, 99, "hello"))
	fake.nextSent(t)
	bot.dispatch(ctx, textUpdate(3, 42, 99, "/reset"))
	if got := fake.nextSent(t); got != "Conversation cleared." {
		t.Fatalf("reset reply = %q", got)
	}
	bot.dispatch(ctx, textUpdate(4, 42, 99, "again"))
	fake.nextSent(t)

	if len(seen) != 2 || len(seen[1]) != 1 {
		t.Errorf("history was not cleared: %#v", seen)
	}
}

func TestDispatchIgnoresNonTextAndDisallowedUsers(t *testing.T) {
	fake := newFakeTelegram(t)
	bot := newTestBot(fake, func([]map[string]any) (assistantMessage, error) {
		t.Error("model should not be called")
		return assistantMessage{}, nil
	})
	bot.allowed = map[int64]bool{7: true}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bot.dispatch(ctx, telegramUpdate{UpdateID: 1})
	bot.dispatch(ctx, textUpdate(2, 7, 99, "   "))
	bot.dispatch(ctx, textUpdate(3, 42, 99, "let me in"))
	fake.expectNoSend(t)
}

func TestDispatchTellsSenderWhenQueueIsFull(t *testing.T) {
	fake := newFakeTelegram(t)
	release := make(chan struct{})
	bot := newTestBot(fake, func([]map[string]any) (assistantMessage, error) {
		<-release
		return assistantMessage{Content: "done"}, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer close(release)

	for id := int64(1); id <= telegramQueueSize+2; id++ {
		bot.dispatch(ctx, textUpdate(id, 42, 99, "queued"))
	}
	if got := fake.nextSent(t); !strings.Contains(got, "still working") {
		t.Fatalf("reply = %q", got)
	}
}

func TestRunTracksOffsetAndStopsOnContextCancel(t *testing.T) {
	fake := newFakeTelegram(t)
	fake.reply("getUpdates", `{"ok":true,"result":[{"update_id":7,"message":{"message_id":1,"from":{"id":42},"chat":{"id":99,"type":"private"},"text":"/help"}}]}`)
	bot := newTestBot(fake, func([]map[string]any) (assistantMessage, error) {
		return assistantMessage{Content: "unused"}, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- bot.run(ctx) }()

	fake.nextSent(t)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("run returned %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("run did not stop")
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.calls[0].Payload["offset"] != nil {
		t.Errorf("first poll should have no offset: %#v", fake.calls[0].Payload)
	}
	for _, call := range fake.calls {
		if call.Method == "getUpdates" && call.Payload["offset"] != nil && call.Payload["offset"] != float64(8) {
			t.Errorf("offset = %#v, want 8", call.Payload["offset"])
		}
	}
}

func TestParseAllowedUsers(t *testing.T) {
	allowed, err := parseAllowedUsers(" 42, 7 ,")
	if err != nil {
		t.Fatalf("parseAllowedUsers: %v", err)
	}
	if !reflect.DeepEqual(allowed, map[int64]bool{42: true, 7: true}) {
		t.Errorf("allowed = %#v", allowed)
	}
	if empty, err := parseAllowedUsers(""); err != nil || len(empty) != 0 {
		t.Errorf("empty = %#v, err = %v", empty, err)
	}
	if _, err := parseAllowedUsers("sergey"); err == nil {
		t.Error("expected error for a non-numeric id")
	}
}

func TestRetryDelayPrefersRetryAfter(t *testing.T) {
	err := &telegramError{RetryAfter: 12}
	if got := retryDelay(err, time.Second); got != 12*time.Second {
		t.Errorf("got %v", got)
	}
	if got := retryDelay(errors.New("boom"), 4*time.Second); got != 4*time.Second {
		t.Errorf("got %v", got)
	}
}

func TestTrimHistoryDropsWholeTurns(t *testing.T) {
	history := []map[string]any{
		{"role": "user", "content": "1"},
		{"role": "assistant", "content": "2"},
		{"role": "user", "content": "3"},
		{"role": "assistant", "content": "4"},
	}
	got := trimHistory(history, 2)
	want := history[2:]
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
	if got := trimHistory(history, 3); got[0]["role"] != "user" {
		t.Errorf("history starts on %#v", got[0])
	}
	if got := trimHistory(history, 10); !reflect.DeepEqual(got, history) {
		t.Errorf("short history was trimmed: %#v", got)
	}
}

func TestRunTelegramRequiresToken(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "")
	if err := runTelegram("key"); err == nil {
		t.Fatal("expected an error without a bot token")
	}
	t.Setenv("TELEGRAM_BOT_TOKEN", "token")
	t.Setenv("TELEGRAM_ALLOWED_USERS", "nope")
	if err := runTelegram("key"); err == nil {
		t.Fatal("expected an error for a bad allowlist")
	}
}

func TestRunRetriesAfterAPollFailure(t *testing.T) {
	fake := newFakeTelegram(t)
	fake.reply("getUpdates", `{"ok":false,"error_code":500,"description":"internal"}`)
	bot := newTestBot(fake, func([]map[string]any) (assistantMessage, error) {
		return assistantMessage{Content: "hi"}, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- bot.run(ctx) }()

	time.Sleep(20 * time.Millisecond)
	fake.reply("getUpdates", `{"ok":true,"result":[{"update_id":3,"message":{"message_id":1,"from":{"id":42},"chat":{"id":99,"type":"private"},"text":"/help"}}]}`)
	fake.nextSent(t)

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("run did not stop")
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	polls := 0
	for _, call := range fake.calls {
		if call.Method == "getUpdates" {
			polls++
		}
	}
	if polls < 2 {
		t.Errorf("polls = %d, want the failed poll to be retried", polls)
	}
}

func TestSleepStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if sleep(ctx, time.Hour) {
		t.Error("sleep should report a cancelled context")
	}
	if !sleep(context.Background(), time.Millisecond) {
		t.Error("sleep should report a completed wait")
	}
}

func TestTrimHistoryDropsEverythingWhenLimitIsZero(t *testing.T) {
	history := []map[string]any{{"role": "user"}, {"role": "assistant"}}
	if got := trimHistory(history, 0); got != nil {
		t.Errorf("got %#v, want nil", got)
	}
}
