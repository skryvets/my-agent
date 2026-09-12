package sandbox

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/skryvets/my-agent/internal/conversation"
)

const (
	// DefaultImage carries the Go toolchain and git, so the agent can build
	// and push what it changes.
	DefaultImage = "golang:1.26"

	// DefaultIdle is how long a container outlives the last command it ran.
	DefaultIdle = 30 * time.Minute

	// workDir is where every container works and where the tools resolve a
	// relative path.
	workDir = "/work"
)

// Options configure a Pool. The zero value of each one is a sane default.
type Options struct {
	Socket string
	Image  string
	Idle   time.Duration
}

// Pool gives each conversation one container and throws it away when the
// conversation ends or goes quiet. Pool satisfies the Workspace the tools
// need, and reads the conversation out of the context of each call.
type Pool struct {
	docker *docker
	image  string
	idle   time.Duration

	mu      sync.Mutex
	running map[string]*entry
}

type entry struct {
	container *Container
	lastUse   time.Time
}

// New reaches the daemon, makes sure the image is there and starts the reaper.
// The reaper stops with ctx.
func New(ctx context.Context, options Options) (*Pool, error) {
	socket := options.Socket
	if socket == "" {
		socket = DefaultSocket
	}
	pool := &Pool{
		docker:  newDocker(socket),
		image:   options.Image,
		idle:    options.Idle,
		running: make(map[string]*entry),
	}
	if pool.image == "" {
		pool.image = DefaultImage
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
	if err := pool.pullImage(ctx); err != nil {
		return nil, err
	}

	go pool.reap(ctx)
	return pool, nil
}

// Close throws away the container of one conversation. Everything written
// inside it is lost, which is what /reset promises.
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

// container returns the container of the conversation and starts one the first
// time. The lock is held across the start, which is fast because New already
// pulled the image.
func (p *Pool) container(ctx context.Context) (*Container, error) {
	key := conversation.KeyOf(ctx)

	p.mu.Lock()
	defer p.mu.Unlock()

	if held, ok := p.running[key]; ok {
		held.lastUse = time.Now()
		return held.container, nil
	}

	container, err := p.start(ctx, key)
	if err != nil {
		return nil, err
	}
	p.running[key] = &entry{container: container, lastUse: time.Now()}
	return container, nil
}
