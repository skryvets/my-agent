package sandbox

import (
	"archive/tar"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

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

	query := url.Values{"dockerfile": {filepath.ToSlash(dockerfile)}}
	if args := config.BuildArgs(); args != nil {
		encoded, _ := json.Marshal(args)
		query.Set("buildargs", string(encoded))
	}
	if config.Build.Target != "" {
		query.Set("target", config.Build.Target)
	}

	archive, writer := io.Pipe()
	defer archive.Close()
	go func() { writer.CloseWithError(pack(contextDir, writer)) }()

	log.Printf("building the image of %s", filepath.Base(config.Root))
	resp, err := p.docker.do(ctx, http.MethodPost, "/build?"+query.Encode(), "application/x-tar", archive)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	image, err := readProgress(resp.Body)
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
