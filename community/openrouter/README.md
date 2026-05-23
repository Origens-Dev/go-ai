# OpenRouter Community Provider

`community/openrouter` implements OpenRouter as a community connector.

```go
provider := openrouter.New(openrouter.Settings{
	APIKey: os.Getenv("OPENROUTER_API_KEY"),
})

result, err := ai.GenerateText(ctx, ai.GenerateTextOptions{
	Model:  provider.LanguageModel("openai/gpt-4o-mini"),
	Prompt: "Say hello from Go.",
})
```

## Supported Surface

- OpenAI-compatible chat generation and streaming through `/api/v1/chat/completions`;
- embeddings through `/api/v1/embeddings`;
- `OPENROUTER_API_KEY`, custom base URL, headers, and HTTP client;
- OpenAI-style tool calls and tool results;
- OpenRouter provider options under `ProviderOptions["openrouter"]` or `ProviderOptions["openrouter.chat"]`;
- provider metadata for model, usage, reasoning details, and OpenRouter usage/cost payloads when returned by the API.

Community connectors are tested and documented, but they may lag first-class provider parity when upstream provider APIs change quickly.
