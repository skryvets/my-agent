// Package sandbox starts the dev container of a repository and runs the
// agent's tools inside it, one container for each task.
package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"strings"

	"github.com/moby/moby/api/types/jsonstream"
	"github.com/moby/moby/client"
)

// apiVersion is pinned. The daemon of Docker 29 serves 1.55 and accepts
// everything back to 1.40, so an old daemon still answers.
const apiVersion = "v1.43"

// label marks every container the agent starts, so a container left behind by
// a process that died can be found again.
const label = "my-agent.task"

// Docker is the daemon the dev containers run on. It reaches the daemon at
// DOCKER_HOST, or at /var/run/docker.sock when it is not set, as the docker
// command does.
type Docker struct {
	client *client.Client
}

// New reaches the daemon and removes what an earlier run of the agent left
// behind.
func New(ctx context.Context) (*Docker, error) {
	docker, err := client.New(client.FromEnv, client.WithAPIVersion(apiVersion))
	if err != nil {
		return nil, err
	}
	d := &Docker{client: docker}
	if err := d.Sweep(ctx); err != nil {
		return nil, err
	}
	return d, nil
}

// Sweep removes every container the agent started, from this run or an
// earlier one. It runs at start, so a restart does not leave one container
// for every task that was under way, and at exit.
func (d *Docker) Sweep(ctx context.Context) error {
	found, err := d.client.ContainerList(ctx, client.ContainerListOptions{
		All:     true,
		Filters: make(client.Filters).Add("label", label),
	})
	if err != nil {
		return err
	}
	for _, summary := range found.Items {
		orphan := &Container{docker: d.client, id: summary.ID}
		if err := orphan.Remove(ctx); err != nil {
			return err
		}
		log.Printf("removed the container %s", shortID(summary.ID))
	}
	return nil
}

// progressTail is how much of a failed build is kept to say why it failed.
const progressTail = 2000

// readProgress waits for a build to end and returns the image id it reports.
// The daemon answers with 200 and sends a failure inside the stream, so the
// status alone says nothing.
func readProgress(body io.Reader) (string, error) {
	decoder := json.NewDecoder(body)
	var image, log string
	for {
		var message jsonstream.Message
		if err := decoder.Decode(&message); err == io.EOF {
			return image, nil
		} else if err != nil {
			return "", err
		}
		if message.Error != nil {
			return "", errors.New(strings.TrimSpace(strings.TrimSpace(log) + "\n" + message.Error.Message))
		}
		if message.Aux != nil {
			var aux struct {
				ID string `json:"ID"`
			}
			if json.Unmarshal(*message.Aux, &aux) == nil && aux.ID != "" {
				image = aux.ID
			}
		}
		log += message.Stream
		if len(log) > progressTail {
			log = log[len(log)-progressTail:]
		}
	}
}

// shortID is how Docker names a container in a log line. A daemon returns a
// long id, but nothing promises one.
func shortID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12]
}
