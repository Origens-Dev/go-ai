# Test Parity Matrix

This file tracks test parity for upstream capabilities that have a Go implementation in this repo. It does not count TypeScript-only type tests, React/browser transports, Node stream helpers with no Go API, or provider families that have not been ported.

Upstream reference: Vercel AI SDK `ai@7.0.58` at
`63db19387ba71ec50820d146658ae720ab50c80b`, compared from `ai@7.0.20` for the
bounded agent/tool/approval/stream/server-UI scope.

## Coverage Summary

The earlier numeric file-count snapshot was tied to canary.152 and is no longer presented as a current percentage. The current release gate is behavioral: every ported P0/P1 capability listed below has focused Go coverage and `go test ./...` must pass. P2 streaming transcription, expanded video, and experimental realtime tests enter the gate only when those APIs are ported.

| Area | Current gate |
| --- | --- |
| `packages/ai` P0/P1 core/runtime | Signed approvals and collision resistance, legacy approval verification, cancellation fencing, error redaction, SSRF regressions, deterministic tools, drift fingerprints, lifecycle aliases, semantic stream timeouts, metadata-only deltas, empty streams, streaming JSON extraction, and persisted UI-tool replay |
| Existing provider packages | Existing Bedrock, Vertex, Anthropic, and community OpenRouter suites remain required |

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
- `packages/ai/tool_security_test.go`
- `packages/anthropic/anthropic_test.go`
- `packages/community/openrouter/openrouter_test.go`
- `packages/bedrock/bedrock_test.go`

Verification command:

```sh
GOCACHE=/private/tmp/go-ai-build-cache go test ./...
go test -race ./packages/ai
```
