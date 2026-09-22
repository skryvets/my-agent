package task

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/skryvets/my-agent/internal/agent"
)

// work is the run itself, one stage after the other. Each stage reports what
// it is doing, because the whole thing takes minutes.
func (r *Runner) work(ctx context.Context, run *Run, repo Repo, dir string, report Report) error {
	git := Git{Dir: dir, Token: r.Token, Host: r.GitHost}

	report(fmt.Sprintf("Cloning %s", repo))
	if err := git.Clone(ctx, repo, run.Branch); err != nil {
		return err
	}

	config, err := environment(dir, report)
	if err != nil {
		return err
	}
	// The container works on the checkout through a bind, so it never sees
	// the token.
	report("Starting the dev container")
	box, err := r.Sandbox.Start(ctx, run.ID, config)
	if err != nil {
		return err
	}
	defer release(ctx, box)
	if err := setUp(ctx, box, config, report); err != nil {
		return err
	}

	run.State = Working
	r.save(*run)
	report("Working on it")

	summary, err := r.think(ctx, repo, run, box, config.Workspace())
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

// think lets the model do the work inside the container, through an agent
// whose tools reach that container and no other. It is told not to touch git,
// because the branch, the commit and the pull request are made with a token
// it must never see.
func (r *Runner) think(ctx context.Context, repo Repo, run *Run, box Container, workspace string) (string, error) {
	worker, err := r.Worker(ctx, box)
	if err != nil {
		return "", err
	}

	instruction := fmt.Sprintf(
		"You are working in %s, a checkout of %s on the branch %s, inside its dev container.\n\n"+
			"Do this: %s\n\n"+
			"Look around with the shell first. Change the files you need to change, "+
			"and run the tests of the project before you finish. "+
			"Do not run git: the branch, the commit and the pull request are made for you. "+
			"When you are done, answer with a short summary of what you changed and why.",
		workspace, repo, run.Branch, run.Instruction)

	answer, err := worker.Chat(ctx, agent.History(nil).WithUser(instruction), io.Discard)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(answer), nil
}
