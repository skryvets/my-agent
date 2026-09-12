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
	"github.com/skryvets/my-agent/internal/approval"
)

// Agent answers a conversation. *agent.Client satisfies it.
type Agent interface {
	Model() string
	Chat(ctx context.Context, history agent.History, stream io.Writer) (agent.Message, error)
}

// Approvals carries the questions of the tools to the person at the keyboard.
// *approval.Broker satisfies it.
type Approvals interface {
	Handle(ask approval.Ask)
}

// An Option changes the session before it reads the first question.
type Option func(*session)

// WithApproval makes the session ask before a tool call the policy does not
// allow by itself.
func WithApproval(approvals Approvals) Option {
	return func(s *session) { s.approvals = approvals }
}

type session struct {
	model     Agent
	approvals Approvals
	in        io.Reader
	out       io.Writer
	errOut    io.Writer
}

// Run reads questions from stdin until end of input and streams each answer to
// stdout. A failed turn is reported and dropped, leaving the session alive.
func Run(ctx context.Context, model Agent, options ...Option) error {
	chat := &session{model: model, in: os.Stdin, out: os.Stdout, errOut: os.Stderr}
	for _, option := range options {
		option(chat)
	}
	return chat.run(ctx)
}

func (s *session) run(ctx context.Context) error {
	var history agent.History
	input := bufio.NewScanner(s.in)
	fmt.Fprintln(s.out, "Chat with "+s.model.Model()+". Ctrl-C or Ctrl-D to quit.")

	if s.approvals != nil {
		s.approvals.Handle(s.askOn(input))
	}

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

// askOn asks on the same input the questions come from. Chat blocks the loop
// above while a tool waits, so only one of the two ever reads at a time.
func (s *session) askOn(input *bufio.Scanner) approval.Ask {
	return func(ctx context.Context, _ string, request approval.Request) (bool, error) {
		fmt.Fprintf(s.out, "\n--- may I run this? ---\n%s %s\n[y/N] ", request.Tool, request.Details)
		if !input.Scan() {
			return false, nil
		}
		answer := strings.ToLower(strings.TrimSpace(input.Text()))
		return answer == "y" || answer == "yes", nil
	}
}
