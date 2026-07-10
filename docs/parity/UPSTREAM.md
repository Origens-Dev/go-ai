# Upstream Baseline

This repo tracks Vercel AI SDK `main` as the upstream behavioral reference.

| Field | Value |
| --- | --- |
| Upstream checkout | Local `/Users/dholbrook/src/ai` refreshed from GitHub `vercel/ai` `main` |
| Upstream package version | `ai@7.0.20` |
| Upstream commit | `58d77caf6733f49431b0864bd71adbe143958aeb` |
| Upstream working tree | Local checkout, clean after fast-forward |
| Checked on | 2026-07-10 |
| Go module | `github.com/holbrookab/go-ai` |

## Compared Scope

Primary scope:

- `packages/ai/src` at `vercel/ai@58d77caf6733f49431b0864bd71adbe143958aeb`
- `packages/amazon-bedrock`
- `packages/anthropic`
- `packages/google-vertex`
- shared provider/provider-utils behavior as needed by the Go public surface

Community-provider comparison scope:

- OpenRouter's community AI SDK provider repository, used as the behavioral reference for `packages/community/openrouter`

The high-signal delta since canary.159 includes signed approval replay, client-error redaction, download SSRF hardening, deterministic tool ordering, tool-definition drift detection, stable lifecycle callbacks, empty-stream rejection, streaming JSON extraction, streaming transcription, expanded video inputs, and experimental realtime APIs. The P0/P1 server-side core items are ported; streaming transcription, expanded video generation, and browser-oriented realtime work remain outside this baseline update.

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
