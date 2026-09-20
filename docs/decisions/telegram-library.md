# Decisions - the Telegram connector moves to go-telegram/bot

## Why a library at all
**Q:** Why does the connector take a dependency now?
**A:** To cut complexity and lines of code. The hand-written Bot API transport, the wire types and the poll loop with its backoff came to 222 lines that a library already covers. The package went from 1423 lines to 1084, and lost 3 files.

## Which library
**Q:** Which Telegram library?
**A:** [go-telegram/bot](https://github.com/go-telegram/bot) v1.27.0. It has no dependencies of its own, it covers the whole Bot API, and it polls, backs off and obeys `retry_after` on its own.

## What is deleted
**Q:** Which files go?
**A:** `client.go` (the Bot API transport), `types.go` (the wire types) and `bot.go` (the poll loop, its backoff and its `retry_after`), with their tests. `bot.New` and `bot.Start` do that work.

## What stays
**Q:** What does the connector still own?
**A:** The per-chat session with its queue and its history, the commands, the allowlist, and `splitMessage`, because a Bot API message holds 4096 characters and the library does not split a long answer.

## The order of the messages
**Q:** The library runs each handler in its own goroutine. Two quick messages in one chat could then reach the queue out of order.
**A:** The bot is built with `WithNotAsyncHandlers`, so one handler runs at a time and the order holds. `dispatch` only queues a message, so the poll loop never waits for an answer, and `/stop` is still read as it arrives.

## The fake in the tests
**Q:** How do the tests reach the bot without the network?
**A:** `newTestBot` points `bot.New` at the `httptest` server of `fakeTelegram` with `WithServerURL`, and skips the `getMe` call at startup with `WithSkipGetMe`. The library posts a multipart form, so the fake records the form fields of each call instead of a JSON body.
