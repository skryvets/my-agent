# Decisions - one session for every connector, one container for every run

## Why a refactor
**Q:** The code worked. Why change its shape?
**A:** The terminal and Telegram did not do the same things: `/task` worked from a phone and not from the keyboard. The refactor is four commits, each one a single change a reader can follow.

## The answer is text
**Q:** `Chat` returned a `Message` with the answer and the tool turns in `Steps`, and `WithAssistant` put the steps back into the history. Why drop that?
**A:** Nothing replayed the steps. A chat offers the model no tools, and a task asks one question and throws the history away. `Chat` returns the text of the answer, and the tool calls of a turn stay inside eino, which holds them for the rounds of one `Run`. The code that filled in a missing tool result for the history went with it.

## The session
**Q:** Where do the commands live, so the terminal and Telegram behave the same?
**A:** In `internal/session`. A `Session` owns the history of one person and answers everything that person can say: a chat turn, `/help`, `/reset`, `/task`. A connector reads messages, makes one `Session` for each person, hands it every message, and sends out what `Reply` gets. The `Agent` and `Tasks` interfaces move there, so a connector declares nothing. The package is not `internal/agent`, because a session runs tasks and `task` imports `agent`.

## What stays in Telegram
**Q:** Why does Telegram keep a queue and `/stop`?
**A:** Many chats share one poll loop, and a `/stop` must be read while the queue of its chat waits behind the message it has to stop. The terminal reads the next line only after the answer, so it has nothing to stop; Ctrl-C ends the process. `/stop` reaches a `Session` only when nothing runs, and answers "Nothing to stop.". The typing indicator, the allowlist and the split of a long reply are Telegram's too.

## The terminal streams, Telegram does not
**Q:** How does one `Session` serve a connector that streams and one that cannot?
**A:** `Session.Stream` is where an answer appears while it arrives. The terminal sets it to stdout. Telegram leaves it nil, and the session then sends the whole answer through `Reply`. Everything else, an error, a progress line, help, goes through `Reply` in both.

## One container for every run
**Q:** The tools were built once on a `Pool`, which found the container of a call through a key in the context, and a reaper removed the containers that went quiet. Why is that gone?
**A:** Only a task uses a container, for as long as the run lasts, and the run removes its container when it ends. The pool, the reaper and the context key were for a chat that had tools, which no chat has. A run now starts its own container, `tools.All` binds the three tools to it, and `Runner.Worker` builds an agent over them. Nothing is shared between two runs, so nothing has to say which run a call belongs to. `Docker.Sweep` removes what a stopped process left behind, at start and at exit.

## The adapter in main
**Q:** `Docker.Start` returns a `*sandbox.Container`, and `task.Sandbox` asks for a `task.Container`. Why the `containers` type in `main.go`?
**A:** Go reads a return type by name, so a method that returns `*sandbox.Container` does not satisfy an interface method that returns `task.Container`, although the value would. The one-line adapter is wiring, which is what `main.go` is for. The alternative, a function field on `Runner`, is the same line in another place.

## Coverage of the Telegram package
**Q:** `Run` builds the bot from the environment and cannot be tested without the network, because `bot.New` calls `getMe`. The package fell below 90%. What changed?
**A:** `newBot` builds the `Bot` without its API client, so `Run` and the tests share it, and the warning about an empty allowlist lives there. A fake that refuses `sendChatAction` covers the log line of `typing`. `Run` itself stays untested, as it was.
