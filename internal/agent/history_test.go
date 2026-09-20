package agent

import (
	"reflect"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestHistoryAppendsTurns(t *testing.T) {
	history := History(nil).WithUser("hi").WithAssistant(Message{Content: "hi back"})

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

func TestHistoryReplaysToolCallsBeforeTheAnswer(t *testing.T) {
	steps := History{
		schema.AssistantMessage("", []schema.ToolCall{toolCall("call_1", "shell", `{"command":"ls"}`)}),
		schema.ToolMessage("README.md", "call_1"),
	}
	history := History(nil).WithUser("list the files").
		WithAssistant(Message{Content: "one file", Steps: steps})

	if len(history) != 4 {
		t.Fatalf("history = %#v", history)
	}
	if history[1].Role != schema.Assistant || len(history[1].ToolCalls) != 1 {
		t.Errorf("call turn = %#v", history[1])
	}
	if history[2].Role != schema.Tool || history[2].Content != "README.md" {
		t.Errorf("result turn = %#v", history[2])
	}
	if history[3].Content != "one file" || history[3].ToolCalls != nil {
		t.Errorf("answer turn = %#v", history[3])
	}
}

func TestHistoryTrimNeverStartsOnAToolResult(t *testing.T) {
	history := History{
		schema.UserMessage("1"),
		schema.AssistantMessage("", []schema.ToolCall{toolCall("call_1", "shell", "{}")}),
		schema.ToolMessage("2", "call_1"),
		schema.AssistantMessage("3", nil),
		schema.UserMessage("4"),
		schema.AssistantMessage("5", nil),
	}
	for _, limit := range []int{1, 2, 3, 4, 5} {
		got := history.Trim(limit)
		if len(got) == 0 {
			continue
		}
		if got[0].Role != schema.User {
			t.Errorf("Trim(%d) starts on %#v", limit, got[0])
		}
	}
}
