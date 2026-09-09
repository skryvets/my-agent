# my-agent

A terminal and Telegram chatbot over the OpenRouter chat completions API. No
third-party Go dependencies - `go.mod` has no `require` block, and it should
stay that way unless there is a reason the standard library cannot cover.

## Layout

```
main.go                              flag parsing and wiring, nothing else
internal/agent/                      the model
  agent.go                           Client, Chat, DefaultModel
  stream.go                          server-sent events parsing
  reasoning.go                       reassembling streamed reasoning_details
  history.go                         a conversation in the API's wire format
internal/communication/terminal/     stdin and stdout connector
internal/communication/telegram/     Telegram bot connector
  telegram.go                        Run, configuration from the environment
  bot.go                             the getUpdates poll loop and its backoff
  session.go                         per-chat goroutine, commands, history
  client.go                          Bot API transport
  types.go                           Bot API wire types
  text.go                            splitting an answer to fit a message
```

## Rules

- `internal/communication/*` depends on `internal/agent`, never the reverse.
  Nothing under `internal/` imports `main`
- a connector takes the `Agent` interface (`Model() string`, `Chat(ctx, history,
  stream)`), declared in the connector that consumes it. Adding a connector is a
  new folder here plus a branch in `main.go` - do not touch `internal/agent`
- conversation state is `agent.History`. Build turns with `WithUser`,
  `WithAssistant`, `DropLast` and `Trim` rather than assembling
  `map[string]any` in a connector
- a failed turn is reported and dropped, never fatal. Only `main.go` calls
  `log.Fatal`
- one file, one concern. If a file grows past roughly 150 lines it is usually
  carrying two
- reasoning is deliberately disabled in `Chat`. The reassembly code in
  `reasoning.go` is kept for when it is switched back on - do not delete it as
  dead code, and do not enable reasoning without being asked
- the README carries two mermaid diagrams, one of the package structure and one
  of a single turn. Changing either shape means updating them

## Testing

`go test ./... -race` must be green before anything ships. Every package keeps
its coverage above 90%; `main.go` is wiring and is not covered.

Test files mirror source files (`session.go` / `session_test.go`). The shared
Telegram fake, an `httptest` server that records calls, lives in `fake_test.go`
alongside `fakeAgent` and `newTestBot`. No test reaches the network.

## Deployment

Railway worker service, single replica - only one instance may poll
`getUpdates`. Railpack builds the root package as `out` and `railway.json`
starts it, so `main.go` stays at the repository root.
