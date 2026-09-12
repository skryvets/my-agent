package telegram

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"
)

// Bot long-polls getUpdates and hands each message to the session for its chat.
type Bot struct {
	client    *client
	agent     Agent
	sandbox   Sandbox
	tasks     Tasks
	allowed   map[int64]bool
	retryBase time.Duration

	mu       sync.Mutex
	sessions map[int64]chan string
	waiting  map[string]chan bool
}

// Only one instance may poll getUpdates at a time, so the bot runs as a single
// replica and the loop is not concurrent.
func (b *Bot) run(ctx context.Context) error {
	log.Printf("telegram bot polling for updates, answering with %s", b.agent.Model())

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

		for _, u := range updates {
			if u.UpdateID >= offset {
				offset = u.UpdateID + 1
			}
			if u.CallbackQuery != nil {
				b.answer(ctx, u.CallbackQuery)
				continue
			}
			b.dispatch(ctx, u)
		}
	}
	return ctx.Err()
}

// A 429 names how long to wait, anything else backs off.
func retryDelay(err error, backoff time.Duration) time.Duration {
	var apiErr *Error
	if errors.As(err, &apiErr) && apiErr.RetryAfter > 0 {
		return time.Duration(apiErr.RetryAfter) * time.Second
	}
	return backoff
}

// sleep reports whether the wait finished rather than the context being cancelled.
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
