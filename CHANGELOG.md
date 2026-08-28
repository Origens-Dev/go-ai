# Changelog

## 0.1.0-alpha.2 - 2026-08-28

- Added a first-class Google Gemini Developer API connector for API keys from
  Google AI Studio, with generate/stream support, Google-specific auth,
  endpoints, provider metadata, tool-call IDs, service tiers, safety blocks,
  and file URL handling while preserving the separate Vertex connector.
- Moved CI and release validation to Origens' Blacksmith runner pool.

## 0.1.0-alpha.1 - 2026-08-09

- Established the Origens-maintained module at `github.com/Origens-Dev/go-ai`,
  based on `github.com/holbrookab/go-ai` `v0.3.0`
  (`f332bd84f1dd40bdc7882517e82b2ad7dafeff83`).
- Refreshed the bounded core parity baseline from Vercel AI SDK `ai@7.0.20` to
  `ai@7.0.58` for agent, tool/approval, stream, and server-side UI behaviors.
- Hardened approval HMAC payloads against field-boundary collisions while
  retaining safe verification of pending legacy signatures.
- Added semantic first-content and inter-content stream timeouts, preserved
  metadata-only text deltas, and tightened persisted UI-tool replay behavior.
- Added Go 1.24 pull-request CI and a manual, validated `v0.1.x` release path.

The entries below are the changelog inherited from the
`github.com/holbrookab/go-ai` baseline.

## 0.3.0 - 2026-07-10

- Hardened UI message streams so server errors are redacted by default while explicit error handlers can opt into client-safe detail.
- Hardened downloads against trailing-dot, legacy numeric IPv4, embedded IPv4/NAT64, CGNAT, reserved, site-local, multicast, and unsafe-redirect SSRF bypasses.
- Added signed tool approval requests, approval-response replay collection, HMAC verification, input and policy revalidation, denial handling, and UI signature preservation.
- Added deterministic provider tool ordering with partial `ToolOrder` overrides and alphabetical fallback ordering.
- Added canonical tool fingerprints and drift detection for server-controlled tool descriptions, titles, and input schemas.
- Added stable step, language-model-call, and end lifecycle callbacks while preserving finish-named compatibility aliases.
- Rejects empty provider streams and applies JSON extraction middleware to streamed fenced JSON without trimming valid suffix whitespace.
- Refreshed the core parity baseline to Vercel AI SDK `ai@7.0.20`.

## 0.2.9 - 2026-06-03

- Added OpenRouter app attribution settings and chat session ID support to the
  community connector, including upstream-style `sessionId` provider option
  normalization.

## 0.2.8 - 2026-06-01

- Added OpenRouter structured-output strictness override support while keeping
  strict JSON schema mode enabled by default.
- Added focused OpenRouter request-body coverage to verify nested schemas are
  preserved for `GenerateObject` and provider routing options pass through.

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
