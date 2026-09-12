package agent

// Streamed tool_calls arrive in the same shape as reasoning_details: indexed
// fragments, where the first fragment carries the id and the tool name and the
// later ones carry more of the arguments string.
func mergeToolCalls(calls, fragments []map[string]any) []map[string]any {
	for _, fragment := range fragments {
		var target map[string]any
		index, hasIndex := fragment["index"].(float64)
		if hasIndex {
			for _, call := range calls {
				if existingIndex, ok := call["index"].(float64); ok && existingIndex == index {
					target = call
					break
				}
			}
		}
		if target == nil {
			calls = append(calls, cloneCall(fragment))
			continue
		}
		mergeCall(target, fragment)
	}
	return calls
}

func mergeCall(target, fragment map[string]any) {
	for field, value := range fragment {
		if field != "function" {
			// A later fragment repeats id and type as empty strings on some
			// providers, which must not erase what the first one carried.
			if text, isString := value.(string); isString && text == "" {
				continue
			}
			target[field] = value
			continue
		}
		next, _ := value.(map[string]any)
		previous, _ := target["function"].(map[string]any)
		if previous == nil {
			target["function"] = cloneDetail(next)
			continue
		}
		for key, part := range next {
			if key == "arguments" {
				soFar, hadString := previous["arguments"].(string)
				more, isString := part.(string)
				if hadString && isString {
					previous["arguments"] = soFar + more
					continue
				}
			}
			previous[key] = part
		}
	}
}

func cloneCall(fragment map[string]any) map[string]any {
	clone := cloneDetail(fragment)
	if function, ok := clone["function"].(map[string]any); ok {
		clone["function"] = cloneDetail(function)
	}
	return clone
}

// callFunction reads the tool name and the arguments of one call.
func callFunction(call map[string]any) (name, arguments string) {
	function, _ := call["function"].(map[string]any)
	name, _ = function["name"].(string)
	arguments, _ = function["arguments"].(string)
	if arguments == "" {
		arguments = "{}"
	}
	return name, arguments
}

// wireCalls drops the streaming index, so the assistant turn that goes back to
// the API carries only the fields the API defines.
func wireCalls(calls []map[string]any) []map[string]any {
	wire := make([]map[string]any, 0, len(calls))
	for _, call := range calls {
		name, arguments := callFunction(call)
		id, _ := call["id"].(string)
		wire = append(wire, map[string]any{
			"id":       id,
			"type":     "function",
			"function": map[string]any{"name": name, "arguments": arguments},
		})
	}
	return wire
}

// toolResult is the turn that answers one call.
func toolResult(call map[string]any, content string) map[string]any {
	name, _ := callFunction(call)
	id, _ := call["id"].(string)
	return map[string]any{
		"role":         "tool",
		"tool_call_id": id,
		"name":         name,
		"content":      content,
	}
}
