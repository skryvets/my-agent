package agent

import (
	"reflect"
	"testing"
)

func TestHistoryAppendsTurns(t *testing.T) {
	msg := Message{
		Content:          "hi back",
		ReasoningDetails: []map[string]any{{"index": float64(0), "text": "think"}},
	}
	history := History(nil).WithUser("hi").WithAssistant(msg)

	want := History{
		{"role": "user", "content": "hi"},
		{"role": "assistant", "content": "hi back", "reasoning_details": msg.ReasoningDetails},
	}
	if !reflect.DeepEqual(history, want) {
		t.Errorf("got %#v, want %#v", history, want)
	}
}

func TestHistoryDropLast(t *testing.T) {
	history := History(nil).WithUser("one").WithUser("two")
	if got := history.DropLast(); len(got) != 1 || got[0]["content"] != "one" {
		t.Errorf("got %#v", got)
	}
	if got := History(nil).DropLast(); len(got) != 0 {
		t.Errorf("got %#v, want empty", got)
	}
}

func TestHistoryTrimDropsWholeTurns(t *testing.T) {
	history := History{
		{"role": "user", "content": "1"},
		{"role": "assistant", "content": "2"},
		{"role": "user", "content": "3"},
		{"role": "assistant", "content": "4"},
	}
	got := history.Trim(2)
	want := history[2:]
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
	if got := history.Trim(3); got[0]["role"] != "user" {
		t.Errorf("history starts on %#v", got[0])
	}
	if got := history.Trim(10); !reflect.DeepEqual(got, history) {
		t.Errorf("short history was trimmed: %#v", got)
	}
}

func TestHistoryTrimDropsEverythingWhenLimitIsZero(t *testing.T) {
	history := History{{"role": "user"}, {"role": "assistant"}}
	if got := history.Trim(0); got != nil {
		t.Errorf("got %#v, want nil", got)
	}
}
