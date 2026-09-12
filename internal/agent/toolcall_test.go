package agent

import (
	"reflect"
	"testing"
)

func TestMergeToolCallsJoinsIndexedFragments(t *testing.T) {
	var calls []map[string]any
	calls = mergeToolCalls(calls, []map[string]any{
		{"index": float64(0), "id": "call_1", "type": "function", "function": map[string]any{"name": "shell", "arguments": `{"command":`}},
		{"index": float64(1), "id": "call_2", "type": "function", "function": map[string]any{"name": "fetch", "arguments": `{"url":"a"}`}},
	})
	calls = mergeToolCalls(calls, []map[string]any{
		{"index": float64(0), "id": "", "function": map[string]any{"arguments": `"ls"}`}},
	})

	if len(calls) != 2 {
		t.Fatalf("calls = %#v", calls)
	}
	name, arguments := callFunction(calls[0])
	if name != "shell" || arguments != `{"command":"ls"}` {
		t.Errorf("first call = %q %q", name, arguments)
	}
	if calls[0]["id"] != "call_1" {
		t.Errorf("an empty id fragment erased the id: %#v", calls[0])
	}
	if name, _ := callFunction(calls[1]); name != "fetch" {
		t.Errorf("second call = %#v", calls[1])
	}
}

func TestMergeToolCallsHandlesMissingPieces(t *testing.T) {
	// A fragment without an index starts a new call, and a first fragment
	// without a function still leaves a call the later fragments can fill.
	calls := mergeToolCalls(nil, []map[string]any{
		{"id": "call_1"},
		{"id": "call_2"},
	})
	if len(calls) != 2 {
		t.Fatalf("calls = %#v", calls)
	}

	calls = mergeToolCalls(nil, []map[string]any{{"index": float64(0), "id": "call_1"}})
	calls = mergeToolCalls(calls, []map[string]any{
		{"index": float64(0), "function": map[string]any{"name": "shell", "arguments": float64(7)}},
	})
	name, arguments := callFunction(calls[0])
	if name != "shell" || arguments != "{}" {
		t.Errorf("call = %q %q, want empty arguments as an empty object", name, arguments)
	}
}

func TestMergeToolCallsDoesNotShareTheFragmentMaps(t *testing.T) {
	fragment := map[string]any{"index": float64(0), "function": map[string]any{"arguments": "one"}}
	calls := mergeToolCalls(nil, []map[string]any{fragment})
	calls = mergeToolCalls(calls, []map[string]any{
		{"index": float64(0), "function": map[string]any{"arguments": "two"}},
	})

	function, _ := fragment["function"].(map[string]any)
	if function["arguments"] != "one" {
		t.Errorf("the fragment was written to: %#v", fragment)
	}
	if _, arguments := callFunction(calls[0]); arguments != "onetwo" {
		t.Errorf("arguments = %q", arguments)
	}
}

func TestWireCallsDropsTheStreamingIndex(t *testing.T) {
	wire := wireCalls([]map[string]any{call(3, "call_1", "shell", `{"command":"ls"}`)})

	want := []map[string]any{{
		"id":       "call_1",
		"type":     "function",
		"function": map[string]any{"name": "shell", "arguments": `{"command":"ls"}`},
	}}
	if !reflect.DeepEqual(wire, want) {
		t.Errorf("got %#v, want %#v", wire, want)
	}
}

func TestToolResultNamesTheCallItAnswers(t *testing.T) {
	result := toolResult(call(0, "call_1", "shell", "{}"), "output")

	want := map[string]any{"role": "tool", "tool_call_id": "call_1", "name": "shell", "content": "output"}
	if !reflect.DeepEqual(result, want) {
		t.Errorf("got %#v, want %#v", result, want)
	}
}
