package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	apiURL = "https://api.telegram.org"

	// pollSeconds is how long getUpdates blocks waiting for a message.
	pollSeconds = 30
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

// call posts payload to one Bot API method and unmarshals the result, turning a
// rejected call into an *Error.
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
		"allowed_updates": []string{"message", "callback_query"},
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

// sendKeyboard sends one message with buttons under it and returns the id of
// the message, so the buttons can be replaced by the decision later.
func (c *client) sendKeyboard(ctx context.Context, chatID int64, text string, buttons [][]button) (int64, error) {
	payload := map[string]any{
		"chat_id":      chatID,
		"text":         text,
		"reply_markup": map[string]any{"inline_keyboard": buttons},
	}

	var sent struct {
		MessageID int64 `json:"message_id"`
	}
	if err := c.call(ctx, "sendMessage", payload, &sent); err != nil {
		return 0, err
	}
	return sent.MessageID, nil
}

// answerCallback stops the clock on the button a person pressed. Telegram
// keeps showing it as pressed until this call arrives.
func (c *client) answerCallback(ctx context.Context, queryID, text string) error {
	payload := map[string]any{"callback_query_id": queryID, "text": text}
	return c.call(ctx, "answerCallbackQuery", payload, nil)
}

// editMessage replaces the text of a message and drops the buttons with it,
// so a question cannot be answered twice.
func (c *client) editMessage(ctx context.Context, chatID, messageID int64, text string) error {
	payload := map[string]any{"chat_id": chatID, "message_id": messageID, "text": text}
	return c.call(ctx, "editMessageText", payload, nil)
}

func (c *client) sendChatAction(ctx context.Context, chatID int64, action string) error {
	return c.call(ctx, "sendChatAction", map[string]any{"chat_id": chatID, "action": action}, nil)
}
