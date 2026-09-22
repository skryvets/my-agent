// Package task does one coding job end to end: clone a repository, let the
// model work on it in a container, run the tests, push a branch and open a
// pull request.
//
// git and the GitHub API run in the agent process, not in the container. The
// model therefore never sees the token, and the two steps that publish the
// change are made by code rather than by the model.
package task

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/skryvets/my-agent/internal/agent"
	"github.com/skryvets/my-agent/internal/devcontainer"
)

// started counts the runs of this process, so two runs of one second still
// get different names.
var started atomic.Int64

// Agent answers a conversation. *agent.Client satisfies it.
type Agent interface {
	Chat(ctx context.Context, history agent.History, stream io.Writer) (string, error)
}

// Sandbox starts the dev container of a checkout on the host and runs the
// commands that set it up. *sandbox.Pool satisfies it.
type Sandbox interface {
	Bind(ctx context.Context, key string, config devcontainer.Config) error
	Exec(ctx context.Context, args []string) (output string, code int, err error)
	Close(ctx context.Context, key string) error
}

// A Report carries one line of progress back to the person who asked. A run
// takes minutes, so silence would look like a hang.
type Report func(text string)

// Runner does the whole job.
type Runner struct {
	Agent   Agent
	Sandbox Sandbox
	GitHub  GitHub
	Store   Store

	// Plain answers the questions that need no tool, such as naming the
	// change. A nil Plain uses Agent, which then carries the tool schemas
	// into a question that cannot use them.
	Plain Agent

	// Token reaches GitHub over https for the clone and the push.
	Token string
	// WorkRoot holds one checkout for each run. An empty WorkRoot uses the
	// temporary directory of the system.
	WorkRoot string
	// GitHost is where git clones from. An empty GitHost means github.com.
	GitHost string
}

// Start does one run and returns when the pull request is open, or when
// something stopped it. The caller is one conversation, which waits.
func (r *Runner) Start(ctx context.Context, chat, repository, instruction string, report Report) error {
	repo, err := ParseRepo(repository)
	if err != nil {
		return err
	}
	if strings.TrimSpace(instruction) == "" {
		return fmt.Errorf("say what to do in %s", repo)
	}

	run := Run{
		ID:          fmt.Sprintf("%s-%d", time.Now().UTC().Format("20060102-150405"), started.Add(1)),
		Chat:        chat,
		Repo:        repo.String(),
		Instruction: instruction,
		State:       Cloning,
		Started:     time.Now().UTC(),
	}
	run.Branch = branchName(run.ID)
	r.save(run)

	dir, err := r.checkout(run.ID, repo)
	if err != nil {
		return r.fail(ctx, run, err)
	}
	defer os.RemoveAll(filepath.Dir(dir))

	if err := r.work(ctx, &run, repo, dir, report); err != nil {
		return r.fail(ctx, run, err)
	}
	return nil
}

// Interrupted marks the runs a restart caught in the middle and returns them,
// so each conversation can be told what was lost.
func (r *Runner) Interrupted(ctx context.Context) []Run {
	runs, err := r.Store.Load()
	if err != nil {
		return nil
	}

	var lost []Run
	for _, run := range runs {
		if run.State.Done() {
			continue
		}
		run.State = Interrupted
		run.Detail = "a restart caught this run in the middle"
		run.Ended = time.Now().UTC()
		r.save(run)
		lost = append(lost, run)
	}
	return lost
}

// checkout names the directory the clone goes into. The last part is the name
// of the repository, because a dev container names its workspace after it.
func (r *Runner) checkout(id string, repo Repo) (string, error) {
	root := r.WorkRoot
	if root == "" {
		root = os.TempDir()
	}
	run := filepath.Join(root, "my-agent-"+id)
	if err := os.MkdirAll(run, 0o755); err != nil {
		return "", err
	}
	// git clone wants a directory that is not there yet.
	return filepath.Join(run, repo.Name), nil
}

func (r *Runner) save(run Run) {
	if err := r.Store.Save(run); err != nil {
		report := fmt.Sprintf("could not write the state of run %s: %v", run.ID, err)
		fmt.Fprintln(os.Stderr, report)
	}
}

// fail records how a run ended. A run whose context ended was stopped from
// the chat, and the error it gave on the way out says nothing more.
func (r *Runner) fail(ctx context.Context, run Run, err error) error {
	run.State = Failed
	run.Detail = err.Error()
	if ctx.Err() != nil {
		run.State = Stopped
		run.Detail = "stopped from the chat"
	}
	run.Ended = time.Now().UTC()
	r.save(run)
	return err
}

// branchName is short, names the agent, and cannot collide with a second run.
func branchName(id string) string { return "my-agent/" + id }
