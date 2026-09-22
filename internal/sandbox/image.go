package sandbox

import (
	"context"
	"log"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"

	"github.com/skryvets/my-agent/internal/devcontainer"
)

// keepAlive holds a container open until it is removed. It replaces the
// entrypoint of the image, which may be a program that exits, and it avoids
// sleep infinity, which busybox does not know.
var keepAlive = []string{"sh", "-c", "trap 'exit 0' TERM; while sleep 1000 & wait $!; do :; done"}

// Start creates and starts the dev container of a repository, with the
// checkout on the host mounted as its workspace. The image is pulled or built
// first, which can take minutes. name says which task the container is for.
func (d *Docker) Start(ctx context.Context, name string, config devcontainer.Config) (*Container, error) {
	image, err := d.image(ctx, config)
	if err != nil {
		return nil, err
	}

	workspace := config.Workspace()
	created, err := d.client.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config: &container.Config{
			Image:      image,
			Entrypoint: keepAlive,
			WorkingDir: workspace,
			Env:        config.ContainerEnvironment(),
			User:       config.ContainerUser,
			Labels:     map[string]string{label: name},
		},
		HostConfig: &container.HostConfig{
			Binds: []string{config.Root + ":" + workspace},
		},
	})
	if err != nil {
		return nil, err
	}
	started := &Container{docker: d.client, id: created.ID, dir: workspace, user: config.RemoteUser}

	if _, err := d.client.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
		started.Remove(context.WithoutCancel(ctx))
		return nil, err
	}
	if len(config.RemoteEnv) > 0 {
		inspected, err := d.client.ContainerInspect(ctx, created.ID, client.ContainerInspectOptions{})
		if err != nil {
			started.Remove(context.WithoutCancel(ctx))
			return nil, err
		}
		started.env = config.RemoteEnvironment(inspected.Container.Config.Env)
	}
	log.Printf("started container %s for %s", shortID(created.ID), name)
	return started, nil
}

// image is the image a dev container starts from. A Dockerfile wins over an
// image name, as in the reference tool.
func (d *Docker) image(ctx context.Context, config devcontainer.Config) (string, error) {
	if config.Build.Dockerfile != "" {
		return d.build(ctx, config)
	}
	return config.Image, d.pull(ctx, config.Image)
}

// pull downloads the image unless the daemon already has it. A reference
// without a tag means latest, as it does for docker pull.
func (d *Docker) pull(ctx context.Context, reference string) error {
	if _, err := d.client.ImageInspect(ctx, reference); err == nil {
		return nil
	}
	log.Printf("pulling %s", reference)
	pulled, err := d.client.ImagePull(ctx, reference, client.ImagePullOptions{})
	if err != nil {
		return err
	}
	return pulled.Wait(ctx)
}
