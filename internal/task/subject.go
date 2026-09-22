package task

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/skryvets/my-agent/internal/agent"
)

const (
	// subjectLimit is the width a commit subject reads well at.
	subjectLimit = 72

	// summaryLimit keeps the body of a pull request short enough to read.
	summaryLimit = 2000
)

// subjectFor asks the model to name its own change. Asking in the turn that
// did the work gives a subject buried in prose, so it is a question of its
// own, to the agent with no tools. A model that answers in another shape
// costs nothing: the instruction is used instead.
func (r *Runner) subjectFor(ctx context.Context, run *Run, summary string) string {
	question := fmt.Sprintf(
		"Write the commit subject for this change. Use the imperative, "+
			"keep it under %d characters, and end it without a full stop. "+
			"Answer with that one line and nothing else.\n\nWhat you did:\n\n%s",
		subjectLimit, summary)

	answer, err := r.Plain.Chat(ctx, agent.History(nil).WithUser(question), io.Discard)
	if err != nil {
		return subjectOf(run.Instruction)
	}
	if subject := subjectLine(answer); subject != "" {
		return subject
	}
	return subjectOf(run.Instruction)
}

// subjectLine reads one commit subject, or nothing when the answer is not one.
func subjectLine(answer string) string {
	answer = strings.TrimSpace(answer)
	if strings.Contains(answer, "\n") {
		return ""
	}
	subject := strings.Trim(answer, "`\"")
	if subject == "" || len(subject) > subjectLimit || strings.HasSuffix(subject, ":") {
		return ""
	}
	return subject
}

// subjectOf falls back to the instruction when the model answered in another
// shape.
func subjectOf(instruction string) string {
	subject, _, _ := strings.Cut(strings.TrimSpace(instruction), "\n")
	subject = strings.TrimSpace(subject)
	if len(subject) > subjectLimit {
		subject = strings.TrimSpace(subject[:subjectLimit-3]) + "..."
	}
	if subject == "" {
		return "Change asked for from the phone"
	}
	return subject
}

func body(instruction, summary string) string {
	if len(summary) > summaryLimit {
		summary = summary[:summaryLimit] + "..."
	}
	return fmt.Sprintf("Asked for:\n\n%s\n\n---\n\n%s\n\nOpened by my-agent.", instruction, summary)
}
