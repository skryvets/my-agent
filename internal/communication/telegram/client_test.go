package telegram

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

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
