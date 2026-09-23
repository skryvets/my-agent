package agent

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// reportFailures turns a failed tool into text for the model, which reads the
// error and tries something else, instead of ending the whole run.
func reportFailures(tools []tool.BaseTool) []tool.BaseTool {
	if len(tools) == 0 {
		return nil
	}
	wrapped := make([]tool.BaseTool, 0, len(tools))
	for _, each := range tools {
		wrapped = append(wrapped, utils.WrapToolWithErrorHandler(each, failure))
	}
	return wrapped
}

func failure(_ context.Context, err error) string { return "error: " + err.Error() }

// unknownTool answers a name the model invented.
func unknownTool(_ context.Context, name, _ string) (string, error) {
	return noSuchTool(name), nil
}

// noSuchTool is what the model reads when it asks for a name that is not a
// tool.
func noSuchTool(name string) string { return fmt.Sprintf("error: no tool named %q", name) }

// plain strips what eino wraps around a failure - the count of the attempts
// and the path of the node that made the call - because the text of the error
// goes straight into a chat.
func plain(err error) error {
	if errors.Is(err, adk.ErrExceedMaxIterations) {
		return fmt.Errorf("the model asked for tools %d times without an answer", maxRounds)
	}
	var exhausted *adk.RetryExhaustedError
	if errors.As(err, &exhausted) && exhausted.LastErr != nil {
		err = exhausted.LastErr
	}
	var refused *openai.APIError
	if errors.As(err, &refused) {
		return refused
	}
	return err
}

// retryable asks for another attempt on a failure. The caller who stopped the
// work gets none, because /stop must end a run at once. A request the server
// refused on its own terms gets none either: a wrong key or a malformed
// request fails the same way every time. Too many requests, a server failure
// and a broken connection are all tried again.
func retryable(ctx context.Context, err error) bool {
	if ctx.Err() != nil {
		return false
	}
	var refused *openai.APIError
	if errors.As(err, &refused) && refused.HTTPStatusCode >= http.StatusBadRequest &&
		refused.HTTPStatusCode < http.StatusInternalServerError {
		return refused.HTTPStatusCode == http.StatusTooManyRequests
	}
	return true
}
