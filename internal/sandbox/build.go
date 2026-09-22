package sandbox

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/moby/moby/client"

	"github.com/skryvets/my-agent/internal/devcontainer"
)

// build makes the image of a dev container from its Dockerfile and returns the
// id of the image. The context travels to the daemon as a tar, as it does for
// docker build.
func (p *Pool) build(ctx context.Context, config devcontainer.Config) (string, error) {
	contextDir := config.BuildContext()
	dockerfile, err := filepath.Rel(contextDir, config.Dockerfile())
	if err != nil || !filepath.IsLocal(dockerfile) {
		return "", fmt.Errorf("the Dockerfile %s is outside the build context %s", config.Build.Dockerfile, config.Build.Context)
	}

	archive, writer := io.Pipe()
	defer archive.Close()
	go func() { writer.CloseWithError(pack(contextDir, writer)) }()

	log.Printf("building the image of %s", filepath.Base(config.Root))
	built, err := p.docker.ImageBuild(ctx, archive, client.ImageBuildOptions{
		Dockerfile: filepath.ToSlash(dockerfile),
		BuildArgs:  config.BuildArgs(),
		Target:     config.Build.Target,
		Remove:     true,
	})
	if err != nil {
		return "", err
	}
	defer built.Body.Close()

	image, err := readProgress(built.Body)
	if err != nil {
		return "", fmt.Errorf("building %s: %w", config.Build.Dockerfile, err)
	}
	if image == "" {
		return "", errors.New("the build ended without an image")
	}
	return image, nil
}

// pack writes a directory as a tar.
// TODO: read .dockerignore, so a large repository sends less to the daemon
func pack(dir string, out io.Writer) error {
	archive := tar.NewWriter(out)
	if err := archive.AddFS(os.DirFS(dir)); err != nil {
		return err
	}
	return archive.Close()
}
