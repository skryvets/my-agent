package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	apiURL      = "https://api.telegram.org"
	pollSeconds = 30
	maxMessage  = 4096
)

// client is the slice of the Bot API this bot uses.
type client struct {
	token   string
	baseURL string
	http    *http.Client
}

func newClient(token string) *client {
	return &client{
		token:   token,
		baseURL: apiURL,
		// The poll blocks for pollSeconds, so the client must outwait it.
		http: &http.Client{Timeout: (pollSeconds + 15) * time.Second},
	}
}

type response struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	Description string          `json:"description"`
	ErrorCode   int             `json:"error_code"`
	Parameters  struct {
		RetryAfter int `json:"retry_after"`
	} `json:"parameters"`
}

// Error is a Bot API call the server rejected.
type Error struct {
	Method      string
	Code        int
	Description string
	RetryAfter  int
}

func (e *Error) Error() string {
	return fmt.Sprintf("telegram %s failed: %d %s", e.Method, e.Code, e.Description)
}

type update struct {
	UpdateID int64    `json:"update_id"`
	Message  *message `json:"message"`
}

type message struct {
	MessageID int64 `json:"message_id"`
	From      struct {
		ID       int64  `json:"id"`
		Username string `json:"username"`
	} `json:"from"`
	Chat struct {
		ID   int64  `json:"id"`
		Type string `json:"type"`
	} `json:"chat"`
	Text string `json:"text"`
}

func (c *client) call(ctx context.Context, method string, payload, result any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/bot%s/%s", c.baseURL, c.token, method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("telegram %s: %w", method, err)
	}
	defer resp.Body.Close()

	var envelope response
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return fmt.Errorf("telegram %s: bad response: %w", method, err)
	}
	if !envelope.OK {
		return &Error{
			Method:      method,
			Code:        envelope.ErrorCode,
			Description: envelope.Description,
			RetryAfter:  envelope.Parameters.RetryAfter,
		}
	}
	if result == nil {
		return nil
	}
	return json.Unmarshal(envelope.Result, result)
}

func (c *client) getUpdates(ctx context.Context, offset int64) ([]update, error) {
	payload := map[string]any{
		"timeout":         pollSeconds,
		"allowed_updates": []string{"message"},
	}
	if offset > 0 {
		payload["offset"] = offset
	}

	var updates []update
	if err := c.call(ctx, "getUpdates", payload, &updates); err != nil {
		return nil, err
	}
	return updates, nil
}

func (c *client) sendMessage(ctx context.Context, chatID int64, text string) error {
	for _, part := range splitMessage(text, maxMessage) {
		payload := map[string]any{"chat_id": chatID, "text": part}
		if err := c.call(ctx, "sendMessage", payload, nil); err != nil {
			return err
		}
	}
	return nil
}

func (c *client) sendChatAction(ctx context.Context, chatID int64, action string) error {
	return c.call(ctx, "sendChatAction", map[string]any{"chat_id": chatID, "action": action}, nil)
}

// Telegram rejects a sendMessage over 4096 characters, so long answers are cut
// on the last blank line, newline or space that still fits.
func splitMessage(text string, limit int) []string {
	var parts []string
	for {
		runes := []rune(text)
		if len(runes) <= limit {
			if len(parts) == 0 || strings.TrimSpace(text) != "" {
				parts = append(parts, text)
			}
			return parts
		}

		head := string(runes[:limit])
		cut := limit
		for _, separator := range []string{"\n\n", "\n", " "} {
			if index := strings.LastIndex(head, separator); index > 0 {
				cut = len([]rune(head[:index]))
				break
			}
		}
		parts = append(parts, strings.TrimRight(string(runes[:cut]), " \n"))
		text = strings.TrimLeft(string(runes[cut:]), " \n")
	}
}
