package main

import (
	"bufio"
	"bytes"
	"encoding/json"
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
		FinishReason string `json:"finish_reason"`
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

	question := "How many r's are in the word 'strawberry'?"

	assistant := stream(apiKey, map[string]any{
		"model":     model,
		"messages":  []map[string]any{{"role": "user", "content": question}},
		"reasoning": map[string]any{"enabled": true},
		"stream":    true,
	})

	messages := []map[string]any{
		{"role": "user", "content": question},
		{
			"role":              "assistant",
			"content":           assistant.Content,
			"reasoning_details": assistant.ReasoningDetails,
		},
		{"role": "user", "content": "Are you sure? Think carefully."},
	}

	stream(apiKey, map[string]any{
		"model":     model,
		"messages":  messages,
		"reasoning": map[string]any{"enabled": true},
		"stream":    true,
	})
}

func stream(apiKey string, payload map[string]any) assistantMessage {
	body, err := json.Marshal(payload)
	if err != nil {
		log.Fatal(err)
	}

	req, err := http.NewRequest(http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		log.Fatalf("request failed: %s: %s", resp.Status, data)
	}

	var msg assistantMessage
	var content, reasoning strings.Builder
	inReasoning := false

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		// ": OPENROUTER PROCESSING" keep-alive comments
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		data, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			continue
		}
		if data == "[DONE]" {
			break
		}

		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			log.Fatalf("bad stream chunk: %v: %s", err, data)
		}
		if chunk.Error != nil {
			log.Fatalf("stream error: %v", chunk.Error)
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta

		if delta.Reasoning != "" {
			if !inReasoning {
				fmt.Print("\n--- reasoning ---\n")
				inReasoning = true
			}
			fmt.Print(delta.Reasoning)
			reasoning.WriteString(delta.Reasoning)
		}
		if delta.Content != "" {
			if inReasoning {
				fmt.Print("\n--- answer ---\n")
				inReasoning = false
			}
			fmt.Print(delta.Content)
			content.WriteString(delta.Content)
		}
		msg.ReasoningDetails = mergeReasoningDetails(msg.ReasoningDetails, delta.ReasoningDetails)
	}
	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}
	fmt.Println()

	msg.Content = content.String()
	return msg
}

// Streamed reasoning_details blocks arrive in fragments that must be reassembled
// in the model's original order before they can be replayed in a later turn.
func mergeReasoningDetails(acc, deltas []map[string]any) []map[string]any {
	for _, d := range deltas {
		idx, hasIdx := d["index"].(float64)
		target := -1
		if hasIdx {
			for i, existing := range acc {
				if e, ok := existing["index"].(float64); ok && e == idx {
					target = i
					break
				}
			}
		}
		if target < 0 {
			acc = append(acc, cloneDetail(d))
			continue
		}
		for k, v := range d {
			s, isStr := v.(string)
			prev, wasStr := acc[target][k].(string)
			if isStr && wasStr && (k == "text" || k == "summary" || k == "data") {
				acc[target][k] = prev + s
				continue
			}
			acc[target][k] = v
		}
	}
	return acc
}

func cloneDetail(d map[string]any) map[string]any {
	out := make(map[string]any, len(d))
	for k, v := range d {
		out[k] = v
	}
	return out
}
