# my-agent

A small chatbot that talks to a reasoning model through the [OpenRouter](https://openrouter.ai) chat completions API and streams the reply as it arrives.

It shows two things that are easy to get wrong:

- server-sent events parsing, including OpenRouter keep-alive comment lines
- reassembling streamed `reasoning_details` fragments so the model's thinking can be replayed in a follow-up turn

It runs in the terminal, or as a Telegram bot.

## Requirements

- Go 1.26 or newer
- An OpenRouter API key

## Usage

```sh
export OPENROUTER_API_KEY=sk-or-...
go run .
```

Type a message at the `you>` prompt and press enter. The whole conversation, including the reassembled `reasoning_details`, is sent back on every turn, so the model can follow up on its own thinking. Ctrl-C or Ctrl-D exits.

Reasoning output is printed under a `--- reasoning ---` header and the final answer under `--- answer ---`. A failed turn prints the error and drops the unanswered message, leaving the session alive.

## Telegram bot

Talk to [@BotFather](https://t.me/BotFather), send `/newbot`, and keep the token it gives you.

```sh
export OPENROUTER_API_KEY=sk-or-...
export TELEGRAM_BOT_TOKEN=123456:ABC-DEF...
export TELEGRAM_ALLOWED_USERS=11111111,22222222
go run . -telegram
```

The bot long-polls `getUpdates` for `message` updates, keeps one conversation per chat, and answers each message with the model. Commands:

- `/start`, `/help` - what the bot does
- `/reset` - forget the conversation in that chat

Details worth knowing:

- `TELEGRAM_ALLOWED_USERS` is a comma-separated list of numeric Telegram user ids. Anyone can find a bot by its username, so without an allowlist strangers spend your OpenRouter credits. Ask [@userinfobot](https://t.me/userinfobot) for your id
- each chat is served by its own goroutine, so a slow answer in one chat does not block another. Messages inside one chat are answered in order, and a sender who runs ahead of the queue is told to wait
- history is capped at the last 10 turns per chat and lives in memory only, so a restart clears it
- answers longer than Telegram's 4096 character limit are split on the last blank line, newline or space that fits
- a `429` from Telegram is retried after the `retry_after` it returns, other poll failures back off up to a minute
- Ctrl-C or `SIGTERM` stops the bot

## Deploying to Railway

The bot has no HTTP server, so it is a worker service: no port, no healthcheck. Railpack builds the Go binary as `out`, and `railway.json` starts it with the flag:

```json
{
  "$schema": "https://railway.com/railway.schema.json",
  "deploy": {
    "startCommand": "./out -telegram",
    "restartPolicyType": "ALWAYS"
  }
}
```

Set `OPENROUTER_API_KEY`, `TELEGRAM_BOT_TOKEN` and `TELEGRAM_ALLOWED_USERS` as service variables. The same command can be typed into Settings -> Deploy -> Custom Start Command instead, but the checked-in file survives a service being recreated.

Only one instance may poll `getUpdates` at a time, so keep the service at a single replica.

## Configuration

The model is set by the `model` constant in `agent.go`.
