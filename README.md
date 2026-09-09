# my-agent

A small terminal chatbot that talks to a reasoning model through the [OpenRouter](https://openrouter.ai) chat completions API and streams the reply as it arrives.

It shows two things that are easy to get wrong:

- server-sent events parsing, including OpenRouter keep-alive comment lines
- reassembling streamed `reasoning_details` fragments so the model's thinking can be replayed in a follow-up turn

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

## Configuration

The model is set by the `model` constant in `agent.go`.
