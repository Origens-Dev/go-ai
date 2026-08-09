package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"testing"

	"github.com/Origens-Dev/go-ai/packages/ai"
)

func TestDoGenerateConvertsOpenRouterChat(t *testing.T) {
	var request map[string]any
	client := fakeDoer{do: func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("missing auth header: %#v", r.Header)
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		return jsonResponse(`{
			"id":"chatcmpl_1",
			"model":"openai/gpt-test",
			"choices":[{"finish_reason":"tool_calls","message":{"reasoning":"think","tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"q\":\"x\"}"}}]}}],
			"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5,"cost":0.001}
		}`), nil
	}}

	provider := New(Settings{APIKey: "test-key", BaseURL: "https://openrouter.test/api/v1", Client: client})
	result, err := provider.LanguageModel("openai/gpt-test").DoGenerate(context.Background(), ai.LanguageModelCallOptions{
		Prompt: []ai.Message{ai.UserMessage("hi")},
		Tools:  []ai.ModelTool{{Type: "function", Name: "lookup", InputSchema: map[string]any{"type": "object"}}},
		ProviderOptions: ai.ProviderOptions{"openrouter": map[string]any{
			"reasoning": map[string]any{"effort": "low"},
		}},
	})
	if err != nil {
		t.Fatalf("DoGenerate failed: %v", err)
	}
	if request["reasoning"] == nil {
		t.Fatalf("expected openrouter provider option passthrough: %#v", request)
	}
	if result.FinishReason.Unified != ai.FinishToolCalls {
		t.Fatalf("unexpected finish: %#v", result.FinishReason)
	}
	if len(result.Content) != 2 {
		t.Fatalf("expected reasoning and tool call, got %#v", result.Content)
	}
}

func TestDoGenerateSendsOpenRouterSessionAndAppAttribution(t *testing.T) {
	var request map[string]any
	client := fakeDoer{do: func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("HTTP-Referer") != "https://example.test" {
			t.Fatalf("expected HTTP-Referer attribution header, got %#v", r.Header)
		}
		if r.Header.Get("X-OpenRouter-Title") != "Example Agent" {
			t.Fatalf("expected title attribution header, got %#v", r.Header)
		}
		if r.Header.Get("X-OpenRouter-Categories") != "agent,productivity" {
			t.Fatalf("expected category attribution header, got %#v", r.Header)
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		return jsonResponse(`{
			"id":"chatcmpl_1",
			"model":"openai/gpt-test",
			"choices":[{"finish_reason":"stop","message":{"content":"hi"}}],
			"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}
		}`), nil
	}}

	provider := New(Settings{
		APIKey:        "test-key",
		BaseURL:       "https://openrouter.test/api/v1",
		SessionID:     "session-provider",
		AppName:       "Example Agent",
		AppURL:        "https://example.test",
		AppCategories: []string{"agent", "productivity"},
		Client:        client,
	})
	_, err := provider.LanguageModel("openai/gpt-test").DoGenerate(context.Background(), ai.LanguageModelCallOptions{
		Prompt: []ai.Message{ai.UserMessage("hi")},
	})
	if err != nil {
		t.Fatalf("DoGenerate failed: %v", err)
	}
	if request["session_id"] != "session-provider" {
		t.Fatalf("expected settings session_id, got %#v", request)
	}
}

func TestDoGenerateNormalizesOpenRouterSessionIDProviderOption(t *testing.T) {
	var request map[string]any
	client := fakeDoer{do: func(r *http.Request) (*http.Response, error) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		return jsonResponse(`{
			"id":"chatcmpl_1",
			"model":"openai/gpt-test",
			"choices":[{"finish_reason":"stop","message":{"content":"hi"}}],
			"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}
		}`), nil
	}}

	provider := New(Settings{
		APIKey:    "test-key",
		BaseURL:   "https://openrouter.test/api/v1",
		SessionID: "session-provider",
		Client:    client,
	})
	_, err := provider.LanguageModel("openai/gpt-test").DoGenerate(context.Background(), ai.LanguageModelCallOptions{
		Prompt: []ai.Message{ai.UserMessage("hi")},
		ProviderOptions: ai.ProviderOptions{"openrouter": map[string]any{
			"sessionId": "session-call",
		}},
	})
	if err != nil {
		t.Fatalf("DoGenerate failed: %v", err)
	}
	if request["session_id"] != "session-call" {
		t.Fatalf("expected provider option session_id override, got %#v", request)
	}
	if request["sessionId"] != nil {
		t.Fatalf("camelCase helper option leaked into request: %#v", request)
	}
}

func TestDoGenerateSendsJSONSchemaResponseFormatWhenSchemaPresent(t *testing.T) {
	var request map[string]any
	client := fakeDoer{do: func(r *http.Request) (*http.Response, error) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		return jsonResponse(`{
			"id":"chatcmpl_1",
			"model":"openai/gpt-test",
			"choices":[{"finish_reason":"stop","message":{"content":"{\"name\":\"Ada\"}"}}],
			"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}
		}`), nil
	}}

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
		},
		"required":             []any{"name"},
		"additionalProperties": false,
	}
	provider := New(Settings{APIKey: "test-key", BaseURL: "https://openrouter.test/api/v1", Client: client})
	_, err := provider.LanguageModel("openai/gpt-test").DoGenerate(context.Background(), ai.LanguageModelCallOptions{
		Prompt: []ai.Message{ai.UserMessage("hi")},
		ResponseFormat: &ai.ResponseFormat{
			Type:        "json",
			Schema:      schema,
			Name:        "person",
			Description: "A generated person.",
		},
	})
	if err != nil {
		t.Fatalf("DoGenerate failed: %v", err)
	}
	format, ok := request["response_format"].(map[string]any)
	if !ok {
		t.Fatalf("expected response_format object, got %#v", request["response_format"])
	}
	if format["type"] != "json_schema" {
		t.Fatalf("expected json_schema response format, got %#v", format)
	}
	jsonSchema, ok := format["json_schema"].(map[string]any)
	if !ok {
		t.Fatalf("expected json_schema object, got %#v", format["json_schema"])
	}
	if jsonSchema["name"] != "person" || jsonSchema["description"] != "A generated person." || jsonSchema["strict"] != true {
		t.Fatalf("unexpected json_schema metadata: %#v", jsonSchema)
	}
	if !reflect.DeepEqual(jsonSchema["schema"], schema) {
		t.Fatalf("expected schema %#v, got %#v", schema, jsonSchema["schema"])
	}
}

func TestGenerateObjectPreservesNestedJSONSchemaResponseFormat(t *testing.T) {
	var request map[string]any
	client := fakeDoer{do: func(r *http.Request) (*http.Response, error) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		return jsonResponse(`{
			"id":"chatcmpl_1",
			"model":"google/gemini-test",
			"choices":[{"finish_reason":"stop","message":{"content":"{\"compiledDefinition\":{\"definition\":{\"startsWhen\":\"After intake is complete\"}}}"}}],
			"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}
		}`), nil
	}}

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"compiledDefinition": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"definition": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"startsWhen": map[string]any{"type": "string"},
						},
						"required":             []any{"startsWhen"},
						"additionalProperties": false,
					},
				},
				"required":             []any{"definition"},
				"additionalProperties": false,
			},
		},
		"required":             []any{"compiledDefinition"},
		"additionalProperties": false,
	}
	provider := New(Settings{APIKey: "test-key", BaseURL: "https://openrouter.test/api/v1", Client: client})
	_, err := ai.GenerateObject(context.Background(), ai.GenerateObjectOptions{
		Model:      provider.LanguageModel("google/gemini-test"),
		Output:     ai.OutputObject,
		Mode:       ai.ObjectModeJSON,
		Schema:     schema,
		SchemaName: "routine_stepgen_outline",
		Prompt:     "Generate an outline.",
		ProviderOptions: ai.ProviderOptions{"openrouter": map[string]any{
			"provider": map[string]any{"require_parameters": true},
		}},
	})
	if err != nil {
		t.Fatalf("GenerateObject failed: %v", err)
	}

	wantFormat := map[string]any{
		"type": "json_schema",
		"json_schema": map[string]any{
			"name":   "routine_stepgen_outline",
			"schema": schema,
			"strict": true,
		},
	}
	if !reflect.DeepEqual(request["response_format"], wantFormat) {
		t.Fatalf("unexpected response_format:\nwant %#v\n got %#v", wantFormat, request["response_format"])
	}
	providerOptions, ok := request["provider"].(map[string]any)
	if !ok || providerOptions["require_parameters"] != true {
		t.Fatalf("expected provider.require_parameters passthrough, got %#v", request["provider"])
	}
	sentSchema := request["response_format"].(map[string]any)["json_schema"].(map[string]any)["schema"].(map[string]any)
	compiled := sentSchema["properties"].(map[string]any)["compiledDefinition"].(map[string]any)
	definition := compiled["properties"].(map[string]any)["definition"].(map[string]any)
	if !reflect.DeepEqual(definition["required"], []any{"startsWhen"}) {
		t.Fatalf("nested required changed: %#v", definition["required"])
	}
	if _, ok := definition["properties"].(map[string]any)["startsWhen"]; !ok {
		t.Fatalf("nested startsWhen property missing: %#v", definition["properties"])
	}
}

func TestDoGenerateAllowsStructuredOutputStrictOverride(t *testing.T) {
	var request map[string]any
	client := fakeDoer{do: func(r *http.Request) (*http.Response, error) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		return jsonResponse(`{
			"id":"chatcmpl_1",
			"model":"openai/gpt-test",
			"choices":[{"finish_reason":"stop","message":{"content":"{\"name\":\"Ada\"}"}}],
			"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}
		}`), nil
	}}

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
		},
		"required":             []any{"name"},
		"additionalProperties": false,
	}
	provider := New(Settings{APIKey: "test-key", BaseURL: "https://openrouter.test/api/v1", Client: client})
	_, err := provider.LanguageModel("openai/gpt-test").DoGenerate(context.Background(), ai.LanguageModelCallOptions{
		Prompt: []ai.Message{ai.UserMessage("hi")},
		ResponseFormat: &ai.ResponseFormat{
			Type:   "json",
			Schema: schema,
			Name:   "person",
		},
		ProviderOptions: ai.ProviderOptions{
			"openrouter": map[string]any{
				"structuredOutputs": map[string]any{"strict": true},
				"reasoning":         map[string]any{"effort": "low"},
			},
			"openrouter.chat": map[string]any{
				"structuredOutputs": map[string]any{"strict": false},
			},
		},
	})
	if err != nil {
		t.Fatalf("DoGenerate failed: %v", err)
	}
	format := request["response_format"].(map[string]any)
	jsonSchema := format["json_schema"].(map[string]any)
	if jsonSchema["strict"] != false {
		t.Fatalf("expected strict override false, got %#v", jsonSchema["strict"])
	}
	if request["structuredOutputs"] != nil {
		t.Fatalf("structuredOutputs helper option leaked into request: %#v", request)
	}
	if request["reasoning"] == nil {
		t.Fatalf("expected regular openrouter option passthrough: %#v", request)
	}
}

func TestDoGenerateSendsJSONObjectResponseFormatWhenSchemaNil(t *testing.T) {
	var request map[string]any
	client := fakeDoer{do: func(r *http.Request) (*http.Response, error) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		return jsonResponse(`{
			"id":"chatcmpl_1",
			"model":"openai/gpt-test",
			"choices":[{"finish_reason":"stop","message":{"content":"{}"}}],
			"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}
		}`), nil
	}}

	provider := New(Settings{APIKey: "test-key", BaseURL: "https://openrouter.test/api/v1", Client: client})
	_, err := provider.LanguageModel("openai/gpt-test").DoGenerate(context.Background(), ai.LanguageModelCallOptions{
		Prompt:         []ai.Message{ai.UserMessage("hi")},
		ResponseFormat: &ai.ResponseFormat{Type: "json"},
	})
	if err != nil {
		t.Fatalf("DoGenerate failed: %v", err)
	}
	format, ok := request["response_format"].(map[string]any)
	if !ok {
		t.Fatalf("expected response_format object, got %#v", request["response_format"])
	}
	if !reflect.DeepEqual(format, map[string]any{"type": "json_object"}) {
		t.Fatalf("expected json_object response format, got %#v", format)
	}
}

func TestDoEmbedConvertsOpenRouterEmbeddings(t *testing.T) {
	client := fakeDoer{do: func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/v1/embeddings" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		return jsonResponse(`{
			"id":"emb_1",
			"model":"text-embedding-test",
			"data":[{"index":0,"embedding":[0.1,0.2]},{"index":1,"embedding":[0.3,0.4]}],
			"usage":{"total_tokens":7}
		}`), nil
	}}

	provider := New(Settings{APIKey: "test-key", BaseURL: "https://openrouter.test/api/v1", Client: client})
	result, err := provider.EmbeddingModel("text-embedding-test").DoEmbed(context.Background(), ai.EmbeddingModelCallOptions{Values: []string{"a", "b"}})
	if err != nil {
		t.Fatalf("DoEmbed failed: %v", err)
	}
	if len(result.Embeddings) != 2 || len(result.Embeddings[1]) != 2 || result.Embeddings[1][0] != 0.3 {
		t.Fatalf("unexpected embeddings: %#v", result.Embeddings)
	}
	if result.Usage.Tokens == nil || *result.Usage.Tokens != 7 {
		t.Fatalf("unexpected usage: %#v", result.Usage)
	}
}

func TestDoStreamConvertsOpenRouterToolDeltas(t *testing.T) {
	client := fakeDoer{do: func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		return streamResponse("" +
			"data: {\"choices\":[{\"delta\":{\"reasoning\":\"think \",\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"lookup\",\"arguments\":\"{\\\"q\\\":\"}}]}}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"\\\"x\\\"}\"}}]},\"finish_reason\":\"tool_calls\"}],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":3,\"total_tokens\":5}}\n\n" +
			"data: [DONE]\n\n"), nil
	}}
	provider := New(Settings{APIKey: "test-key", BaseURL: "https://openrouter.test/api/v1", Client: client})
	result, err := provider.LanguageModel("openai/gpt-test").DoStream(context.Background(), ai.LanguageModelCallOptions{
		Prompt: []ai.Message{ai.UserMessage("hi")},
	})
	if err != nil {
		t.Fatalf("DoStream failed: %v", err)
	}
	var reasoning string
	var call ai.StreamPart
	var finish ai.StreamPart
	for part := range result.Stream {
		switch part.Type {
		case "reasoning-delta":
			reasoning += part.ReasoningDelta
		case "tool-call":
			call = part
		case "finish":
			finish = part
		}
	}
	if reasoning != "think " {
		t.Fatalf("unexpected reasoning: %q", reasoning)
	}
	if call.ToolCallID != "call_1" || call.ToolName != "lookup" || call.ToolInput != `{"q":"x"}` {
		t.Fatalf("unexpected tool call: %#v", call)
	}
	if finish.Usage.TotalTokens == nil || *finish.Usage.TotalTokens != 5 {
		t.Fatalf("unexpected usage: %#v", finish.Usage)
	}
}

type fakeDoer struct {
	do func(*http.Request) (*http.Response, error)
}

func (f fakeDoer) Do(req *http.Request) (*http.Response, error) {
	return f.do(req)
}

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}
}

func streamResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}
}
