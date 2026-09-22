// Package terminal chats with the agent over stdin and stdout, in one session.
package terminal

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/skryvets/my-agent/internal/session"
)

// key names the terminal in the runs it starts. There is one terminal, so it
// never has to tell itself apart from another.
const key = "terminal"

// Run reads messages from stdin until end of input and streams each answer to
// stdout. A nil tasks answers /task with the reason it is off.
func Run(ctx context.Context, model session.Agent, tasks session.Tasks) error {
	return run(ctx, model, tasks, os.Stdin, os.Stdout)
}

func run(ctx context.Context, model session.Agent, tasks session.Tasks, in io.Reader, out io.Writer) error {
	chat := &session.Session{
		Agent:  model,
		Tasks:  tasks,
		Key:    key,
		Reply:  func(text string) { fmt.Fprintln(out, text) },
		Stream: out,
	}
	fmt.Fprintln(out, "Chat with "+model.Model()+". /help lists the commands. Ctrl-C or Ctrl-D to quit.")
	reportInterrupted(ctx, chat)

	input := bufio.NewScanner(in)
	for {
		fmt.Fprint(out, "\nyou> ")
		if !input.Scan() {
			break
		}
		text := strings.TrimSpace(input.Text())
		if text == "" {
			continue
		}
		chat.Handle(ctx, text)
	}
	return input.Err()
}

// reportInterrupted tells the terminal about the run of its own that a
// restart caught in the middle.
func reportInterrupted(ctx context.Context, chat *session.Session) {
	if chat.Tasks == nil {
		return
	}
	for _, run := range chat.Tasks.Interrupted(ctx) {
		if run.Chat == key {
			chat.Reply(session.Lost(run))
		}
	}
}
