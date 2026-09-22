// Package session is one conversation with one person: the history the model
// sees, and the commands that work the same way in every connector. A
// connector reads messages and sends replies; the session does the rest.
package session

import (
	"context"
	"io"
	"strings"

	"github.com/skryvets/my-agent/internal/agent"
	"github.com/skryvets/my-agent/internal/task"
)

// historyTurns caps how much of a conversation is replayed to the model.
// History lives in memory only, so a restart clears it.
const historyTurns = 10

// Agent answers a conversation. *agent.Client satisfies it.
type Agent interface {
	Model() string
	Chat(ctx context.Context, history agent.History, stream io.Writer) (string, error)
}

// Tasks does a coding job end to end and opens a pull request.
// *task.Runner satisfies it.
type Tasks interface {
	Start(ctx context.Context, chat, repository, instruction string, report task.Report) error
	Interrupted(ctx context.Context) []task.Run
}

// Session is one conversation. A connector makes one for each person it talks
// to, hands it every message, and sends out what Reply gets.
type Session struct {
	Agent Agent
	// Tasks runs /task. A nil Tasks answers /task with the reason it is off.
	Tasks Tasks
	// Key names this conversation in the runs it starts.
	Key string
	// Reply sends one text to the person.
	Reply func(text string)
	// Stream is where an answer appears while it arrives. A connector that
	// cannot show a partial answer leaves it nil and gets the whole answer
	// through Reply.
	Stream io.Writer

	history agent.History
}

// Handle answers one message. ctx ends when the person stops the work, and a
// stopped turn is not reported, because the stop was answered already.
func (s *Session) Handle(ctx context.Context, text string) {
	switch commandName(text) {
	case "/start", "/help":
		s.Reply(s.Help())
	case "/reset":
		s.history = nil
		s.Reply("Conversation cleared.")
	case "/stop":
		// A connector that can stop work answers /stop before it gets here.
		s.Reply("Nothing to stop.")
	case "/task":
		s.task(ctx, text)
	default:
		s.chat(ctx, text)
	}
}

// chat is one turn of the conversation. A turn the model failed to answer is
// dropped, so it never poisons the history.
func (s *Session) chat(ctx context.Context, text string) {
	s.history = s.history.WithUser(text)

	stream := s.Stream
	if stream == nil {
		stream = io.Discard
	}
	answer, err := s.Agent.Chat(ctx, s.history, stream)
	if err != nil {
		s.history = s.history.DropLast()
		if ctx.Err() == nil {
			s.Reply("That turn failed: " + err.Error())
		}
		return
	}
	if s.Stream == nil {
		s.Reply(answer)
	}
	s.history = s.history.WithAssistant(answer).Trim(historyTurns * 2)
}

// Help lists what the person can send.
func (s *Session) Help() string {
	help := "Send me a message and I will answer with " + s.Agent.Model() + ".\n\n"
	if s.Tasks != nil {
		help += "/task owner/name what to change - change a repository in its dev container and open a pull request\n"
	}
	return help +
		"/stop - stop what I am doing and drop the messages that wait\n" +
		"/reset - forget this conversation\n" +
		"/help - show this message"
}

func commandName(text string) string {
	name, _, _ := strings.Cut(strings.TrimSpace(text), " ")
	return name
}
