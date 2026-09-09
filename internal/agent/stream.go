package agent

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content          string           `json:"content"`
			Reasoning        string           `json:"reasoning"`
			ReasoningDetails []map[string]any `json:"reasoning_details"`
		} `json:"delta"`
	} `json:"choices"`
	Error map[string]any `json:"error"`
}

func readStream(r io.Reader, out io.Writer) (Message, error) {
	var msg Message
	var content strings.Builder
	inReasoning := false

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		data, ok := strings.CutPrefix(scanner.Text(), "data: ")
		if !ok {
			continue
		}
		if data == "[DONE]" {
			break
		}

		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return msg, fmt.Errorf("bad stream chunk: %v: %s", err, data)
		}
		if chunk.Error != nil {
			return msg, fmt.Errorf("stream error: %v", chunk.Error)
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta

		if delta.Reasoning != "" {
			if !inReasoning {
				fmt.Fprint(out, "\n--- reasoning ---\n")
				inReasoning = true
			}
			fmt.Fprint(out, delta.Reasoning)
		}
		if delta.Content != "" {
			if inReasoning {
				fmt.Fprint(out, "\n--- answer ---\n")
				inReasoning = false
			}
			fmt.Fprint(out, delta.Content)
			content.WriteString(delta.Content)
		}
		msg.ReasoningDetails = mergeReasoningDetails(msg.ReasoningDetails, delta.ReasoningDetails)
	}
	if err := scanner.Err(); err != nil {
		return msg, err
	}
	fmt.Fprintln(out)

	msg.Content = content.String()
	if msg.Content == "" {
		return msg, errors.New("empty response from model")
	}
	return msg, nil
}
