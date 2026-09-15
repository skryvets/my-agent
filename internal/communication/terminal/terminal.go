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

type session struct {
	model  Agent
	in     io.Reader
	out    io.Writer
	errOut io.Writer
}

// Run reads questions from stdin until end of input and streams each answer to
// stdout. A failed turn is reported and dropped, leaving the session alive.
func Run(ctx context.Context, model Agent) error {
	chat := &session{model: model, in: os.Stdin, out: os.Stdout, errOut: os.Stderr}
	return chat.run(ctx)
}

func (s *session) run(ctx context.Context) error {
	var history agent.History
	input := bufio.NewScanner(s.in)
	fmt.Fprintln(s.out, "Chat with "+s.model.Model()+". Ctrl-C or Ctrl-D to quit.")

	for {
		fmt.Fprint(s.out, "\nyou> ")
		if !input.Scan() {
			break
		}
		question := strings.TrimSpace(input.Text())
		if question == "" {
			continue
		}

		history = history.WithUser(question)
		assistant, err := s.model.Chat(ctx, history, s.out)
		if err != nil {
			fmt.Fprintf(s.errOut, "error: %v\n", err)
			history = history.DropLast()
			continue
		}
		history = history.WithAssistant(assistant)
	}
	return input.Err()
}
