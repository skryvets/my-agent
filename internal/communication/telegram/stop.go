package telegram

import "context"

// stop answers /stop. It runs on the poll loop and not in the queue of the
// chat, because the queue waits behind the very message it has to stop.
func (b *Bot) stop(ctx context.Context, chatID int64) {
	b.mu.Lock()
	queue := b.sessions[chatID]
	cancel := b.working[chatID]
	b.mu.Unlock()

	dropped := drain(queue)
	if cancel == nil && !dropped {
		b.reply(ctx, chatID, "Nothing to stop.")
		return
	}
	if cancel != nil {
		cancel()
	}
	b.reply(ctx, chatID, "Stopped.")
}

// begin marks the chat as busy with one message until done is called, so
// /stop can end it.
func (b *Bot) begin(ctx context.Context, chatID int64) (context.Context, func()) {
	work, cancel := context.WithCancel(ctx)
	b.mu.Lock()
	b.working[chatID] = cancel
	b.mu.Unlock()

	return work, func() {
		b.mu.Lock()
		delete(b.working, chatID)
		b.mu.Unlock()
		cancel()
	}
}

// drain empties a queue without waiting and reports whether it held anything.
func drain(queue chan string) bool {
	dropped := false
	for {
		select {
		case <-queue:
			dropped = true
		default:
			return dropped
		}
	}
}
