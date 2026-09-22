// Package sandbox starts the dev container of a repository and runs the
// agent's tools inside it, one container for each conversation.
package sandbox

import (
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/moby/moby/api/types/jsonstream"
	"github.com/moby/moby/client"
)

// apiVersion is pinned. The daemon of Docker 29 serves 1.55 and accepts
// everything back to 1.40, so an old daemon still answers.
const apiVersion = "v1.43"

// newDocker reaches the daemon at DOCKER_HOST, or at /var/run/docker.sock
// when it is not set, as the docker command does.
func newDocker() (*client.Client, error) {
	return client.New(client.FromEnv, client.WithAPIVersion(apiVersion))
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
