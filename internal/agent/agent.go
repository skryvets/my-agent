// Package agent talks to a reasoning model through the OpenRouter chat
// completions API and streams the reply as it arrives.
package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

const apiURL = "https://openrouter.ai/api/v1/chat/completions"

// Model is the OpenRouter model slug every client uses unless told otherwise.
var Model = os.Getenv("MY_AGENT_MODEL")

// Message is one assistant turn, with the reasoning blocks that produced it.
type Message struct {
	Content          string
	ReasoningDetails []map[string]any

	// ToolCalls is set instead of Content when the model asks for a tool.
	ToolCalls []map[string]any

	// Steps are the tool calls and tool results that came before Content.
	// WithAssistant puts them back into the history in front of the answer.
	Steps History
}

// Client is an OpenRouter chat completions client bound to one model.
type Client struct {
	apiKey string
	model  string
	apiURL string
	http   *http.Client
	tools  *Registry
}

// New returns a client for Model that offers the given tools.
func New(apiKey string, tools ...Tool) *Client {
	return &Client{
		apiKey: apiKey,
		model:  Model,
		apiURL: apiURL,
		http:   http.DefaultClient,
		tools:  NewRegistry(tools...),
	}
}

// Model reports the model slug answers come from.
func (c *Client) Model() string { return c.model }

// complete is one request and one streamed reply.
func (c *Client) complete(ctx context.Context, history History, stream io.Writer) (Message, error) {
	// Reasoning is off on purpose. The stream and history still carry
	// reasoning_details so switching it back on needs no other change.
	payload := map[string]any{
		"model":     c.model,
		"messages":  []map[string]any(history),
		"reasoning": map[string]any{"enabled": false},
		"stream":    true,
	}
	if schemas := c.tools.schemas(); schemas != nil {
		payload["tools"] = schemas
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Message{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL, bytes.NewReader(body))
	if err != nil {
		return Message{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.http.Do(req)
	if err != nil {
		return Message{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return Message{}, fmt.Errorf("request failed: %s: %s", resp.Status, data)
	}
	return readStream(resp.Body, stream)
}
