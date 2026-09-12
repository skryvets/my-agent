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

func TestHistoryReplaysToolCallsBeforeTheAnswer(t *testing.T) {
	asked := Message{ToolCalls: []map[string]any{call(0, "call_1", "shell", `{"command":"ls"}`)}}
	steps := History(nil).WithToolCalls(asked)
	steps = append(steps, toolResult(asked.ToolCalls[0], "README.md"))

	history := History(nil).WithUser("list the files").
		WithAssistant(Message{Content: "one file", Steps: steps})

	if len(history) != 4 {
		t.Fatalf("history = %#v", history)
	}
	if history[1]["role"] != "assistant" || history[1]["tool_calls"] == nil {
		t.Errorf("call turn = %#v", history[1])
	}
	if history[2]["role"] != "tool" || history[2]["content"] != "README.md" {
		t.Errorf("result turn = %#v", history[2])
	}
	if history[3]["content"] != "one file" || history[3]["tool_calls"] != nil {
		t.Errorf("answer turn = %#v", history[3])
	}
}

func TestHistoryTrimNeverStartsOnAToolResult(t *testing.T) {
	history := History{
		{"role": "user", "content": "1"},
		{"role": "assistant", "tool_calls": []map[string]any{}},
		{"role": "tool", "content": "2"},
		{"role": "assistant", "content": "3"},
		{"role": "user", "content": "4"},
		{"role": "assistant", "content": "5"},
	}
	for _, limit := range []int{1, 2, 3, 4, 5} {
		got := history.Trim(limit)
		if len(got) == 0 {
			continue
		}
		if got[0]["role"] != "user" {
			t.Errorf("Trim(%d) starts on %#v", limit, got[0])
		}
	}
}
