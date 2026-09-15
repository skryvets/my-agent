package sandbox

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/skryvets/my-agent/internal/conversation"
	"github.com/skryvets/my-agent/internal/devcontainer"
)

// DefaultIdle is how long a container outlives the last command it ran.
const DefaultIdle = 30 * time.Minute

// Options configure a Pool. The zero value of each one is a sane default.
type Options struct {
	Socket string
	Idle   time.Duration
}

// Pool holds one container for each conversation that was given one, and
// throws it away when the conversation ends or goes quiet. Pool satisfies the
// Workspace the tools need, and reads the conversation out of the context of
// each call.
type Pool struct {
	docker *docker
	idle   time.Duration

	mu      sync.Mutex
	running map[string]*entry
}

type entry struct {
	container *Container
	lastUse   time.Time
}

// New reaches the daemon, clears an earlier run and starts the reaper. The
// reaper stops with ctx.
func New(ctx context.Context, options Options) (*Pool, error) {
	socket := options.Socket
	if socket == "" {
		socket = DefaultSocket
	}
	pool := &Pool{
		docker:  newDocker(socket),
		idle:    options.Idle,
		running: make(map[string]*entry),
	}
	if pool.idle <= 0 {
		pool.idle = DefaultIdle
	}

	if err := pool.docker.call(ctx, http.MethodGet, "/version", nil, nil); err != nil {
		return nil, err
	}
	if err := pool.sweepOrphans(ctx); err != nil {
		return nil, err
	}
	go pool.reap(ctx)
	return pool, nil
}

// Bind starts the dev container of a repository for one conversation, with
// the checkout on the host mounted as its workspace. The image is pulled or
// built first, which can take minutes, so the lock is only held to start it.
func (p *Pool) Bind(ctx context.Context, key string, config devcontainer.Config) error {
	image, err := p.image(ctx, config)
	if err != nil {
		return err
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if _, taken := p.running[key]; taken {
		return fmt.Errorf("%s already has a container", key)
	}
	container, err := p.start(ctx, key, image, config)
	if err != nil {
		return err
	}
	p.running[key] = &entry{container: container, lastUse: time.Now()}
	return nil
}

// Close throws away the container of one conversation, with everything
// written inside it that is not in the workspace.
func (p *Pool) Close(ctx context.Context, key string) error {
	p.mu.Lock()
	held, ok := p.running[key]
	delete(p.running, key)
	p.mu.Unlock()
	if !ok {
		return nil
	}
	return held.container.remove(ctx)
}

// Shutdown throws away every container the pool started.
func (p *Pool) Shutdown(ctx context.Context) {
	p.mu.Lock()
	held := p.running
	p.running = make(map[string]*entry)
	p.mu.Unlock()

	for key, held := range held {
		if err := held.container.remove(ctx); err != nil {
			log.Printf("removing the container of %s: %v", key, err)
		}
	}
}

// container returns the container of the conversation in ctx.
func (p *Pool) container(ctx context.Context) (*Container, error) {
	key := conversation.KeyOf(ctx)

	p.mu.Lock()
	defer p.mu.Unlock()
	held, ok := p.running[key]
	if !ok {
		return nil, fmt.Errorf("%s has no container", key)
	}
	held.lastUse = time.Now()
	return held.container, nil
}
