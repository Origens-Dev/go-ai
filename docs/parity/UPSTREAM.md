# Upstream Baseline

This repo tracks Vercel AI SDK `main` as the upstream behavioral reference.

| Field | Value |
| --- | --- |
| Upstream checkout | Local `/Users/dholbrook/src/ai` with the GitHub `vercel/ai` release tag fetched |
| Upstream package version | `ai@7.0.58` |
| Upstream commit | `63db19387ba71ec50820d146658ae720ab50c80b` |
| Compared range | `ai@7.0.20..ai@7.0.58` |
| Checked on | 2026-08-09 |
| Go module | `github.com/Origens-Dev/go-ai` |
| Downstream baseline | `github.com/holbrookab/go-ai` `v0.3.0` (`f332bd84f1dd40bdc7882517e82b2ad7dafeff83`) |

## Compared Scope

Primary scope:

- `packages/ai/src` from `ai@7.0.20` through `ai@7.0.58`, bounded to
  agent, tool/approval, stream, and server-side UI behavior
- shared provider/provider-utils types only where needed to understand those
  existing core behaviors

Community-provider comparison scope:

- OpenRouter's community AI SDK provider repository, used as the behavioral reference for `packages/community/openrouter`

The high-signal bounded delta includes injective approval signatures, approval
denial replay fixes, cancellation correctness, settings-level agent timeout
forwarding, semantic first/inter-chunk streaming timeouts, metadata-only text
delta preservation, repeated tool-call ID handling, and persisted UI-tool replay
fixes. Experimental tool callers/code mode, batch and translation APIs, expanded
video/transcription, realtime, and browser chat state management remain outside
this update.

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
- Provider package deltas were not audited or expanded in this bounded core pass.
