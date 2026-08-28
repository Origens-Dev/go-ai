package google

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Origens-Dev/go-ai/packages/ai"
)

var _ ai.Provider = (*Provider)(nil)

func TestGenerateUsesDeveloperAPIKeyEndpointAndGoogleNamespace(t *testing.T) {
	client := &captureClient{response: `{
		"candidates":[{"content":{"parts":[
			{"text":"hello","thoughtSignature":"sig"},
			{"functionCall":{"id":"provider-call-id","name":"weather","args":{"city":"Paris"}}}
		]},"finishReason":"STOP"}],
		"usageMetadata":{"promptTokenCount":4,"candidatesTokenCount":2,"serviceTier":"flex"}
	}`}
	provider := New(Settings{
		APIKey:  "studio-key",
		Headers: map[string]string{"X-Custom": "provider"},
		Client:  client,
	})
	model := provider.LanguageModel("gemini-2.5-flash")
	frequencyPenalty := 0.5
	presencePenalty := 0.25
	result, err := model.DoGenerate(context.Background(), ai.LanguageModelCallOptions{
		Prompt:           []ai.Message{ai.SystemMessage("be concise"), ai.UserMessage("hello")},
		FrequencyPenalty: &frequencyPenalty,
		PresencePenalty:  &presencePenalty,
		ProviderOptions: ai.ProviderOptions{"google": map[string]any{
			"serviceTier":    "flex",
			"thinkingConfig": map[string]any{"includeThoughts": true},
		}},
		Headers: map[string]string{"X-Request": "call"},
	})
	if err != nil {
		t.Fatalf("DoGenerate failed: %v", err)
	}
	if got := model.Provider(); got != "google.generative-ai" {
		t.Fatalf("provider = %q", got)
	}
	if got := client.request.URL.String(); got != "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent" {
		t.Fatalf("url = %q", got)
	}
	if got := client.request.Header.Get("x-goog-api-key"); got != "studio-key" {
		t.Fatalf("x-goog-api-key = %q", got)
	}
	if got := client.request.Header.Get("Authorization"); got != "" {
		t.Fatalf("unexpected Authorization header %q", got)
	}
	if got := client.request.Header.Get("User-Agent"); got != "ai-sdk/google/"+ai.Version {
		t.Fatalf("User-Agent = %q", got)
	}
	if client.request.Header.Get("X-Custom") != "provider" || client.request.Header.Get("X-Request") != "call" {
		t.Fatalf("custom headers missing: %#v", client.request.Header)
	}

	var body map[string]any
	if err := json.Unmarshal(client.body, &body); err != nil {
		t.Fatal(err)
	}
	if got := body["serviceTier"]; got != "flex" {
		t.Fatalf("serviceTier = %#v", got)
	}
	generation := body["generationConfig"].(map[string]any)
	if _, ok := generation["frequencyPenalty"]; ok {
		t.Fatalf("Gemini 2.5 request contains frequencyPenalty: %#v", generation)
	}
	if _, ok := generation["presencePenalty"]; ok {
		t.Fatalf("Gemini 2.5 request contains presencePenalty: %#v", generation)
	}
	if len(result.Warnings) != 2 || result.Warnings[0].Feature != "frequencyPenalty" || result.Warnings[1].Feature != "presencePenalty" {
		t.Fatalf("warnings = %#v", result.Warnings)
	}
	if _, ok := result.ProviderMetadata["google"]; !ok {
		t.Fatalf("google provider metadata missing: %#v", result.ProviderMetadata)
	}
	if _, ok := result.ProviderMetadata["googleVertex"]; ok {
		t.Fatalf("Vertex metadata leaked into Google result: %#v", result.ProviderMetadata)
	}
	text := result.Content[0].(ai.TextPart)
	if _, ok := text.ProviderMetadata["google"]; !ok {
		t.Fatalf("thought signature not stored under google: %#v", text.ProviderMetadata)
	}
	call := result.Content[1].(ai.ToolCallPart)
	if call.ToolCallID != "provider-call-id" {
		t.Fatalf("tool call id = %q", call.ToolCallID)
	}
}

func TestAPIKeyFallsBackToGoogleGenerativeAIEnvironment(t *testing.T) {
	t.Setenv("GOOGLE_GENERATIVE_AI_API_KEY", "environment-key")
	client := &captureClient{response: `{"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}]}`}
	_, err := New(Settings{Client: client}).LanguageModel("gemini-2.5-flash").DoGenerate(
		context.Background(),
		ai.LanguageModelCallOptions{Prompt: []ai.Message{ai.UserMessage("hello")}},
	)
	if err != nil {
		t.Fatalf("DoGenerate failed: %v", err)
	}
	if got := client.request.Header.Get("x-goog-api-key"); got != "environment-key" {
		t.Fatalf("x-goog-api-key = %q", got)
	}
}

func TestMissingAPIKeyFailsBeforeHTTP(t *testing.T) {
	t.Setenv("GOOGLE_GENERATIVE_AI_API_KEY", "")
	client := &captureClient{response: `{}`}
	_, err := New(Settings{Client: client}).LanguageModel("gemini-2.5-flash").DoGenerate(
		context.Background(),
		ai.LanguageModelCallOptions{Prompt: []ai.Message{ai.UserMessage("hello")}},
	)
	if err == nil || !strings.Contains(err.Error(), "GOOGLE_GENERATIVE_AI_API_KEY") {
		t.Fatalf("expected missing key error, got %v", err)
	}
	if client.request != nil {
		t.Fatal("HTTP request was made without an API key")
	}
}

func TestCustomBaseURLModelPathAndSupportedURLs(t *testing.T) {
	client := &captureClient{response: `{"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}]}`}
	model := New(Settings{
		APIKey:  "key",
		BaseURL: "https://proxy.example/v1beta/",
		Client:  client,
	}).LanguageModel("publishers/example/models/gemini-custom")
	_, err := model.DoGenerate(context.Background(), ai.LanguageModelCallOptions{
		Prompt: []ai.Message{ai.UserMessage("hello")},
	})
	if err != nil {
		t.Fatalf("DoGenerate failed: %v", err)
	}
	if got := client.request.URL.String(); got != "https://proxy.example/v1beta/publishers/example/models/gemini-custom:generateContent" {
		t.Fatalf("url = %q", got)
	}
	supported, err := model.SupportedURLs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !ai.IsURLSupported(supported, "application/pdf", "https://proxy.example/v1beta/files/abc") {
		t.Fatalf("custom file URL is not supported: %#v", supported)
	}
	if !ai.IsURLSupported(supported, "application/pdf", "https://example.com/report.pdf") {
		t.Fatalf("Gemini external PDF URL is not supported: %#v", supported)
	}
}

func TestToolReplayIncludesDeveloperAPIFunctionCallIDs(t *testing.T) {
	client := &captureClient{response: `{"candidates":[{"content":{"parts":[{"text":"done"}]},"finishReason":"STOP"}]}`}
	_, err := New(Settings{APIKey: "key", Client: client}).LanguageModel("gemini-3-flash-preview").DoGenerate(
		context.Background(),
		ai.LanguageModelCallOptions{
			Prompt: []ai.Message{
				{Role: ai.RoleAssistant, Content: []ai.Part{ai.ToolCallPart{
					ToolCallID: "call-123",
					ToolName:   "weather",
					Input:      map[string]any{"city": "Paris"},
				}}},
				{Role: ai.RoleTool, Content: []ai.Part{ai.ToolResultPart{
					ToolCallID: "call-123",
					ToolName:   "weather",
					Output:     ai.ToolResultOutput{Type: "text", Value: "sunny"},
				}}},
			},
			Tools: []ai.ModelTool{{
				Type:        "function",
				Name:        "weather",
				Description: "Get weather",
				InputSchema: map[string]any{"type": "object"},
			}},
			ToolChoice: ai.ToolChoiceFor("weather"),
		},
	)
	if err != nil {
		t.Fatalf("DoGenerate failed: %v", err)
	}
	var body struct {
		Contents []struct {
			Parts []map[string]any `json:"parts"`
		} `json:"contents"`
		ToolConfig map[string]any `json:"toolConfig"`
	}
	if err := json.Unmarshal(client.body, &body); err != nil {
		t.Fatal(err)
	}
	call := body.Contents[0].Parts[0]["functionCall"].(map[string]any)
	if call["id"] != "call-123" {
		t.Fatalf("functionCall id = %#v", call["id"])
	}
	response := body.Contents[1].Parts[0]["functionResponse"].(map[string]any)
	if response["id"] != "call-123" {
		t.Fatalf("functionResponse id = %#v", response["id"])
	}
	functionConfig := body.ToolConfig["functionCallingConfig"].(map[string]any)
	if functionConfig["mode"] != "ANY" {
		t.Fatalf("function calling config = %#v", functionConfig)
	}
}

func TestPromptSafetyBlockMapsToContentFilter(t *testing.T) {
	client := &captureClient{response: `{
		"promptFeedback":{"blockReason":"SAFETY"},
		"usageMetadata":{"promptTokenCount":3,"totalTokenCount":3}
	}`}
	result, err := New(Settings{APIKey: "key", Client: client}).LanguageModel("gemini-2.5-flash").DoGenerate(
		context.Background(),
		ai.LanguageModelCallOptions{Prompt: []ai.Message{ai.UserMessage("hello")}},
	)
	if err != nil {
		t.Fatalf("DoGenerate failed: %v", err)
	}
	if result.FinishReason.Unified != ai.FinishContentFilter || result.FinishReason.Raw != "SAFETY" {
		t.Fatalf("finish reason = %#v", result.FinishReason)
	}
	if _, ok := result.ProviderMetadata["google"]; !ok {
		t.Fatalf("provider metadata missing: %#v", result.ProviderMetadata)
	}
}

func TestStreamUsesDeveloperEndpointAndGoogleThoughtMetadata(t *testing.T) {
	client := &captureClient{response: "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"reason\",\"thought\":true,\"thoughtSignature\":\"sig\"}]}}]}\n\ndata: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"done\"}]},\"finishReason\":\"STOP\"}],\"usageMetadata\":{\"promptTokenCount\":1,\"candidatesTokenCount\":1}}\n\n"}
	result, err := New(Settings{APIKey: "key", Client: client}).LanguageModel("gemini-2.5-flash").DoStream(
		context.Background(),
		ai.LanguageModelCallOptions{Prompt: []ai.Message{ai.UserMessage("hello")}},
	)
	if err != nil {
		t.Fatalf("DoStream failed: %v", err)
	}
	if got := client.request.URL.String(); got != "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:streamGenerateContent?alt=sse" {
		t.Fatalf("url = %q", got)
	}
	var sawReasoning, sawFinish bool
	for part := range result.Stream {
		switch part.Type {
		case "reasoning-delta":
			sawReasoning = true
			if _, ok := part.ProviderMetadata["google"]; !ok {
				t.Fatalf("reasoning metadata = %#v", part.ProviderMetadata)
			}
			if _, ok := part.ProviderMetadata["vertex"]; ok {
				t.Fatalf("Vertex metadata leaked into stream: %#v", part.ProviderMetadata)
			}
		case "finish":
			sawFinish = true
			if part.FinishReason.Unified != ai.FinishStop {
				t.Fatalf("finish reason = %#v", part.FinishReason)
			}
			if _, ok := part.ProviderMetadata["google"]; !ok {
				t.Fatalf("finish metadata = %#v", part.ProviderMetadata)
			}
		}
	}
	if !sawReasoning || !sawFinish {
		t.Fatalf("saw reasoning=%v finish=%v", sawReasoning, sawFinish)
	}
}

type captureClient struct {
	request  *http.Request
	body     []byte
	response string
}

func (c *captureClient) Do(req *http.Request) (*http.Response, error) {
	c.request = req.Clone(req.Context())
	c.body, _ = io.ReadAll(req.Body)
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(c.response)),
		Request:    req,
	}, nil
}
