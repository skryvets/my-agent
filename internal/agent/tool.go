package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// maxParallelCalls caps how many calls of one round run at the same time.
const maxParallelCalls = 4

// A Tool is one action the model can ask for by name.
type Tool interface {
	Name() string
	Description() string
	// Parameters is the JSON Schema of the arguments object.
	Parameters() map[string]any
	Call(ctx context.Context, args json.RawMessage) (string, error)
}

// Registry is the set of tools one client offers, in the order it was given.
type Registry struct {
	order  []Tool
	byName map[string]Tool
}

// NewRegistry collects tools for a client. A name that is already taken is
// ignored, because the model addresses a tool by its name alone.
func NewRegistry(tools ...Tool) *Registry {
	registry := &Registry{byName: make(map[string]Tool, len(tools))}
	for _, tool := range tools {
		if _, taken := registry.byName[tool.Name()]; taken {
			continue
		}
		registry.byName[tool.Name()] = tool
		registry.order = append(registry.order, tool)
	}
	return registry
}

// schemas is the tools array the API expects, or nil when there is no tool.
func (r *Registry) schemas() []map[string]any {
	if r == nil || len(r.order) == 0 {
		return nil
	}
	schemas := make([]map[string]any, 0, len(r.order))
	for _, tool := range r.order {
		schemas = append(schemas, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        tool.Name(),
				"description": tool.Description(),
				"parameters":  tool.Parameters(),
			},
		})
	}
	return schemas
}

// run answers every call of one round and returns the tool turns in the order
// the model asked for them. Independent calls run at the same time.
func (r *Registry) run(ctx context.Context, calls []map[string]any) History {
	results := make(History, len(calls))
	slots := make(chan struct{}, maxParallelCalls)
	var wg sync.WaitGroup

	for i, call := range calls {
		wg.Add(1)
		go func() {
			defer wg.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			results[i] = toolResult(call, r.call(ctx, call))
		}()
	}
	wg.Wait()
	return results
}

// call runs one tool. A failure is reported to the model as text instead of to
// the caller, because the model can read the error and try something else.
func (r *Registry) call(ctx context.Context, call map[string]any) string {
	name, arguments := callFunction(call)
	tool, ok := r.byName[name]
	if !ok {
		return fmt.Sprintf("error: no tool named %q", name)
	}
	output, err := tool.Call(ctx, json.RawMessage(arguments))
	if err != nil {
		return "error: " + err.Error()
	}
	return output
}
