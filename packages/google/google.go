package google

import (
	"github.com/Origens-Dev/go-ai/internal/httputil"
	"github.com/Origens-Dev/go-ai/packages/ai"
	"github.com/Origens-Dev/go-ai/packages/vertex"
)

// Settings configures the Google Gemini Developer API, commonly accessed with
// an API key created in Google AI Studio.
type Settings struct {
	// APIKey is sent in the x-goog-api-key request header. When empty, the
	// provider reads GOOGLE_GENERATIVE_AI_API_KEY.
	APIKey string

	// BaseURL overrides the Gemini Developer API endpoint. It defaults to
	// https://generativelanguage.googleapis.com/v1beta.
	BaseURL string

	// Headers are included in every request. Per-call headers take precedence.
	Headers map[string]string

	// Client overrides the HTTP client, primarily for middleware and tests.
	Client httputil.Doer

	// GenerateID overrides tool-call and source ID generation.
	GenerateID func() string
}

// Provider creates Gemini language models backed by the Google Gemini
// Developer API rather than Vertex AI.
type Provider struct {
	delegate *vertex.Provider
}

// New creates a Google Gemini Developer API provider.
func New(settings Settings) *Provider {
	return &Provider{delegate: vertex.NewGenerativeAI(vertex.GenerativeAISettings{
		APIKey:     settings.APIKey,
		BaseURL:    settings.BaseURL,
		Headers:    settings.Headers,
		Client:     settings.Client,
		GenerateID: settings.GenerateID,
	})}
}

// LanguageModel returns a Gemini language model for generate and stream calls.
func (p *Provider) LanguageModel(modelID string) ai.LanguageModel {
	return p.delegate.LanguageModel(modelID)
}
