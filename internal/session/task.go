package session

import (
	"context"
	"strings"
	"unicode"

	"github.com/skryvets/my-agent/internal/task"
)

// task answers /task <owner/name> <what to do>. The conversation waits for the
// run, which takes minutes, and reads the progress as it arrives.
func (s *Session) task(ctx context.Context, text string) {
	if s.Tasks == nil {
		s.Reply("I cannot open pull requests. Start me with GITHUB_TOKEN set, on a machine with Docker.")
		return
	}

	rest := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(text), "/task"))
	repository, instruction := rest, ""
	if i := strings.IndexFunc(rest, unicode.IsSpace); i >= 0 {
		repository, instruction = rest[:i], strings.TrimSpace(rest[i:])
	}
	if repository == "" || instruction == "" {
		s.Reply("Write: /task owner/name what to change")
		return
	}

	if err := s.Tasks.Start(ctx, s.Key, repository, instruction, s.Reply); err != nil && ctx.Err() == nil {
		s.Reply("The task stopped: " + err.Error())
	}
}

// Lost says what a restart did to a run it caught in the middle, so a task
// never simply disappears.
func Lost(run task.Run) string {
	return "A restart stopped the task on " + run.Repo + " (" + run.Instruction +
		"). It reached the state \"" + string(run.State) + "\" on the branch " +
		run.Branch + ". Send it again to start over."
}
