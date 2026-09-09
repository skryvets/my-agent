# my-agent

A small Go program that talks to a reasoning model through the [OpenRouter](https://openrouter.ai) chat completions API and streams the reply to the terminal.

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

The program asks a question, prints the reasoning and the answer as they stream in, then sends a second turn ("Are you sure?") with the previous reasoning attached.

Reasoning output is printed under a `--- reasoning ---` header and the final answer under `--- answer ---`.

## Configuration

The model is set by the `model` constant in `agent.go`.
