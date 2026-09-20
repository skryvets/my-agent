# Decisions - the Telegram connector moves to go-telegram/bot

## Why a library at all
**Q:** Why does the connector take a dependency now?
**A:** To cut complexity and lines of code. The hand-written Bot API transport, the wire types and the poll loop with its backoff came to 222 lines that a library already covers. The package went from 1423 lines to 1084, and lost 3 files.
**Source:** user (2026-09-19)

## Which library
**Q:** Which Telegram library?
**A:** [go-telegram/bot](https://github.com/go-telegram/bot) v1.27.0. The user supplied it. It has no dependencies of its own, it covers the whole Bot API, and it polls, backs off and obeys `retry_after` on its own.
**Source:** user (2026-09-19)

## What is deleted
**Q:** Which files go?
**A:** `client.go` (the Bot API transport), `types.go` (the wire types) and `bot.go` (the poll loop, its backoff and its `retry_after`), with their tests. `bot.New` and `bot.Start` do that work.
**Source:** model (2026-09-19)

## What stays
**Q:** What does the connector still own?
**A:** The per-chat session with its queue and its history, the commands, the allowlist, and `splitMessage`, because a Bot API message holds 4096 characters and the library does not split a long answer.
**Source:** model (2026-09-19)

## The order of the messages
**Q:** The library runs each handler in its own goroutine. Two quick messages in one chat could then reach the queue out of order.
**A:** The bot is built with `WithNotAsyncHandlers`, so one handler runs at a time and the order holds. `dispatch` only queues a message, so the poll loop never waits for an answer, and `/stop` is still read as it arrives.
**Source:** model (2026-09-19)

## The fake in the tests
**Q:** How do the tests reach the bot without the network?
**A:** `newTestBot` points `bot.New` at the `httptest` server of `fakeTelegram` with `WithServerURL`, and skips the `getMe` call at startup with `WithSkipGetMe`. The library posts a multipart form, so the fake records the form fields of each call instead of a JSON body.
**Source:** model (2026-09-19)
