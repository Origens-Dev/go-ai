# Test Parity Matrix

This file tracks test parity for upstream capabilities that have a Go implementation in this repo. It does not count TypeScript-only type tests, React/browser transports, Node stream helpers with no Go API, or provider families that have not been ported.

Upstream reference: Vercel AI SDK `main` at `9f1e1ba4b93b514f6cca1c8452e6a1fb23e44907`.

## Coverage Summary

| Area | Applicable upstream test files | Go-covered files | Coverage |
| --- | ---: | ---: | ---: |
| `packages/ai` core/runtime | 82 | 82 | 100.0% |
| `packages/bedrock` text/Converse provider | 10 | 10 | 100.0% |
| `packages/vertex` Gemini text provider | 5 | 5 | 100.0% |
| `packages/anthropic` Messages text provider | 8 | 8 | 100.0% |
| `packages/community/openrouter` chat/embedding connector | 3 | 3 | 100.0% |
| **Total** | **108** | **108** | **100.0%** |

The total is the release gate: current scoped test parity is complete for the upstream/community surfaces that have Go implementations in this repo.

## Scope Rules

- Counted: upstream runtime test files whose behavior maps to an existing Go package or API.
- Counted as covered: there is a Go test exercising the same externally visible behavior, even when the Go test is grouped differently than upstream.
- Excluded: `.test-d.ts`, browser hooks/transports, React/Svelte/Vue/Angular packages, MCP apps, workflow packages, OpenTelemetry package internals, provider packages not present in this repo, and provider subfamilies this repo does not implement.
- OpenRouter is not a Vercel-owned upstream package, so its count is based on local community connector capabilities: chat generate, chat stream, and embeddings.

## Excluded From The Ported-Surface Gate

| Area | Gap | Why it remains |
| --- | --- | --- |
| `packages/ai` telemetry | Dispatcher/registry and diagnostic-channel exact parity | Go has per-call event recorder hooks and filtering tests; the TS global diagnostic channel architecture is not a Go API. |
| `packages/ai` util | Async iterable / ReadableStream helpers | These map to Go channels/readers; Go channel, HTTP stream, and UI stream behavior are covered in existing package tests. |
| `packages/ai` UI | Browser transport tests | Intentional `n/a-go`; server-side chat/UI stream behavior is covered. |
| `packages/bedrock` | Mantle, embedding, image, reranking subproviders | Not implemented in this repo's Bedrock connector. |
| `packages/anthropic` | Files/skills APIs and full sandbox-backed native tool helpers | Explicitly out of first-pass provider scope; text, stream, tools, reasoning, and file tool results are covered. |
| `packages/vertex` | Edge/MaaS/Anthropic/XAI/image/video/embedding subproviders | Not implemented in this repo's Vertex connector. |

## Current Evidence

Current Go test files added or expanded for parity:

- `packages/ai/parity_test.go`
- `packages/ai/tool_parity_test.go`
- `packages/ai/include_telemetry_parity_test.go`
- `packages/anthropic/anthropic_test.go`
- `packages/community/openrouter/openrouter_test.go`
- `packages/bedrock/bedrock_test.go`

Verification command:

```sh
GOCACHE=/private/tmp/go-ai-build-cache go test ./...
```
