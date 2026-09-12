package sandbox

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
)

// label marks every container the agent starts, so a container left behind by
// a process that died can be found again.
const label = "my-agent.conversation"

// start creates and starts one container. It has no network of its own, so a
// command inside it cannot reach the internet or the host. A bind, when there
// is one, is the host directory the container works on.
func (p *Pool) start(ctx context.Context, key, bind string) (*Container, error) {
	var created struct {
		ID string `json:"Id"`
	}
	host := map[string]any{
		"NetworkMode": "none",
		"AutoRemove":  false,
	}
	if bind != "" {
		host["Binds"] = []string{bind + ":" + workDir}
	}
	config := map[string]any{
		"Image":      p.image,
		"Cmd":        []string{"sleep", "infinity"},
		"WorkingDir": workDir,
		"Labels":     map[string]string{label: key},
		"HostConfig": host,
	}
	if err := p.docker.call(ctx, http.MethodPost, "/containers/create", config, &created); err != nil {
		return nil, err
	}

	if err := p.docker.call(ctx, http.MethodPost, "/containers/"+created.ID+"/start", nil, nil); err != nil {
		return nil, err
	}
	log.Printf("started container %s for %s", shortID(created.ID), key)
	return &Container{docker: p.docker, id: created.ID, dir: workDir}, nil
}

// pullImage downloads the image unless the daemon already has it. New does it
// once, so starting a container later is fast and holds the lock briefly.
func (p *Pool) pullImage(ctx context.Context) error {
	if err := p.docker.call(ctx, http.MethodGet, "/images/"+url.PathEscape(p.image)+"/json", nil, nil); err == nil {
		return nil
	}

	name, tag, found := strings.Cut(p.image, ":")
	if !found {
		tag = "latest"
	}
	query := "?fromImage=" + url.QueryEscape(name) + "&tag=" + url.QueryEscape(tag)
	log.Printf("pulling %s, this happens once", p.image)

	resp, err := p.docker.do(ctx, http.MethodPost, "/images/create"+query, "", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// The daemon reports progress until the pull ends. Reading to the end is
	// what waits for it.
	_, err = io.Copy(io.Discard, resp.Body)
	return err
}

// sweepOrphans removes the containers of a previous run of the agent. A
// restart on Railway would otherwise leave one container for every chat.
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
