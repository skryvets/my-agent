package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	telegramAPI          = "https://api.telegram.org"
	telegramPollSeconds  = 30
	telegramMaxMessage   = 4096
	telegramHistoryTurns = 10
	telegramQueueSize    = 4
)

const telegramHelp = "Send me a message and I will answer with " + model + ".\n\n" +
	"/reset - forget this conversation\n" +
	"/help - show this message"

type telegramClient struct {
	token   string
	baseURL string
	http    *http.Client
}

func newTelegramClient(token string) *telegramClient {
	return &telegramClient{
		token:   token,
		baseURL: telegramAPI,
		// The poll blocks for telegramPollSeconds, so the client must outwait it.
		http: &http.Client{Timeout: (telegramPollSeconds + 15) * time.Second},
	}
}

type telegramResponse struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	Description string          `json:"description"`
	ErrorCode   int             `json:"error_code"`
	Parameters  struct {
		RetryAfter int `json:"retry_after"`
	} `json:"parameters"`
}

type telegramError struct {
	Method      string
	Code        int
	Description string
	RetryAfter  int
}

func (e *telegramError) Error() string {
	return fmt.Sprintf("telegram %s failed: %d %s", e.Method, e.Code, e.Description)
}

type telegramUpdate struct {
	UpdateID int64            `json:"update_id"`
	Message  *telegramMessage `json:"message"`
}

type telegramMessage struct {
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

func (c *telegramClient) call(ctx context.Context, method string, payload, result any) error {
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

	var envelope telegramResponse
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return fmt.Errorf("telegram %s: bad response: %w", method, err)
	}
	if !envelope.OK {
		return &telegramError{
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

func (c *telegramClient) getUpdates(ctx context.Context, offset int64) ([]telegramUpdate, error) {
	payload := map[string]any{
		"timeout":         telegramPollSeconds,
		"allowed_updates": []string{"message"},
	}
	if offset > 0 {
		payload["offset"] = offset
	}

	var updates []telegramUpdate
	if err := c.call(ctx, "getUpdates", payload, &updates); err != nil {
		return nil, err
	}
	return updates, nil
}

func (c *telegramClient) sendMessage(ctx context.Context, chatID int64, text string) error {
	for _, part := range splitMessage(text, telegramMaxMessage) {
		payload := map[string]any{"chat_id": chatID, "text": part}
		if err := c.call(ctx, "sendMessage", payload, nil); err != nil {
			return err
		}
	}
	return nil
}

func (c *telegramClient) sendChatAction(ctx context.Context, chatID int64, action string) error {
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

type telegramBot struct {
	client    *telegramClient
	answer    func(messages []map[string]any) (assistantMessage, error)
	allowed   map[int64]bool
	retryBase time.Duration

	mu       sync.Mutex
	sessions map[int64]chan string
}

func runTelegram(apiKey string) error {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		return errors.New("TELEGRAM_BOT_TOKEN is not set")
	}
	allowed, err := parseAllowedUsers(os.Getenv("TELEGRAM_ALLOWED_USERS"))
	if err != nil {
		return err
	}
	if len(allowed) == 0 {
		log.Print("warning: TELEGRAM_ALLOWED_USERS is empty, anyone who finds the bot can spend your OpenRouter credits")
	}

	bot := &telegramBot{
		client: newTelegramClient(token),
		answer: func(messages []map[string]any) (assistantMessage, error) {
			return chat(apiKey, messages, io.Discard)
		},
		allowed:   allowed,
		retryBase: time.Second,
		sessions:  map[int64]chan string{},
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := bot.run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

func parseAllowedUsers(value string) (map[int64]bool, error) {
	allowed := map[int64]bool{}
	for _, field := range strings.Split(value, ",") {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		id, err := strconv.ParseInt(field, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("TELEGRAM_ALLOWED_USERS: %q is not a user id", field)
		}
		allowed[id] = true
	}
	return allowed, nil
}

func (b *telegramBot) run(ctx context.Context) error {
	log.Printf("telegram bot polling for updates, answering with %s", model)

	var offset int64
	backoff := b.retryBase
	for ctx.Err() == nil {
		updates, err := b.client.getUpdates(ctx, offset)
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			log.Printf("getUpdates: %v", err)
			if !sleep(ctx, retryDelay(err, backoff)) {
				break
			}
			backoff = min(backoff*2, time.Minute)
			continue
		}
		backoff = b.retryBase

		for _, update := range updates {
			if update.UpdateID >= offset {
				offset = update.UpdateID + 1
			}
			b.dispatch(ctx, update)
		}
	}
	return ctx.Err()
}

func retryDelay(err error, backoff time.Duration) time.Duration {
	var apiErr *telegramError
	if errors.As(err, &apiErr) && apiErr.RetryAfter > 0 {
		return time.Duration(apiErr.RetryAfter) * time.Second
	}
	return backoff
}

func sleep(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (b *telegramBot) dispatch(ctx context.Context, update telegramUpdate) {
	message := update.Message
	if message == nil || strings.TrimSpace(message.Text) == "" {
		return
	}
	if len(b.allowed) > 0 && !b.allowed[message.From.ID] {
		log.Printf("ignoring message from user %d (@%s)", message.From.ID, message.From.Username)
		return
	}

	queue := b.session(ctx, message.Chat.ID)
	select {
	case queue <- message.Text:
	default:
		if err := b.client.sendMessage(ctx, message.Chat.ID, "I am still working on your previous messages, try again in a moment."); err != nil {
			log.Printf("sendMessage: %v", err)
		}
	}
}

func (b *telegramBot) session(ctx context.Context, chatID int64) chan string {
	b.mu.Lock()
	defer b.mu.Unlock()

	if queue, ok := b.sessions[chatID]; ok {
		return queue
	}
	queue := make(chan string, telegramQueueSize)
	b.sessions[chatID] = queue
	go b.serve(ctx, chatID, queue)
	return queue
}

func (b *telegramBot) serve(ctx context.Context, chatID int64, queue <-chan string) {
	var history []map[string]any

	for {
		var text string
		select {
		case <-ctx.Done():
			return
		case text = <-queue:
		}

		command, _, _ := strings.Cut(strings.TrimSpace(text), " ")
		switch command {
		case "/start", "/help":
			b.reply(ctx, chatID, telegramHelp)
			continue
		case "/reset":
			history = nil
			b.reply(ctx, chatID, "Conversation cleared.")
			continue
		}

		history = append(history, map[string]any{"role": "user", "content": text})
		if err := b.client.sendChatAction(ctx, chatID, "typing"); err != nil {
			log.Printf("sendChatAction: %v", err)
		}

		assistant, err := b.answer(history)
		if err != nil {
			log.Printf("chat: %v", err)
			history = history[:len(history)-1]
			b.reply(ctx, chatID, "That turn failed: "+err.Error())
			continue
		}

		history = append(history, map[string]any{
			"role":              "assistant",
			"content":           assistant.Content,
			"reasoning_details": assistant.ReasoningDetails,
		})
		history = trimHistory(history, telegramHistoryTurns*2)
		b.reply(ctx, chatID, assistant.Content)
	}
}

func (b *telegramBot) reply(ctx context.Context, chatID int64, text string) {
	if err := b.client.sendMessage(ctx, chatID, text); err != nil {
		log.Printf("sendMessage: %v", err)
	}
}

// Whole turns are dropped so the history never starts on an assistant reply.
func trimHistory(history []map[string]any, limit int) []map[string]any {
	if len(history) <= limit {
		return history
	}
	drop := len(history) - limit
	if drop%2 != 0 {
		drop++
	}
	if drop >= len(history) {
		return nil
	}
	return history[drop:]
}
