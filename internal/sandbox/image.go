package sandbox

import (
	"context"
	"log"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"

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
	workspace := config.Workspace()
	created, err := p.docker.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config: &container.Config{
			Image:      image,
			Entrypoint: keepAlive,
			WorkingDir: workspace,
			Env:        config.ContainerEnvironment(),
			User:       config.ContainerUser,
			Labels:     map[string]string{label: key},
		},
		HostConfig: &container.HostConfig{
			Binds: []string{config.Root + ":" + workspace},
		},
	})
	if err != nil {
		return nil, err
	}
	started := &Container{docker: p.docker, id: created.ID, dir: workspace, user: config.RemoteUser}

	if _, err := p.docker.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
		started.remove(context.WithoutCancel(ctx))
		return nil, err
	}
	if len(config.RemoteEnv) > 0 {
		inspected, err := p.docker.ContainerInspect(ctx, created.ID, client.ContainerInspectOptions{})
		if err != nil {
			started.remove(context.WithoutCancel(ctx))
			return nil, err
		}
		started.env = config.RemoteEnvironment(inspected.Container.Config.Env)
	}
	log.Printf("started container %s for %s", shortID(created.ID), key)
	return started, nil
}

// pull downloads the image unless the daemon already has it. A reference
// without a tag means latest, as it does for docker pull.
func (p *Pool) pull(ctx context.Context, reference string) error {
	if _, err := p.docker.ImageInspect(ctx, reference); err == nil {
		return nil
	}
	log.Printf("pulling %s", reference)
	pulled, err := p.docker.ImagePull(ctx, reference, client.ImagePullOptions{})
	if err != nil {
		return err
	}
	return pulled.Wait(ctx)
}

// sweepOrphans removes the containers of a previous run of the agent, so a
// restart does not leave one container for every task that was under way.
func (p *Pool) sweepOrphans(ctx context.Context) error {
	found, err := p.docker.ContainerList(ctx, client.ContainerListOptions{
		All:     true,
		Filters: make(client.Filters).Add("label", label),
	})
	if err != nil {
		return err
	}
	for _, summary := range found.Items {
		orphan := &Container{docker: p.docker, id: summary.ID}
		if err := orphan.remove(ctx); err != nil {
			return err
		}
		log.Printf("removed the container %s of an earlier run", shortID(summary.ID))
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
