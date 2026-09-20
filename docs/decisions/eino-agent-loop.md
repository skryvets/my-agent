# Decisions - the agentic work moves to eino

## Why a library at all
**Q:** Why does a project with an empty `require` block take a dependency now?
**A:** To cut complexity and lines of code. The hand-written server-sent events parser, the `tool_calls` reassembly, the `reasoning_details` reassembly, the tool registry and the tool-calling loop came to about 420 lines that a library already covers. `internal/agent` and `internal/tools` went from 783 lines to 498, and the loop gained retries for free.
**Source:** user (2026-09-19)

## Which library
**Q:** Which agent library?
**A:** [eino](https://github.com/cloudwego/eino), with `eino-ext/components/model/openai` pointed at the OpenRouter base URL. The user supplied a working example of that pairing.
**Source:** user (2026-09-19)

## What is deleted
**Q:** Which files go?
**A:** `internal/agent/stream.go`, `toolcall.go`, `reasoning.go`, `tool.go` and `loop.go`, with their tests. eino parses the stream, joins the `tool_calls` fragments, joins the reasoning chunks with `schema.ConcatMessages`, holds the tool list and runs the loop.

`reasoning.go` was kept before as the code that reassembles `reasoning_details` for the day reasoning is switched on. eino does that work now, so the file is dead in the exact sense the earlier rule wanted to avoid: code with no caller and no future caller. Reasoning itself stays off.
**Source:** model (2026-09-19)

## What the connectors see
**Q:** Does a connector change?
**A:** Almost not at all. The `Agent` interface is still `Model() string` and `Chat(ctx, history, stream)`. `agent.History` is now `[]*schema.Message` instead of `[]map[string]any`, and `WithToolCalls` is gone, because only the loop used it. `WithUser`, `WithAssistant`, `DropLast` and `Trim` are unchanged.
**Source:** model (2026-09-19)

## Retries
**Q:** What does a failed model call do?
**A:** eino tries it again, up to three times, with its exponential backoff and jitter: 100 ms, then double each time, to a 10 s ceiling. Two failures get no second attempt: a call whose context the caller cancelled, because `/stop` must end a run at once, and a request the server refused with a 4xx other than 429, because a wrong key or a malformed request fails the same way every time. After the last attempt the failure reaches the chat as before, and the turn is dropped.
**Source:** user asked for retries (2026-09-19); the numbers are the eino defaults

## A tool that fails
**Q:** How does a tool failure reach the model?
**A:** `agent.New` wraps every tool with `utils.WrapToolWithErrorHandler`, which turns the error into `error: <text>` for the model. `UnknownToolsHandler` answers a name the model invented with `error: no tool named "<name>"`. Both keep the old behaviour: the model reads the failure and tries something else.
**Source:** model (2026-09-19)

## A tool result the runner does not report
**Q:** Why does `chat.go` fill in a missing tool result?
**A:** The eino runner answers a hallucinated tool name without emitting an event for it. The assistant turn that asked for the call would then reach the history with no result beside it, and the next request would be malformed. `answered` adds the missing result before `Chat` returns.
**Source:** model (2026-09-19)

## The text of a failure
**Q:** Why does `agent.go` unwrap the error before it returns?
**A:** eino wraps a failure in the path of the node that made the call, and in the count of the attempts, which reads as `[NodeRunError] exceeds max retries: ... node path: [node_1]`. That text goes straight into a Telegram chat. `plain` unwraps it to the API error, so the chat reads `error, status code: 401, ... Missing Authentication header`.
**Source:** model (2026-09-19)

## The arguments schema of a tool
**Q:** Who writes the JSON Schema of a tool?
**A:** Nobody. `utils.InferTool` reads it from a Go struct with `jsonschema` tags. A `description` in such a tag carries no comma, because a comma separates the options of the tag.
**Source:** model (2026-09-19)

## The rest of the project
**Q:** Does `internal/sandbox` move to the Docker SDK?
**A:** No. One library for the agentic work is the whole budget. The Docker Engine API stays on `net/http` and its own `DialContext`, and `internal/devcontainer` stays on `encoding/json`.
**Source:** model (2026-09-19)
