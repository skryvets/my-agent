package task

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Git runs git on the host, where the token lives. The container the model
// works in never sees the token and never needs a network.
type Git struct {
	Dir   string
	Token string
	// Host is where the repositories live. An empty Host means github.com.
	Host string
}

// Clone copies one repository into Dir and leaves a new branch checked out.
// The token is in the address, which is why every error is cleaned first.
func (g Git) Clone(ctx context.Context, repo Repo, branch string) error {
	return g.cloneFrom(ctx, g.address(repo), branch)
}

// address is where one repository is cloned from, with the token in it when
// there is one.
func (g Git) address(repo Repo) string {
	host := g.Host
	if host == "" {
		host = "https://github.com"
	}
	if g.Token != "" {
		if rest, https := strings.CutPrefix(host, "https://"); https {
			host = fmt.Sprintf("https://x-access-token:%s@%s", g.Token, rest)
		}
	}
	return fmt.Sprintf("%s/%s/%s.git", host, repo.Owner, repo.Name)
}

func (g Git) cloneFrom(ctx context.Context, url, branch string) error {
	if _, err := g.run(ctx, "clone", "--depth", "1", url, g.Dir); err != nil {
		return err
	}
	_, err := g.run(ctx, "-C", g.Dir, "checkout", "-b", branch)
	return err
}

// Changed reports whether the checkout differs from what was cloned.
func (g Git) Changed(ctx context.Context) (bool, error) {
	out, err := g.run(ctx, "-C", g.Dir, "status", "--porcelain")
	return strings.TrimSpace(out) != "", err
}

// Commit records every change under one subject.
func (g Git) Commit(ctx context.Context, subject, body string) error {
	if _, err := g.run(ctx, "-C", g.Dir, "add", "-A"); err != nil {
		return err
	}
	message := subject
	if body != "" {
		message += "\n\n" + body
	}
	_, err := g.run(ctx, "-C", g.Dir,
		"-c", "user.name=my-agent",
		"-c", "user.email=my-agent@users.noreply.github.com",
		"commit", "-m", message)
	return err
}

// Push sends the branch to the remote it was cloned from.
func (g Git) Push(ctx context.Context, branch string) error {
	_, err := g.run(ctx, "-C", g.Dir, "push", "--set-upstream", "origin", branch)
	return err
}

// run keeps the token out of the error it returns, because the error is sent
// to a chat.
func (g Git) run(ctx context.Context, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, "git", args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %v: %s", args[0], err, g.hide(string(out)))
	}
	return string(out), nil
}

func (g Git) hide(text string) string {
	if g.Token == "" {
		return strings.TrimSpace(text)
	}
	return strings.TrimSpace(strings.ReplaceAll(text, g.Token, "***"))
}
