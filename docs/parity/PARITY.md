# Active Parity Backlog

This is the active work board for outstanding parity items. It intentionally excludes completed work and broad inventory. For the full snapshot, see `AUDIT.md`.

Baseline: `ai@7.0.58`, upstream commit `63db19387ba71ec50820d146658ae720ab50c80b`.

## Implemented In Current Worktree

- Core `Instructions` support for generate, stream, object generation, agents, prepare-step options, and tool-call repair options.
- AI SDK 7-style generate/stream accumulation for content, tool calls, tool results, sources, files, response messages, total usage, and `FinalStep`.
- `PrepareStep` message override carry-forward with per-step response messages.
- Stable include controls for request/response bodies and raw chunks, with `IncludeRawChunks` kept as a compatibility alias.
- `RefineToolInput`, tool metadata fields, canonical file tool-result fields, sandbox plumbing, no chunk telemetry event emission, and step performance fields.
- Official Anthropic provider package under `packages/anthropic`.
- Official Google Gemini Developer API language provider under `packages/google`.
- Community connector boundary under `packages/community/`, with initial OpenRouter chat/embedding connector.
- Bedrock replay updates for signed reasoning, cache-point preservation, and file tool-result content.
- Canary.159 core stream helper parity: standalone text/UI stream conversion, `OnEnd` callback aliasing, stream abort telemetry, optional sandbox spawning, output chunk timing metrics, and `output-error` UI replay validation.
- AI SDK 7.0.20 P0/P1 parity: client-safe UI error defaults, expanded SSRF guards, signed approval replay, deterministic tool ordering, tool-definition fingerprints/drift detection, stable lifecycle callbacks, empty-stream rejection, and streamed JSON extraction.
- Bounded AI SDK 7.0.58 core parity: injective approval signatures with safe
  legacy verification, cancellation fencing, semantic stream timeouts,
  metadata-only delta preservation, repeated tool-call ID handling, terminal
  tool-input replay tolerance, and stricter incomplete UI-tool filtering.
- Scoped test parity is complete for ported upstream/community capabilities; see `TEST_PARITY.md`.

## Queue

| Priority | Area | Label | Work item | Done when |
| --- | --- | --- | --- | --- |
| P1 | `packages/anthropic` | feature | Add Anthropic files/skills APIs and concrete sandbox-backed native tool helpers. | File/skill upload APIs and bash/code execution provider tools can use the configured Go sandbox. |
| P1 | `packages/community/openrouter` | fixture-needed | Broaden OpenRouter coverage beyond the ported-surface gate. | Anthropic-routed prompt caching, provider routing preferences, usage/cost/debug metadata, and error responses have fixture tests. |
| P1 | `tool-result` / files | behavior | Download file URL tool outputs when the target model cannot consume the URL directly. | File URL outputs use the existing download hook and model supported-URL map before being replayed as tool results. |
| P1 | `sandbox` | behavior | Add concrete sandbox-backed provider tool helpers for Anthropic. | Bash/code execution provider tools can receive the configured Go sandbox and have tests for read/write/run-command calls. |
| P1 | `telemetry` | go-native | Add a Go-native telemetry dispatcher/registry and tool execution context events. | Telemetry can be installed globally or per call; language/tool/model/tool-execution events carry filtered attributes with runtime/tools context only when enabled. |
| P1 | `packages/vertex` | fixture-needed | Add targeted Vertex parity fixtures for the canary.152 audit deltas. | Tests cover service-tier metadata, no-arg tool calls with thought signatures, code execution metadata, grounding metadata, cached-token usage, and project/base URL propagation. |
| P1 | `packages/bedrock` | feature | Add unported Bedrock subproviders and broaden edge fixtures as those surfaces land. | Mantle, embedding, image, and reranking surfaces exist or are explicitly left out of scope; optional-input and structured-output edge cases have provider tests if implemented. |
| P2 | `tool` | behavior | Support flexible tool descriptions and optional tool context. | Tool descriptions accept the current upstream flexible shape or a documented Go-native equivalent; tools without context validate and execute with nil/empty context safely. |
| P2 | `error` | fixture-needed | Tighten exact error taxonomy and message text coverage. | Named error guards exist for provider/provider-utils errors and fixture tests assert message/cause/status fields for branchable edge cases. |
| P2 | `text-stream` / HTTP | fixture-needed | Broaden HTTP response fixture coverage. | Tests cover headers, status text, flush behavior, error propagation, context cancellation, text stream, data stream, completion stream, and UI stream interop. |
| P2 | `transcribe` | feature | Add streaming transcription through an optional Go streaming model interface. | Delta/partial/final transcript parts, cancellation, raw chunks, metadata, and unsupported-model behavior have tests. |
| P2 | `generate-video` | feature | Add batching, frame images, image/video references, audio generation, and URL download normalization. | Provider call options and aggregation match applicable AI SDK 7 behavior. |
| P2 | `generate-text` / tools | behavior | Evaluate experimental tool callers/code-mode routing and tool input lifecycle callbacks from `ai@7.0.58`. | A Go-native caller boundary and callback ordering are designed without broadening provider packages, with focused tests. |
| P2 | `prepare-step` / middleware | behavior | Evaluate per-step language-model call-setting overrides and default-instructions middleware from `ai@7.0.58`. | The Go API has an explicit override/precedence contract and regression tests. |
| P3 | realtime | n/a-go | Keep browser sessions out of scope; evaluate provider/factory contracts and server token/tool helpers separately. | The browser-only boundary and any Go-native experimental subset are documented. |
| P3 | browser transports/hooks | n/a-go | Keep React/browser hooks and browser transports documented as intentionally absent. | `useChat`, `useCompletion`, browser `DefaultChatTransport`, and direct browser fetch helpers are listed as `n/a-go` with server helper replacements named. |

## Current Blockers

None.
