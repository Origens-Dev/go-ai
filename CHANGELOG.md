# Changelog

## 0.2.7 - 2026-06-01

- Fixed the OpenRouter community connector to send schema-constrained structured output requests when `ResponseFormat.Schema` is present, while preserving JSON-object mode when no schema is supplied.

## 0.2.6 - 2026-06-01

- Added standalone stream conversion helpers for text and UI message streams.
- Added `OnEnd` callback support with `OnFinish` kept as a compatibility alias, plus stream abort callbacks and telemetry.
- Added optional sandbox process spawning support without breaking existing sandbox implementations.
- Expanded stream performance metrics with time-to-first-output and output chunk timing stats.
- Preserved tool metadata through UI stream conversion and persisted UI tool parts.
- Fixed UI message validation so persisted `output-error` tool parts do not revalidate invalid inputs during replay.

## 0.2.5 - 2026-05-23

- Added AI SDK 7 parity providers and tests, including Anthropic and OpenRouter coverage.
- Moved OpenRouter into the `packages/community` namespace.

## 0.2.4 - 2026-05-05

- Added task-step streaming metadata.

## 0.2.3 - 2026-05-05

- Fixed Vertex thought signature round-tripping for replayed assistant text, reasoning, file, and function call parts.

## 0.2.2 - 2026-05-05

- Fixed Vertex message mapping so provider-facing messages use `Message.Text` as a fallback and skip empty content entries instead of sending empty `parts` arrays.

## 0.2.1 - 2026-05-05

- Aligned tool approval behavior with AI SDK semantics.

## 0.2.0 - 2026-05-01

- Expanded `packages/ai` toward Vercel AI SDK parity, including object generation, embeddings, agents, UI message streams, middleware, richer error types, media/upload helpers, and stricter schema validation.
- Added and broadened Bedrock and Vertex provider behavior, tests, and stream/raw chunk handling.
- Moved parity tracking into `docs/parity` with separate upstream baseline, active backlog, and audit snapshot docs.
- Added Apache-2.0 licensing with Vercel AI SDK attribution.

## 0.1.0

- Initial Go port of the core text generation and streaming surface.
