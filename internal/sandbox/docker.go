// Package sandbox starts the dev container of a repository and runs the
// agent's tools inside it, one container for each conversation.
package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
)

// apiVersion is pinned. The daemon of Docker 29 serves 1.55 and accepts
// everything back to 1.40, so an old daemon still answers.
const apiVersion = "v1.43"

// DefaultSocket is where the Docker Engine listens on Linux and on macOS.
const DefaultSocket = "/var/run/docker.sock"

// docker is the Engine API over a unix socket. The socket is not a network
// address, so the transport dials it and the URL host is a placeholder.
type docker struct {
	http *http.Client
}

func newDocker(socket string) *docker {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", socket)
		},
	}
	return &docker{http: &http.Client{Transport: transport}}
}

// do sends one request and fails on any status the caller did not expect.
func (d *docker) do(ctx context.Context, method, path, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, "http://docker/"+apiVersion+path, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := d.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("the Docker daemon did not answer: %w", err)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		defer resp.Body.Close()
		return nil, dockerError(method, path, resp)
	}
	return resp, nil
}

// call sends a JSON request and reads a JSON answer. A nil in sends no body, a
// nil out throws the answer away.
func (d *docker) call(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	contentType := ""
	if in != nil {
		encoded, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
		contentType = "application/json"
	}

	resp, err := d.do(ctx, method, path, contentType, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if out == nil {
		_, err = io.Copy(io.Discard, resp.Body)
		return err
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// dockerError turns the message the daemon returns into an error.
func dockerError(method, path string, resp *http.Response) error {
	var answer struct {
		Message string `json:"message"`
	}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err := json.Unmarshal(data, &answer); err == nil && answer.Message != "" {
		return fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, answer.Message)
	}
	return fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(data)))
}

// progressTail is how much of a failed build is kept to say why it failed.
const progressTail = 2000

// readProgress waits for a pull or a build to end and returns the image id a
// build reports. The daemon answers both with 200 and sends a failure inside
// the stream, so the status alone says nothing.
func readProgress(body io.Reader) (string, error) {
	decoder := json.NewDecoder(body)
	var image, log string
	for {
		var message struct {
			Stream string `json:"stream"`
			Error  string `json:"error"`
			Aux    struct {
				ID string `json:"ID"`
			} `json:"aux"`
		}
		if err := decoder.Decode(&message); err == io.EOF {
			return image, nil
		} else if err != nil {
			return "", err
		}
		if message.Error != "" {
			return "", errors.New(strings.TrimSpace(strings.TrimSpace(log) + "\n" + message.Error))
		}
		if message.Aux.ID != "" {
			image = message.Aux.ID
		}
		log += message.Stream
		if len(log) > progressTail {
			log = log[len(log)-progressTail:]
		}
	}
}
