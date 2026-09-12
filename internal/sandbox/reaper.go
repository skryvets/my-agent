package sandbox

import (
	"context"
	"log"
	"time"
)

// sweepEvery is how often the reaper looks for idle containers. A pool that
// gives up its containers sooner is swept that much sooner.
const sweepEvery = time.Minute

// reap removes the containers of the conversations that went quiet.
func (p *Pool) reap(ctx context.Context) {
	ticker := time.NewTicker(min(p.idle, sweepEvery))
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			p.Shutdown(context.WithoutCancel(ctx))
			return
		case now := <-ticker.C:
			for _, key := range p.idleKeys(now) {
				if err := p.Close(ctx, key); err != nil {
					log.Printf("reaping the container of %s: %v", key, err)
					continue
				}
				log.Printf("reaped the idle container of %s", key)
			}
		}
	}
}

func (p *Pool) idleKeys(now time.Time) []string {
	p.mu.Lock()
	defer p.mu.Unlock()

	var idle []string
	for key, held := range p.running {
		if now.Sub(held.lastUse) >= p.idle {
			idle = append(idle, key)
		}
	}
	return idle
}
