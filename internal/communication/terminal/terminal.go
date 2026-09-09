// Package terminal chats with the agent over stdin and stdout.
package terminal

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/skryvets/my-agent/internal/agent"
)

// Agent answers a conversation. *agent.Client satisfies it.
type Agent interface {
	Model() string
	Chat(ctx context.Context, history agent.History, stream io.Writer) (agent.Message, error)
}

// Run reads questions from stdin until end of input and streams each answer to
// stdout. A failed turn is reported and dropped, leaving the session alive.
func Run(ctx context.Context, model Agent) error {
	return run(ctx, model, os.Stdin, os.Stdout, os.Stderr)
}

func run(ctx context.Context, model Agent, in io.Reader, out, errOut io.Writer) error {
	var history agent.History
	input := bufio.NewScanner(in)
	fmt.Fprintln(out, "Chat with "+model.Model()+". Ctrl-C or Ctrl-D to quit.")

	for {
		fmt.Fprint(out, "\nyou> ")
		if !input.Scan() {
			break
		}
		question := strings.TrimSpace(input.Text())
		if question == "" {
			continue
		}

		history = history.WithUser(question)

		assistant, err := model.Chat(ctx, history, out)
		if err != nil {
			fmt.Fprintf(errOut, "error: %v\n", err)
			history = history.DropLast()
			continue
		}
		history = history.WithAssistant(assistant)
	}
	return input.Err()
}
