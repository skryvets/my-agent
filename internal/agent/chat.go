package agent

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

// Chat answers one user turn. The model may ask for tools first: eino runs
// them, asks the model again, and stops when the model answers in words. The
// answer is written to stream as it arrives, and returned whole at the end.
func (c *Client) Chat(ctx context.Context, history History, stream io.Writer) (string, error) {
	events := c.runner.Run(ctx, history)

	var last *schema.Message
	for {
		event, ok := events.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			return "", plain(event.Err)
		}
		if event.Output == nil || event.Output.MessageOutput == nil {
			continue
		}
		message, err := read(event.Output.MessageOutput, stream)
		if err != nil {
			return "", err
		}
		if message != nil {
			last = message
		}
	}

	if last == nil {
		return "", errors.New("empty response from model")
	}
	if last.Content != "" {
		fmt.Fprintln(stream)
	}
	return last.Content, nil
}

// read writes one message to the stream while it arrives and returns it whole.
func read(variant *adk.MessageVariant, stream io.Writer) (*schema.Message, error) {
	if !variant.IsStreaming {
		show(stream, variant.Role, variant.Message.Content)
		announce(stream, variant.Message)
		return variant.Message, nil
	}
	defer variant.MessageStream.Close()

	var chunks []*schema.Message
	for {
		chunk, err := variant.MessageStream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		show(stream, variant.Role, chunk.Content)
		chunks = append(chunks, chunk)
	}
	if len(chunks) == 0 {
		return nil, nil
	}

	message, err := schema.ConcatMessages(chunks)
	if err != nil {
		return nil, err
	}
	announce(stream, message)
	return message, nil
}

// show writes what the model says. A tool result is left out, because the
// stream is what the person reads.
func show(stream io.Writer, role schema.RoleType, text string) {
	if role == schema.Assistant && text != "" {
		fmt.Fprint(stream, text)
	}
}

// announce names the tools of one round, so a long run shows what it is doing.
func announce(stream io.Writer, message *schema.Message) {
	for _, call := range message.ToolCalls {
		fmt.Fprintf(stream, "--- tool: %s %s ---\n", call.Function.Name, call.Function.Arguments)
	}
}
