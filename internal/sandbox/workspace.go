package sandbox

import "context"

// A Pool is the Workspace the tools work in. Every call names its conversation
// through the context, and the pool finds or starts that container.

// Run executes a command in the container of the conversation.
func (p *Pool) Run(ctx context.Context, command string) (string, error) {
	container, err := p.container(ctx)
	if err != nil {
		return "", err
	}
	return container.Run(ctx, command)
}

// ReadFile returns a file of the container of the conversation.
func (p *Pool) ReadFile(ctx context.Context, name string) (string, error) {
	container, err := p.container(ctx)
	if err != nil {
		return "", err
	}
	return container.ReadFile(ctx, name)
}

// WriteFile writes a file into the container of the conversation.
func (p *Pool) WriteFile(ctx context.Context, name, content string) error {
	container, err := p.container(ctx)
	if err != nil {
		return err
	}
	return container.WriteFile(ctx, name, content)
}
