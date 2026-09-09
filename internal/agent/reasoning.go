package agent

// Streamed reasoning_details blocks arrive in fragments that must be reassembled
// in the model's original order before they can be replayed in a later turn.
func mergeReasoningDetails(details, fragments []map[string]any) []map[string]any {
	for _, fragment := range fragments {
		var target map[string]any
		index, hasIndex := fragment["index"].(float64)
		if hasIndex {
			for _, detail := range details {
				if existingIndex, ok := detail["index"].(float64); ok && existingIndex == index {
					target = detail
					break
				}
			}
		}
		if target == nil {
			details = append(details, cloneDetail(fragment))
			continue
		}
		for field, value := range fragment {
			switch field {
			case "text", "summary", "data":
				previous, previousIsString := target[field].(string)
				next, nextIsString := value.(string)
				if previousIsString && nextIsString {
					value = previous + next
				}
			}
			target[field] = value
		}
	}
	return details
}

func cloneDetail(detail map[string]any) map[string]any {
	clone := make(map[string]any, len(detail))
	for field, value := range detail {
		clone[field] = value
	}
	return clone
}
