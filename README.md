# go-ai

A Go port of Vercel's AI SDK, with the public surface organized under `packages/*` to match the upstream TypeScript repo where that shape makes sense in Go.

This Origens-maintained lineage starts at `v0.1.0-alpha.1`. It is based on
[`github.com/holbrookab/go-ai`](https://github.com/holbrookab/go-ai) `v0.3.0`
(`f332bd84f1dd40bdc7882517e82b2ad7dafeff83`) and retains that project's
Apache-2.0 license, attribution, and history.

- `packages/ai`: generation, streaming, object generation, embeddings, tool loops, agents, UI message streams, middleware, prompt normalization, retries, stop conditions, and shared model/provider contracts.
- `packages/anthropic`: Anthropic Messages provider.
- `packages/bedrock`: Amazon Bedrock Converse provider.
- `packages/google`: Google Gemini Developer API provider for Google AI Studio API keys.
- `packages/vertex`: Google Vertex AI Gemini provider.
- `packages/community/openrouter`: OpenRouter community connector for chat and embeddings.

## Parity Tracking

Parity work lives in [`docs/parity`](docs/parity/README.md). That directory is the source of truth for what this port has been compared against and what remains:

- [`docs/parity/UPSTREAM.md`](docs/parity/UPSTREAM.md): upstream AI SDK version, commit, local path, and comparison scope.
- [`docs/parity/PARITY.md`](docs/parity/PARITY.md): active backlog only. Treat it like the JIRA board for unfinished parity work.
- [`docs/parity/AUDIT.md`](docs/parity/AUDIT.md): broader snapshot of implemented, Go-native, fixture-needed, and intentionally non-Go surfaces.
- [`docs/parity/TEST_PARITY.md`](docs/parity/TEST_PARITY.md): scoped upstream test-file parity for ported capabilities.

When updating parity, refresh `UPSTREAM.md` first, update `AUDIT.md` as the broad reference, and keep `PARITY.md` limited to actionable outstanding work. Completed rows should come out of `PARITY.md` instead of accumulating as history.

## Generate Text

```go
package main

import (
	"context"
	"fmt"

	"github.com/Origens-Dev/go-ai/packages/ai"
	"github.com/Origens-Dev/go-ai/packages/vertex"
)

func main() {
	provider := vertex.New(vertex.Settings{
		Project:  "my-project",
		Location: "global",
	})

	result, err := ai.GenerateText(context.Background(), ai.GenerateTextOptions{
		Model:        provider.LanguageModel("gemini-2.5-flash"),
		Instructions: "You are concise.",
		Prompt:       "Say hello from Go.",
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(result.Text)
}
```

## Tool Loop

Use `StopWhen: []ai.StopCondition{ai.LoopFinished()}` to allow the model to call client-side tools until the model stops asking for tools.

```go
result, err := ai.GenerateText(ctx, ai.GenerateTextOptions{
	Model:    model,
	Prompt:   "What is the weather in NYC?",
	StopWhen: []ai.StopCondition{ai.LoopFinished()},
	Tools: map[string]ai.Tool{
		"weather": {
			Description: "Get the weather for a city.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"city": map[string]any{"type": "string"},
				},
				"required": []string{"city"},
			},
			Execute: func(ctx context.Context, call ai.ToolCall, opts ai.ToolExecutionOptions) (any, error) {
				return "sunny", nil
			},
		},
	},
})
```

## Provider Auth

Anthropic auth uses an explicit API key, `ANTHROPIC_API_KEY`, an explicit bearer token, or `ANTHROPIC_AUTH_TOKEN`.

Bedrock auth follows the upstream SDK precedence: explicit API key, `AWS_BEARER_TOKEN_BEDROCK`, then SigV4 via credential provider, explicit keys, or AWS environment variables.

Vertex auth uses `GOOGLE_VERTEX_API_KEY` or explicit API key for Express Mode. Otherwise it uses an injected token source, `GOOGLE_VERTEX_ACCESS_TOKEN`, service account JSON from `GOOGLE_APPLICATION_CREDENTIALS`, or the metadata server.

Google Gemini Developer API auth uses an explicit API key or
`GOOGLE_GENERATIVE_AI_API_KEY`. This is the connector for keys created in
Google AI Studio; it calls `generativelanguage.googleapis.com` and is separate
from Vertex AI authentication and endpoints.

OpenRouter community auth uses an explicit API key or `OPENROUTER_API_KEY`.
Its connector also supports OpenRouter app attribution with `AppName`,
`AppURL`, and `AppCategories`, plus chat session attribution with `SessionID`
or provider options such as `ProviderOptions["openrouter"]["sessionId"]`.

## Provider Boundaries

First-class providers live directly under `packages/` and are included in parity tracking with the upstream AI SDK provider packages. Community connectors live under `packages/community/`; they use the same core interfaces and include tests/docs, but have a lighter maintenance promise because their upstream package ownership and release cadence are outside Vercel AI SDK.

## License

This project is licensed under Apache-2.0. It descends from
`github.com/holbrookab/go-ai` `v0.3.0`, and portions are derived from Vercel AI
SDK. See [LICENSE](LICENSE) and [NOTICE](NOTICE) for attribution.
