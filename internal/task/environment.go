package task

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/skryvets/my-agent/internal/devcontainer"
)

// setupTail keeps enough of a failed setup command to say why it failed.
const setupTail = 1500

// environment reads what the repository says about its dev container, and
// opens the checkout to the user that container runs as.
func environment(dir string, report Report) (devcontainer.Config, error) {
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
	return config, nil
}

// setUp runs the commands that finish the setup of a new container, in the
// order the specification gives.
func setUp(ctx context.Context, box Container, config devcontainer.Config, report Report) error {
	for _, step := range config.Lifecycle() {
		report("Running " + step.Name)
		output, code, err := box.Exec(ctx, step.Args)
		if err != nil {
			return err
		}
		if code != 0 {
			return fmt.Errorf("%s exited with %d: %s", step.Name, code, tail(output))
		}
	}
	return nil
}

// release throws the container away. What the user of the image wrote into
// the checkout may belong to that user, so the container first makes it
// writable for everyone, or the host could not delete the checkout.
func release(ctx context.Context, box Container) {
	ctx = context.WithoutCancel(ctx)
	box.Exec(ctx, []string{"chmod", "-R", "a+rwX", "."})
	box.Remove(ctx)
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
