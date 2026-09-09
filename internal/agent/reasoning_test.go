package agent

import (
	"reflect"
	"testing"
)

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
