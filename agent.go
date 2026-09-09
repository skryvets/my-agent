package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

const (
	apiURL = "https://openrouter.ai/api/v1/chat/completions"
	model  = "deepseek/deepseek-v4-flash-0731"
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

type assistantMessage struct {
	Content          string
	ReasoningDetails []map[string]any
}

func main() {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENROUTER_API_KEY is not set")
	}

	var messages []map[string]any
	input := bufio.NewScanner(os.Stdin)
	fmt.Println("Chat with " + model + ". Ctrl-C or Ctrl-D to quit.")

	for {
		fmt.Print("\nyou> ")
		if !input.Scan() {
			break
		}
		question := strings.TrimSpace(input.Text())
		if question == "" {
			continue
		}

		messages = append(messages, map[string]any{"role": "user", "content": question})

		assistant, err := chat(apiKey, messages, os.Stdout)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			messages = messages[:len(messages)-1]
			continue
		}

		messages = append(messages, map[string]any{
			"role":              "assistant",
			"content":           assistant.Content,
			"reasoning_details": assistant.ReasoningDetails,
		})
	}
	if err := input.Err(); err != nil {
		log.Fatal(err)
	}
}

func chat(apiKey string, messages []map[string]any, out io.Writer) (assistantMessage, error) {
	body, err := json.Marshal(map[string]any{
		"model":     model,
		"messages":  messages,
		"reasoning": map[string]any{"enabled": true},
		"stream":    true,
	})
	if err != nil {
		return assistantMessage{}, err
	}

	req, err := http.NewRequest(http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return assistantMessage{}, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return assistantMessage{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return assistantMessage{}, fmt.Errorf("request failed: %s: %s", resp.Status, data)
	}

	return readStream(resp.Body, out)
}

func readStream(r io.Reader, out io.Writer) (assistantMessage, error) {
	var msg assistantMessage
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
