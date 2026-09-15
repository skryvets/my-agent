package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
)

// Container is one conversation's workspace. Commands run inside it, and the
// files the agent reads and writes are its files.
type Container struct {
	docker *docker
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
		return "", err
	}
	if code != 0 {
		output += fmt.Sprintf("\n[exit status %d]", code)
	}
	return output, nil
}

// Exec runs one program with its arguments, as the user and with the
// environment of the dev container, and returns the output and the exit code.
func (c *Container) Exec(ctx context.Context, args []string) (string, int, error) {
	var created struct {
		ID string `json:"Id"`
	}
	config := map[string]any{
		"AttachStdout": true,
		"AttachStderr": true,
		"Tty":          true,
		"Cmd":          args,
		"WorkingDir":   c.dir,
		"User":         c.user,
		"Env":          c.env,
	}
	if err := c.docker.call(ctx, http.MethodPost, "/containers/"+c.id+"/exec", config, &created); err != nil {
		return "", 0, err
	}

	// Tty asks for one raw stream instead of the multiplexed one, so stdout
	// and stderr arrive already joined and need no frame headers removed.
	start := map[string]any{"Detach": false, "Tty": true}
	resp, err := c.docker.do(ctx, http.MethodPost, "/exec/"+created.ID+"/start", "application/json", jsonBody(start))
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	output, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, err
	}

	var status struct {
		ExitCode int `json:"ExitCode"`
	}
	if err := c.docker.call(ctx, http.MethodGet, "/exec/"+created.ID+"/json", nil, &status); err != nil {
		return "", 0, err
	}
	return string(output), status.ExitCode, nil
}

// remove throws the container away with everything written inside it.
func (c *Container) remove(ctx context.Context) error {
	return c.docker.call(ctx, http.MethodDelete, "/containers/"+c.id+"?force=true&v=true", nil, nil)
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

func jsonBody(value any) io.Reader {
	encoded, err := json.Marshal(value)
	if err != nil {
		return strings.NewReader("{}")
	}
	return bytes.NewReader(encoded)
}
