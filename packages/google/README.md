# Google Gemini Developer API provider

This first-class provider connects to the Gemini Developer API used by Google
AI Studio. It is separate from `packages/vertex`, which targets Vertex AI.

```go
provider := google.New(google.Settings{
	APIKey: os.Getenv("GOOGLE_GENERATIVE_AI_API_KEY"),
})

model := provider.LanguageModel("gemini-2.5-flash")
```

When `APIKey` is empty, the provider reads
`GOOGLE_GENERATIVE_AI_API_KEY`. Requests default to
`https://generativelanguage.googleapis.com/v1beta` and send the credential in
the `x-goog-api-key` header.
