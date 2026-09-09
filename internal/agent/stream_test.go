package agent

import (
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestReadStreamCollectsContentAndSkipsKeepAlives(t *testing.T) {
	sse := strings.Join([]string{
		": OPENROUTER PROCESSING",
		"",
		`data: {"choices":[{"delta":{"reasoning":"think "}}]}`,
		`data: {"choices":[{"delta":{"reasoning":"harder"}}]}`,
		`data: {"choices":[{"delta":{"content":"Hello"}}]}`,
		`data: {"choices":[{"delta":{"content":", world"}}]}`,
		`data: {"choices":[]}`,
		"data: [DONE]",
	}, "\n")

	var out strings.Builder
	msg, err := readStream(strings.NewReader(sse), &out)
	if err != nil {
		t.Fatalf("readStream: %v", err)
	}
	if msg.Content != "Hello, world" {
		t.Errorf("content = %q", msg.Content)
	}
	printed := out.String()
	if !strings.Contains(printed, "--- reasoning ---\nthink harder") {
		t.Errorf("reasoning not printed: %q", printed)
	}
	if !strings.Contains(printed, "--- answer ---\nHello, world") {
		t.Errorf("answer not printed: %q", printed)
	}
}

func TestReadStreamMergesReasoningDetails(t *testing.T) {
	sse := strings.Join([]string{
		`data: {"choices":[{"delta":{"reasoning_details":[{"index":0,"type":"reasoning.text","text":"one "}]}}]}`,
		`data: {"choices":[{"delta":{"reasoning_details":[{"index":0,"text":"two"}]}}]}`,
		`data: {"choices":[{"delta":{"content":"done"}}]}`,
		"data: [DONE]",
	}, "\n")

	msg, err := readStream(strings.NewReader(sse), io.Discard)
	if err != nil {
		t.Fatalf("readStream: %v", err)
	}
	want := []map[string]any{{"index": float64(0), "type": "reasoning.text", "text": "one two"}}
	if !reflect.DeepEqual(msg.ReasoningDetails, want) {
		t.Errorf("reasoning_details = %#v, want %#v", msg.ReasoningDetails, want)
	}
}

func TestReadStreamReturnsErrors(t *testing.T) {
	cases := map[string]string{
		"stream error":  `data: {"error":{"message":"rate limited"}}`,
		"bad chunk":     `data: {oops`,
		"empty content": "data: [DONE]",
	}
	for name, sse := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := readStream(strings.NewReader(sse), io.Discard); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestMergeReasoningDetailsKeepsSeparateIndexes(t *testing.T) {
	acc := mergeReasoningDetails(nil, []map[string]any{
		{"index": float64(0), "text": "a"},
		{"index": float64(1), "text": "b"},
	})
	acc = mergeReasoningDetails(acc, []map[string]any{{"index": float64(1), "text": "c"}})

	want := []map[string]any{
		{"index": float64(0), "text": "a"},
		{"index": float64(1), "text": "bc"},
	}
	if !reflect.DeepEqual(acc, want) {
		t.Errorf("got %#v, want %#v", acc, want)
	}
}

func TestMergeReasoningDetailsDoesNotMutateInput(t *testing.T) {
	delta := map[string]any{"index": float64(0), "text": "a"}
	acc := mergeReasoningDetails(nil, []map[string]any{delta})
	mergeReasoningDetails(acc, []map[string]any{{"index": float64(0), "text": "b"}})

	if delta["text"] != "a" {
		t.Errorf("input mutated: %#v", delta)
	}
}

func TestMergeReasoningDetailsMergesTextAndReplacesMetadata(t *testing.T) {
	details := mergeReasoningDetails(nil, []map[string]any{
		{"index": float64(0), "text": "a", "summary": "b", "data": "c", "type": "old", "format": "old"},
	})
	details = mergeReasoningDetails(details, []map[string]any{
		{"index": float64(0), "text": "d", "summary": "e", "data": "f", "type": "new"},
	})
	want := []map[string]any{
		{"index": float64(0), "text": "ad", "summary": "be", "data": "cf", "type": "new", "format": "old"},
	}
	if !reflect.DeepEqual(details, want) {
		t.Errorf("got %#v, want %#v", details, want)
	}
}

func TestMergeReasoningDetailsKeepsUnindexedFragmentsSeparate(t *testing.T) {
	fragments := []map[string]any{{"text": "a"}, {"text": "b"}}
	if got := mergeReasoningDetails(nil, fragments); !reflect.DeepEqual(got, fragments) {
		t.Errorf("got %#v, want %#v", got, fragments)
	}
}

func TestMergeReasoningDetailsReplacesNonStringValues(t *testing.T) {
	details := []map[string]any{{"index": float64(0), "text": nil, "summary": "old"}}
	fragment := map[string]any{"index": float64(0), "text": "new", "summary": nil, "data": "new"}
	want := []map[string]any{fragment}
	if got := mergeReasoningDetails(details, want); !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}
