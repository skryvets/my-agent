package telegram

import (
	"encoding/json"
	"fmt"
)

// response is the envelope every Bot API method replies with.
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
	UpdateID      int64          `json:"update_id"`
	Message       *message       `json:"message"`
	CallbackQuery *callbackQuery `json:"callback_query"`
}

// callbackQuery is a press of one button under a message the bot sent.
type callbackQuery struct {
	ID   string `json:"id"`
	Data string `json:"data"`
	From struct {
		ID       int64  `json:"id"`
		Username string `json:"username"`
	} `json:"from"`
	Message *message `json:"message"`
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

// button is one key of an inline keyboard. callback_data is capped at 64
// bytes by the Bot API, so it carries an id and nothing else.
type button struct {
	Text string `json:"text"`
	Data string `json:"callback_data"`
}
