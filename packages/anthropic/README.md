# Anthropic Provider

`packages/anthropic` implements the Anthropic Messages API as a first-class go-ai provider.

```go
provider := anthropic.New(anthropic.Settings{
	APIKey: os.Getenv("ANTHROPIC_API_KEY"),
})

result, err := ai.GenerateText(ctx, ai.GenerateTextOptions{
	Model:        provider.LanguageModel("claude-sonnet-4-5"),
	Instructions: "You are concise.",
	Prompt:       "Say hello from Go.",
})
```

## Supported Surface

- text generation and streaming via `/v1/messages`;
- `x-api-key` and bearer auth;
- custom `BaseURL`, headers, and HTTP client;
- text, image, and document input blocks from go-ai message parts;
- tool calls and tool results, including file tool-result output blocks;
- reasoning/thinking parts and replay signatures;
- provider options under `ProviderOptions["anthropic"]` or `ProviderOptions["anthropic.messages"]`.

Anthropic file and skill upload APIs are not part of this first pass.
