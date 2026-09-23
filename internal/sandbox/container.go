package sandbox

import (
	"context"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/moby/moby/client"
)

// Container is the dev container of one task. Commands run inside it, and
// the files the agent reads and writes are its files. It is the Workspace the
// tools work in.
type Container struct {
	docker *client.Client
	id     string
	dir    string
	user   string
	env    []string
}

// Run executes one command line with sh -c and returns its combined output.
// The exit status is reported in the text, because a command that fails is an
// answer the model reads, not a failure of the sandbox.
func (c *Container) Run(ctx context.Context, command string) (string, error) {
	output, code, err := c.Exec(ctx, []string{"sh", "-c", command})
	if err != nil {
		return output, err
	}
	if code != 0 {
		output += fmt.Sprintf("\n[exit status %d]", code)
	}
	return output, nil
}

// Exec runs one program with its arguments, as the user and with the
// environment of the dev container, and returns the output and the exit code.
func (c *Container) Exec(ctx context.Context, args []string) (string, int, error) {
	created, err := c.docker.ExecCreate(ctx, c.id, client.ExecCreateOptions{
		AttachStdout: true,
		AttachStderr: true,
		TTY:          true,
		Cmd:          args,
		WorkingDir:   c.dir,
		User:         c.user,
		Env:          c.env,
	})
	if err != nil {
		return "", 0, err
	}

	// A TTY gives one raw stream instead of the multiplexed one, so stdout and
	// stderr arrive already joined and need no frame headers removed.
	attached, err := c.docker.ExecAttach(ctx, created.ID, client.ExecAttachOptions{TTY: true})
	if err != nil {
		return "", 0, err
	}
	defer attached.Close()
	// The client watches ctx only until the connection is upgraded, so a
	// command that never ends would block the read below forever.
	stop := context.AfterFunc(ctx, attached.Close)
	defer stop()

	output, err := io.ReadAll(attached.Reader)
	if ctx.Err() != nil {
		return string(output), 0, ctx.Err()
	}
	if err != nil {
		return "", 0, err
	}

	status, err := c.docker.ExecInspect(ctx, created.ID, client.ExecInspectOptions{})
	if err != nil {
		return "", 0, err
	}
	return string(output), status.ExitCode, nil
}

// Remove throws the container away with everything written inside it that is
// not in the workspace.
func (c *Container) Remove(ctx context.Context) error {
	_, err := c.docker.ContainerRemove(ctx, c.id, client.ContainerRemoveOptions{Force: true, RemoveVolumes: true})
	return err
}

// resolve reads a path the model gave against the working directory.
func (c *Container) resolve(name string) string {
	if path.IsAbs(name) {
		return path.Clean(name)
	}
	return path.Join(c.dir, name)
}

func shellQuote(text string) string {
	return "'" + strings.ReplaceAll(text, "'", `'\''`) + "'"
}
