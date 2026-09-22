// Package terminal chats with the agent over stdin and stdout.
package terminal

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ergochat/readline"

	"github.com/skryvets/my-agent/internal/agent"
)

// prompt is shown before every question.
const prompt = "\nyou> "

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
	if !isTerminal(os.Stdin) {
		return chat.run(ctx)
	}
	editor, err := readline.NewFromConfig(&readline.Config{Prompt: prompt})
	if err != nil {
		return err
	}
	defer editor.Close()
	return chat.runInteractive(ctx, editor)
}

// isTerminal reports whether the file is a terminal. A pipe or a file keeps
// the plain line reader, which is also how the tests drive the session.
func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// runInteractive reads with line editing and history while the input is a
// terminal. Ctrl-C and end of input end the session. The answer streams
// through the editor, which redraws the prompt around it.
func (s *session) runInteractive(ctx context.Context, editor *readline.Instance) error {
	out, errOut := editor.Stdout(), editor.Stderr()
	fmt.Fprintln(out, banner(s.model.Model()))

	var history agent.History
	for {
		line, err := editor.ReadLine()
		switch {
		case errors.Is(err, io.EOF), errors.Is(err, readline.ErrInterrupt):
			return nil
		case err != nil:
			return err
		}
		question := strings.TrimSpace(line)
		if question == "" {
			continue
		}
		history = s.turn(ctx, out, errOut, history, question)
	}
}

// run reads line by line, for input that is not a terminal.
func (s *session) run(ctx context.Context) error {
	var history agent.History
	input := bufio.NewScanner(s.in)
	fmt.Fprintln(s.out, banner(s.model.Model()))

	for {
		fmt.Fprint(s.out, prompt)
		if !input.Scan() {
			break
		}
		question := strings.TrimSpace(input.Text())
		if question == "" {
			continue
		}
		history = s.turn(ctx, s.out, s.errOut, history, question)
	}
	return input.Err()
}

// turn answers one question and returns the history after it.
func (s *session) turn(ctx context.Context, out, errOut io.Writer, history agent.History, question string) agent.History {
	history = history.WithUser(question)
	assistant, err := s.model.Chat(ctx, history, out)
	if err != nil {
		fmt.Fprintf(errOut, "error: %v\n", err)
		return history.DropLast()
	}
	return history.WithAssistant(assistant)
}

func banner(model string) string {
	return "Chat with " + model + ". Ctrl-C or Ctrl-D to quit."
}
