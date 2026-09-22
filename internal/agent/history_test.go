package agent

import (
	"reflect"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestHistoryAppendsTurns(t *testing.T) {
	history := History(nil).WithUser("hi").WithAssistant("hi back")

	want := History{schema.UserMessage("hi"), schema.AssistantMessage("hi back", nil)}
	if !reflect.DeepEqual(history, want) {
		t.Errorf("got %#v, want %#v", history, want)
	}
}

func TestHistoryDropLast(t *testing.T) {
	history := History(nil).WithUser("one").WithUser("two")
	if got := history.DropLast(); len(got) != 1 || got[0].Content != "one" {
		t.Errorf("got %#v", got)
	}
	if got := History(nil).DropLast(); len(got) != 0 {
		t.Errorf("got %#v, want empty", got)
	}
}

func TestHistoryTrimDropsWholeTurns(t *testing.T) {
	history := History{
		schema.UserMessage("1"),
		schema.AssistantMessage("2", nil),
		schema.UserMessage("3"),
		schema.AssistantMessage("4", nil),
	}
	got := history.Trim(2)
	want := history[2:]
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
	if got := history.Trim(3); got[0].Role != schema.User {
		t.Errorf("history starts on %#v", got[0])
	}
	if got := history.Trim(10); !reflect.DeepEqual(got, history) {
		t.Errorf("short history was trimmed: %#v", got)
	}
}

func TestHistoryTrimDropsEverythingWhenLimitIsZero(t *testing.T) {
	history := History{schema.UserMessage("1"), schema.AssistantMessage("2", nil)}
	if got := history.Trim(0); got != nil {
		t.Errorf("got %#v, want nil", got)
	}
}
