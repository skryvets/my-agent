package task

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/skryvets/my-agent/internal/conversation"
	"github.com/skryvets/my-agent/internal/devcontainer"
)

// setupTail keeps enough of a failed setup command to say why it failed.
const setupTail = 1500

// prepare starts the dev container of the repository on the checkout and runs
// the commands that finish its setup. The caller releases the container, which
// is safe even when prepare failed half way.
func (r *Runner) prepare(ctx context.Context, key, dir string, report Report) (devcontainer.Config, error) {
	config, err := devcontainer.Load(dir)
	if err != nil {
		return devcontainer.Config{}, err
	}
	if skipped := config.Skipped(); len(skipped) > 0 {
		report("The dev container also asks for " + strings.Join(skipped, ", ") + ", which I skip")
	}
	if err := shareCheckout(dir); err != nil {
		return devcontainer.Config{}, err
	}

	report("Starting the dev container")
	if err := r.Sandbox.Bind(ctx, key, config); err != nil {
		return devcontainer.Config{}, err
	}
	inside := conversation.WithKey(ctx, key)
	for _, step := range config.Lifecycle() {
		report("Running " + step.Name)
		output, code, err := r.Sandbox.Exec(inside, step.Args)
		if err != nil {
			return devcontainer.Config{}, err
		}
		if code != 0 {
			return devcontainer.Config{}, fmt.Errorf("%s exited with %d: %s", step.Name, code, tail(output))
		}
	}
	return config, nil
}

// release throws the container away. What the user of the image wrote into
// the checkout may belong to that user, so the container first makes it
// writable for everyone, or the host could not delete the checkout.
func (r *Runner) release(ctx context.Context, key string) {
	ctx = context.WithoutCancel(ctx)
	r.Sandbox.Exec(conversation.WithKey(ctx, key), []string{"chmod", "-R", "a+rwX", "."})
	r.Sandbox.Close(ctx, key)
}

// shareCheckout makes the clone writable for every user, because the user a
// dev container runs as is rarely the user that cloned on the host.
func shareCheckout(dir string) error {
	return filepath.WalkDir(dir, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		share := fs.FileMode(0o666)
		if entry.IsDir() {
			share = 0o777
		}
		return os.Chmod(name, info.Mode().Perm()|share)
	})
}

func tail(output string) string {
	output = strings.TrimSpace(output)
	if len(output) > setupTail {
		output = "..." + output[len(output)-setupTail:]
	}
	return output
}
