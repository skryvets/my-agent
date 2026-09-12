package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// stubTool answers with a fixed string, or fails, and records the arguments.
type stubTool struct {
	name  string
	err   error
	seen  chan string
	start func()
}

func (s stubTool) Name() string        { return s.name }
func (s stubTool) Description() string { return s.name + " does something" }
func (s stubTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}

func (s stubTool) Call(ctx context.Context, args json.RawMessage) (string, error) {
	if s.start != nil {
		s.start()
	}
	if s.seen != nil {
		s.seen <- string(args)
	}
	if s.err != nil {
		return "", s.err
	}
	return s.name + " ran", nil
}

func call(index int, id, name, arguments string) map[string]any {
	return map[string]any{
		"index":    float64(index),
		"id":       id,
		"type":     "function",
		"function": map[string]any{"name": name, "arguments": arguments},
	}
}

func TestRegistryKeepsTheFirstToolOfAName(t *testing.T) {
	registry := NewRegistry(stubTool{name: "one"}, stubTool{name: "two"}, stubTool{name: "one", err: errors.New("shadow")})

	schemas := registry.schemas()
	if len(schemas) != 2 {
		t.Fatalf("schemas = %#v", schemas)
	}
	function, _ := schemas[0]["function"].(map[string]any)
	if schemas[0]["type"] != "function" || function["name"] != "one" {
		t.Errorf("first schema = %#v", schemas[0])
	}
	if function["description"] != "one does something" || function["parameters"] == nil {
		t.Errorf("first schema = %#v", schemas[0])
	}
	if got := registry.call(context.Background(), call(0, "a", "one", "{}")); got != "one ran" {
		t.Errorf("call = %q, want the first tool of the name", got)
	}
}

func TestRegistrySchemasAreEmptyWithoutTools(t *testing.T) {
	if schemas := NewRegistry().schemas(); schemas != nil {
		t.Errorf("schemas = %#v, want nil", schemas)
	}
	var missing *Registry
	if schemas := missing.schemas(); schemas != nil {
		t.Errorf("schemas of a nil registry = %#v, want nil", schemas)
	}
}

func TestRegistryReportsFailuresToTheModel(t *testing.T) {
	registry := NewRegistry(stubTool{name: "broken", err: errors.New("disk is full")})

	if got := registry.call(context.Background(), call(0, "a", "broken", "{}")); got != "error: disk is full" {
		t.Errorf("failed call = %q", got)
	}
	got := registry.call(context.Background(), call(0, "a", "absent", "{}"))
	if !strings.Contains(got, `no tool named "absent"`) {
		t.Errorf("unknown tool = %q", got)
	}
}

func TestRegistryRunKeepsTheOrderOfTheCalls(t *testing.T) {
	seen := make(chan string, 2)
	registry := NewRegistry(
		stubTool{name: "one", seen: seen},
		stubTool{name: "two", seen: seen, err: errors.New("no")},
	)
	calls := []map[string]any{
		call(0, "id-1", "one", `{"a":1}`),
		call(1, "id-2", "two", `{"b":2}`),
	}

	results := registry.run(context.Background(), calls)

	if len(results) != 2 {
		t.Fatalf("results = %#v", results)
	}
	if results[0]["tool_call_id"] != "id-1" || results[0]["content"] != "one ran" {
		t.Errorf("first result = %#v", results[0])
	}
	if results[0]["role"] != "tool" || results[0]["name"] != "one" {
		t.Errorf("first result = %#v", results[0])
	}
	if results[1]["tool_call_id"] != "id-2" || results[1]["content"] != "error: no" {
		t.Errorf("second result = %#v", results[1])
	}
	close(seen)
	arguments := []string{}
	for got := range seen {
		arguments = append(arguments, got)
	}
	if len(arguments) != 2 {
		t.Errorf("arguments = %#v", arguments)
	}
}

func TestRegistryRunsCallsInParallelUpToTheLimit(t *testing.T) {
	var running, peak atomic.Int64
	var wg sync.WaitGroup
	wg.Add(maxParallelCalls)

	// Every call blocks until maxParallelCalls of them have started, so the
	// test only ends when the registry really runs them at the same time.
	tools := make([]Tool, 0, maxParallelCalls)
	calls := make([]map[string]any, 0, maxParallelCalls)
	for i := range maxParallelCalls {
		name := string(rune('a' + i))
		tools = append(tools, stubTool{name: name, start: func() {
			if now := running.Add(1); now > peak.Load() {
				peak.Store(now)
			}
			wg.Done()
			wg.Wait()
			running.Add(-1)
		}})
		calls = append(calls, call(i, "id", name, "{}"))
	}

	results := NewRegistry(tools...).run(context.Background(), calls)

	if len(results) != maxParallelCalls {
		t.Fatalf("results = %#v", results)
	}
	if peak.Load() != maxParallelCalls {
		t.Errorf("peak = %d, want %d", peak.Load(), maxParallelCalls)
	}
}
