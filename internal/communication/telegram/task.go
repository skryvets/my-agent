package telegram

import (
	"context"
	"log"
	"strconv"
	"strings"
)

// task answers /task <owner/name> <what to do>. The chat waits for the run,
// which takes minutes, and reads the progress as it arrives.
// ctx carries the replies, and work is the run itself, which /stop ends.
func (b *Bot) task(ctx, work context.Context, chatID int64, text string) {
	if b.tasks == nil {
		b.reply(ctx, chatID, "I cannot open pull requests. Start me with GITHUB_TOKEN set, on a machine with Docker.")
		return
	}

	rest := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(text), "/task"))
	repository, instruction, _ := strings.Cut(rest, " ")
	if repository == "" || strings.TrimSpace(instruction) == "" {
		b.reply(ctx, chatID, "Write: /task owner/name what to change")
		return
	}

	report := func(line string) { b.reply(ctx, chatID, line) }
	if err := b.tasks.Start(work, chatKey(chatID), repository, instruction, report); err != nil {
		if work.Err() != nil {
			return
		}
		log.Printf("task in chat %d: %v", chatID, err)
		b.reply(ctx, chatID, "The task stopped: "+err.Error())
	}
}

// reportInterrupted tells each chat about the run a restart caught in the
// middle, so a task never simply disappears.
func (b *Bot) reportInterrupted(ctx context.Context) {
	if b.tasks == nil {
		return
	}
	for _, run := range b.tasks.Interrupted(ctx) {
		chatID, err := strconv.ParseInt(run.Chat, 10, 64)
		if err != nil {
			continue
		}
		b.reply(ctx, chatID, "A restart stopped the task on "+run.Repo+
			" ("+run.Instruction+"). It reached the state \""+string(run.State)+
			"\" on the branch "+run.Branch+". Send it again to start over.")
	}
}
