# Upstream Baseline

This repo tracks Vercel AI SDK `main` as the upstream behavioral reference.

| Field | Value |
| --- | --- |
| Upstream checkout | Local `/Users/dholbrook/src/ai` refreshed from GitHub `vercel/ai` `main` |
| Upstream package version | `ai@7.0.0-canary.159` |
| Upstream commit | `ab6d66482` |
| Upstream working tree | Local checkout, clean after fast-forward |
| Checked on | 2026-06-01 |
| Go module | `github.com/holbrookab/go-ai` |

## Compared Scope

Primary scope:

- `packages/ai/src` at `vercel/ai@ab6d66482`
- `packages/amazon-bedrock`
- `packages/anthropic`
- `packages/google-vertex`
- shared provider/provider-utils behavior as needed by the Go public surface

Community-provider comparison scope:

- OpenRouter's community AI SDK provider repository, used as the behavioral reference for `packages/community/openrouter`

The high-signal upstream delta since the prior canary.152 automation baseline is concentrated in core `packages/ai`: standalone stream conversion helpers, `OnEnd` and abort callback semantics, language-model-call telemetry wrapping, sandbox process spawning, output chunk timing metrics, and `output-error` UI message replay validation.

Current Go implementation scope:

- `packages/ai`
- `packages/anthropic`
- `packages/bedrock`
- `packages/vertex`
- `packages/community/openrouter`
- `internal/httputil`
- `internal/retry`
- `packages/bedrock/internal/sigv4`

## Tracking Rules

- `UPSTREAM.md` records the comparison baseline, not outstanding work.
- `PARITY.md` records outstanding work only.
- `AUDIT.md` records the broader state of the port, including completed and Go-native areas.
- `TEST_PARITY.md` records scoped upstream test coverage for ported capabilities.
- If upstream is bumped, update this file first, then refresh `AUDIT.md`, then create or adjust active backlog rows in `PARITY.md`.

## Known Go-Native Differences

- TypeScript compile-time inference tests are replaced by Go compile/runtime tests and examples.
- React, RSC, browser transports, and hooks are not runtime Go packages. Server-side HTTP helpers and UI stream primitives are the Go replacement surface.
- Node/Web stream helpers map to Go channels, `io.Reader`, `http.Response`, and `http.ResponseWriter` helpers.
