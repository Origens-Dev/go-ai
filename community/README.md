# Community Connectors

Community connectors are useful integrations that implement the public `packages/ai` model interfaces but are not first-class parity targets in the same way as `packages/vertex`, `packages/bedrock`, and `packages/anthropic`.

Each connector in this directory should:

- keep provider-specific behavior isolated to its own package;
- avoid global registry side effects;
- include package docs, README coverage, examples, and fixture-based tests;
- expose idiomatic Go settings for API keys, base URLs, headers, and custom HTTP clients;
- preserve provider metadata when the remote API exposes useful routing, usage, cost, or debug details.

The initial community connector is `community/openrouter`.
