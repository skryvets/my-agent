package sandbox

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/skryvets/my-agent/internal/devcontainer"
)

// label marks every container the agent starts, so a container left behind by
// a process that died can be found again.
const label = "my-agent.conversation"

// keepAlive holds a container open until it is removed. It replaces the
// entrypoint of the image, which may be a program that exits, and it avoids
// sleep infinity, which busybox does not know.
var keepAlive = []string{"sh", "-c", "trap 'exit 0' TERM; while sleep 1000 & wait $!; do :; done"}

// image is the image a dev container starts from. A Dockerfile wins over an
// image name, as in the reference tool.
func (p *Pool) image(ctx context.Context, config devcontainer.Config) (string, error) {
	if config.Build.Dockerfile != "" {
		return p.build(ctx, config)
	}
	return config.Image, p.pull(ctx, config.Image)
}

// start creates and starts one container with the checkout mounted as its
// workspace.
func (p *Pool) start(ctx context.Context, key, image string, config devcontainer.Config) (*Container, error) {
	var created struct {
		ID string `json:"Id"`
	}
	workspace := config.Workspace()
	create := map[string]any{
		"Image":      image,
		"Entrypoint": keepAlive,
		"WorkingDir": workspace,
		"Env":        config.ContainerEnvironment(),
		"User":       config.ContainerUser,
		"Labels":     map[string]string{label: key},
		"HostConfig": map[string]any{
			"Binds": []string{config.Root + ":" + workspace},
		},
	}
	if err := p.docker.call(ctx, http.MethodPost, "/containers/create", create, &created); err != nil {
		return nil, err
	}
	container := &Container{docker: p.docker, id: created.ID, dir: workspace, user: config.RemoteUser}

	if err := p.docker.call(ctx, http.MethodPost, "/containers/"+created.ID+"/start", nil, nil); err != nil {
		container.remove(context.WithoutCancel(ctx))
		return nil, err
	}
	if len(config.RemoteEnv) > 0 {
		var inspected struct {
			Config struct {
				Env []string `json:"Env"`
			} `json:"Config"`
		}
		if err := p.docker.call(ctx, http.MethodGet, "/containers/"+created.ID+"/json", nil, &inspected); err != nil {
			container.remove(context.WithoutCancel(ctx))
			return nil, err
		}
		container.env = config.RemoteEnvironment(inspected.Config.Env)
	}
	log.Printf("started container %s for %s", shortID(created.ID), key)
	return container, nil
}

// pull downloads the image unless the daemon already has it.
func (p *Pool) pull(ctx context.Context, reference string) error {
	if err := p.docker.call(ctx, http.MethodGet, "/images/"+url.PathEscape(reference)+"/json", nil, nil); err == nil {
		return nil
	}
	name, tag := splitReference(reference)
	query := "?fromImage=" + url.QueryEscape(name) + "&tag=" + url.QueryEscape(tag)

	log.Printf("pulling %s", reference)
	resp, err := p.docker.do(ctx, http.MethodPost, "/images/create"+query, "", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, err = readProgress(resp.Body)
	return err
}

// splitReference parts an image reference into what /images/create takes. An
// empty tag would pull every tag of the image, so a reference without one
// means latest, as it does for docker pull.
func splitReference(reference string) (name, tag string) {
	if name, digest, found := strings.Cut(reference, "@"); found {
		return name, digest
	}
	slash := strings.LastIndex(reference, "/")
	if colon := strings.LastIndex(reference, ":"); colon > slash {
		return reference[:colon], reference[colon+1:]
	}
	return reference, "latest"
}

// sweepOrphans removes the containers of a previous run of the agent, so a
// restart does not leave one container for every task that was under way.
func (p *Pool) sweepOrphans(ctx context.Context) error {
	filters := url.QueryEscape(`{"label":["` + label + `"]}`)
	var found []struct {
		ID string `json:"Id"`
	}
	if err := p.docker.call(ctx, http.MethodGet, "/containers/json?all=true&filters="+filters, nil, &found); err != nil {
		return err
	}
	for _, container := range found {
		orphan := &Container{docker: p.docker, id: container.ID}
		if err := orphan.remove(ctx); err != nil {
			return err
		}
		log.Printf("removed the container %s of an earlier run", shortID(container.ID))
	}
	return nil
}

// shortID is how Docker names a container in a log line. A daemon returns a
// long id, but nothing promises one.
func shortID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12]
}
