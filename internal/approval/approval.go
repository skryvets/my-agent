// Package approval keeps a person in the loop. A tool call the policy does not
// allow by itself waits for a yes or a no from the conversation it belongs to.
package approval

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/skryvets/my-agent/internal/conversation"
)

// answerWithin is how long a question waits before it counts as a no. A
// person who is not at the phone must not hold a container open all day.
const answerWithin = 5 * time.Minute

// Request is one call a person is asked to allow.
type Request struct {
	// Tool is the name the model asked for.
	Tool string
	// Details is the one line a person reads before deciding.
	Details string
}

// An Ask puts the question to a person and waits for the answer. A connector
// registers one; the key names the conversation the call belongs to.
type Ask func(ctx context.Context, key string, request Request) (bool, error)

// Broker joins the tools, which ask, to the connector, which has the person.
// The zero Broker refuses every call, because nobody is listening.
type Broker struct {
	mu  sync.RWMutex
	ask Ask
}

// Handle registers the connector that carries the question to a person.
func (b *Broker) Handle(ask Ask) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ask = ask
}

// Allowed asks whether one call may run. A question nobody answers within
// answerWithin is a no.
func (b *Broker) Allowed(ctx context.Context, request Request) (bool, error) {
	b.mu.RLock()
	ask := b.ask
	b.mu.RUnlock()

	if ask == nil {
		return false, errors.New("there is nobody to ask")
	}

	ctx, cancel := context.WithTimeout(ctx, answerWithin)
	defer cancel()

	allowed, err := ask(ctx, conversation.KeyOf(ctx), request)
	if errors.Is(err, context.DeadlineExceeded) {
		return false, nil
	}
	return allowed, err
}
