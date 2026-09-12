package task

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/skryvets/my-agent/internal/agent"
	"github.com/skryvets/my-agent/internal/conversation"
)

const (
	// subjectLimit is the width a commit subject reads well at.
	subjectLimit = 72

	// summaryLimit keeps the body of a pull request short enough to read.
	summaryLimit = 2000
)

// work is the run itself, one stage after the other. Each stage reports what
// it is doing, because the whole thing takes minutes.
func (r *Runner) work(ctx context.Context, run *Run, repo Repo, dir string, report Report) error {
	git := Git{Dir: dir, Token: r.Token, Host: r.GitHost}

	report(fmt.Sprintf("Cloning %s", repo))
	if err := git.Clone(ctx, repo, run.Branch); err != nil {
		return err
	}

	// The container works on the checkout through a bind, so it needs no
	// network and never sees the token.
	key := "task-" + run.ID
	if err := r.Sandbox.Bind(ctx, key, dir); err != nil {
		return err
	}
	defer r.Sandbox.Close(context.WithoutCancel(ctx), key)

	run.State = Working
	r.save(*run)
	report("Working on it")

	inside := conversation.WithKey(ctx, key)
	summary, err := r.think(inside, repo, run)
	if err != nil {
		return err
	}
	report(summary)

	changed, err := git.Changed(ctx)
	if err != nil {
		return err
	}
	if !changed {
		run.State = NoChange
		run.Detail = summary
		run.Ended = time.Now().UTC()
		r.save(*run)
		report("Nothing changed, so there is no pull request.")
		return nil
	}

	run.State = Pushing
	r.save(*run)
	report("Pushing " + run.Branch)

	subject := r.subjectFor(ctx, run, summary)
	if err := git.Commit(ctx, subject, run.Instruction); err != nil {
		return err
	}
	if err := git.Push(ctx, run.Branch); err != nil {
		return err
	}

	base, err := r.GitHub.DefaultBranch(ctx, repo)
	if err != nil {
		return err
	}
	url, err := r.GitHub.OpenPullRequest(ctx, repo, subject, run.Branch, base, body(run.Instruction, summary))
	if err != nil {
		return err
	}

	run.State = Opened
	run.Detail = url
	run.Ended = time.Now().UTC()
	r.save(*run)
	report("Pull request open: " + url)
	return nil
}

// think lets the model do the work inside the container. It is told not to
// touch git, because the branch, the commit and the pull request are made
// with a token it must never see.
func (r *Runner) think(ctx context.Context, repo Repo, run *Run) (string, error) {
	instruction := fmt.Sprintf(
		"You are working in /work, a checkout of %s on the branch %s.\n\n"+
			"Do this: %s\n\n"+
			"Look around with the shell first. Change the files you need to change, "+
			"and run the tests of the project before you finish. "+
			"Do not run git: the branch, the commit and the pull request are made for you. "+
			"When you are done, answer with a short summary of what you changed and why.",
		repo, run.Branch, run.Instruction)

	answer, err := r.Agent.Chat(ctx, agent.History(nil).WithUser(instruction), io.Discard)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(answer.Content), nil
}

// subjectFor asks the model to name its own change. Asking in the turn that
// did the work gives a subject buried in prose, so it is a question of its
// own. A model that answers in another shape costs nothing: the instruction
// is used instead.
func (r *Runner) subjectFor(ctx context.Context, run *Run, summary string) string {
	question := fmt.Sprintf(
		"Write the commit subject for this change. Use the imperative, "+
			"keep it under %d characters, and end it without a full stop. "+
			"Answer with that one line and nothing else.\n\nWhat you did:\n\n%s",
		subjectLimit, summary)

	namer := r.Plain
	if namer == nil {
		namer = r.Agent
	}
	answer, err := namer.Chat(ctx, agent.History(nil).WithUser(question), io.Discard)
	if err != nil {
		return subjectOf(run.Instruction)
	}
	if subject := subjectLine(answer.Content); subject != "" {
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
