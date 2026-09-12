package agent

import (
	"context"
	"fmt"
	"io"
)

// maxToolRounds caps one Chat call, so a model that keeps asking for tools
// cannot run forever.
const maxToolRounds = 10

// Chat answers one user turn. The model may ask for tools first: each round
// runs the calls it asked for, appends the results and asks the model again,
// until it answers in words or the round cap is reached. The answer is written
// to stream as it arrives.
func (c *Client) Chat(ctx context.Context, history History, stream io.Writer) (Message, error) {
	var steps History

	for round := 0; ; round++ {
		turns := make(History, 0, len(history)+len(steps))
		turns = append(turns, history...)
		turns = append(turns, steps...)

		msg, err := c.complete(ctx, turns, stream)
		if err != nil {
			return Message{}, err
		}
		if len(msg.ToolCalls) == 0 {
			msg.Steps = steps
			return msg, nil
		}
		if round == maxToolRounds {
			return Message{}, fmt.Errorf("stopped after %d rounds of tool calls", maxToolRounds)
		}

		steps = steps.WithToolCalls(msg)
		announce(stream, msg.ToolCalls)
		steps = append(steps, c.tools.run(ctx, msg.ToolCalls)...)
	}
}

// announce names the tools of one round on the stream the answer goes to, so a
// long run shows what it is doing.
func announce(stream io.Writer, calls []map[string]any) {
	for _, call := range calls {
		name, arguments := callFunction(call)
		fmt.Fprintf(stream, "--- tool: %s %s ---\n", name, arguments)
	}
}
