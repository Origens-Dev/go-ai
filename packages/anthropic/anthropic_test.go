package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/holbrookab/go-ai/packages/ai"
)

func TestDoGenerateConvertsMessagesAndToolCalls(t *testing.T) {
	var request map[string]any
	client := fakeDoer{do: func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("x-api-key") != "test-key" {
			t.Fatalf("missing api key header: %#v", r.Header)
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		return jsonResponse(`{
			"id":"msg_1",
			"model":"claude-test",
			"stop_reason":"tool_use",
			"usage":{"input_tokens":2,"output_tokens":3},
			"content":[{"type":"thinking","thinking":"hmm","signature":"sig-1"},{"type":"tool_use","id":"toolu_1","name":"lookup","input":{"q":"x"}}]
		}`), nil
	}}

	provider := New(Settings{APIKey: "test-key", BaseURL: "https://anthropic.test/v1", Client: client})
	result, err := provider.LanguageModel("claude-test").DoGenerate(context.Background(), ai.LanguageModelCallOptions{
		Prompt: []ai.Message{ai.SystemMessage("rules"), ai.UserMessage("hi")},
		Tools:  []ai.ModelTool{{Type: "function", Name: "lookup", Description: "Lookup", InputSchema: map[string]any{"type": "object"}}},
	})
	if err != nil {
		t.Fatalf("DoGenerate failed: %v", err)
	}
	if request["model"] != "claude-test" {
		t.Fatalf("unexpected request: %#v", request)
	}
	if result.FinishReason.Unified != ai.FinishToolCalls {
		t.Fatalf("unexpected finish: %#v", result.FinishReason)
	}
	if len(result.Content) != 2 {
		t.Fatalf("expected reasoning and tool call, got %#v", result.Content)
	}
	call, ok := result.Content[1].(ai.ToolCallPart)
	if !ok || call.ToolName != "lookup" || call.InputRaw != `{"q":"x"}` {
		t.Fatalf("unexpected tool call: %#v", result.Content[1])
	}
}

func TestDoStreamConvertsThinkingAndToolDeltas(t *testing.T) {
	client := fakeDoer{do: func(r *http.Request) (*http.Response, error) {
		return streamResponse("" +
			"event: content_block_start\n" +
			"data: {\"index\":0,\"content_block\":{\"type\":\"thinking\"}}\n\n" +
			"event: content_block_delta\n" +
			"data: {\"index\":0,\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"hmm\"}}\n\n" +
			"event: content_block_start\n" +
			"data: {\"index\":1,\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu_1\",\"name\":\"lookup\"}}\n\n" +
			"event: content_block_delta\n" +
			"data: {\"index\":1,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"q\\\":\"}}\n\n" +
			"event: content_block_delta\n" +
			"data: {\"index\":1,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"\\\"x\\\"}\"}}\n\n" +
			"event: content_block_stop\n" +
			"data: {\"index\":1}\n\n" +
			"event: message_delta\n" +
			"data: {\"delta\":{\"stop_reason\":\"tool_use\"},\"usage\":{\"input_tokens\":2,\"output_tokens\":3}}\n\n"), nil
	}}
	provider := New(Settings{APIKey: "test-key", BaseURL: "https://anthropic.test/v1", Client: client})
	result, err := provider.LanguageModel("claude-test").DoStream(context.Background(), ai.LanguageModelCallOptions{
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
	if reasoning != "hmm" {
		t.Fatalf("unexpected reasoning: %q", reasoning)
	}
	if call.ToolCallID != "toolu_1" || call.ToolName != "lookup" || call.ToolInput != `{"q":"x"}` {
		t.Fatalf("unexpected tool call part: %#v", call)
	}
	if finish.FinishReason.Unified != ai.FinishToolCalls {
		t.Fatalf("unexpected finish: %#v", finish.FinishReason)
	}
}

func TestHeadersPreferAuthTokenAndForwardBetaHeader(t *testing.T) {
	client := fakeDoer{do: func(r *http.Request) (*http.Response, error) {
		if got := r.Header.Get("Authorization"); got != "Bearer auth-token" {
			t.Fatalf("expected bearer auth token, got %q", got)
		}
		if got := r.Header.Get("x-api-key"); got != "" {
			t.Fatalf("api key should be omitted when Authorization is present, got %q", got)
		}
		if got := r.Header.Get("anthropic-beta"); got != "tools-2024-04-04" {
			t.Fatalf("expected beta header, got %q", got)
		}
		return jsonResponse(`{"id":"msg_1","model":"claude-test","stop_reason":"end_turn","content":[{"type":"text","text":"ok"}]}`), nil
	}}
	provider := New(Settings{APIKey: "api-key", AuthToken: "auth-token", Headers: map[string]string{"anthropic-beta": "tools-2024-04-04"}, BaseURL: "https://anthropic.test/v1", Client: client})
	_, err := provider.LanguageModel("claude-test").DoGenerate(context.Background(), ai.LanguageModelCallOptions{
		Prompt: []ai.Message{ai.UserMessage("hi")},
	})
	if err != nil {
		t.Fatalf("DoGenerate failed: %v", err)
	}
}

func TestProviderOptionsPassThroughToMessagesBody(t *testing.T) {
	var request map[string]any
	client := fakeDoer{do: func(r *http.Request) (*http.Response, error) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		return jsonResponse(`{"id":"msg_1","model":"claude-test","stop_reason":"end_turn","content":[{"type":"text","text":"ok"}]}`), nil
	}}
	provider := New(Settings{APIKey: "test-key", BaseURL: "https://anthropic.test/v1", Client: client})
	_, err := provider.LanguageModel("claude-test").DoGenerate(context.Background(), ai.LanguageModelCallOptions{
		Prompt: []ai.Message{ai.UserMessage("hi")},
		ProviderOptions: ai.ProviderOptions{
			"anthropic": map[string]any{
				"metadata": map[string]any{"user_id": "user-1"},
			},
			"anthropic.messages": map[string]any{
				"container": "container-1",
				"thinking":  map[string]any{"type": "enabled", "budget_tokens": float64(1024)},
			},
		},
	})
	if err != nil {
		t.Fatalf("DoGenerate failed: %v", err)
	}
	if request["container"] != "container-1" {
		t.Fatalf("expected container provider option, got %#v", request["container"])
	}
	if _, ok := request["metadata"].(map[string]any); !ok {
		t.Fatalf("expected metadata provider option, got %#v", request["metadata"])
	}
	thinking := request["thinking"].(map[string]any)
	if thinking["type"] != "enabled" || thinking["budget_tokens"].(float64) != 1024 {
		t.Fatalf("unexpected thinking option: %#v", thinking)
	}
}

func TestToolResultFileOutputMapsToAnthropicDocumentBlock(t *testing.T) {
	var request map[string]any
	client := fakeDoer{do: func(r *http.Request) (*http.Response, error) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		return jsonResponse(`{"id":"msg_1","model":"claude-test","stop_reason":"end_turn","content":[{"type":"text","text":"ok"}]}`), nil
	}}
	provider := New(Settings{APIKey: "test-key", BaseURL: "https://anthropic.test/v1", Client: client})
	_, err := provider.LanguageModel("claude-test").DoGenerate(context.Background(), ai.LanguageModelCallOptions{
		Prompt: []ai.Message{ai.ToolMessage(ai.ToolResultPart{
			ToolCallID: "toolu_1",
			ToolName:   "make_file",
			Output: ai.ToolResultOutput{Files: []ai.ToolResultFile{{
				Data:      []byte("hello"),
				MediaType: "text/plain",
				Filename:  "hello.txt",
			}}},
		})},
	})
	if err != nil {
		t.Fatalf("DoGenerate failed: %v", err)
	}
	messages := request["messages"].([]any)
	content := messages[0].(map[string]any)["content"].([]any)
	toolResult := content[0].(map[string]any)
	if toolResult["type"] != "tool_result" || toolResult["tool_use_id"] != "toolu_1" {
		t.Fatalf("unexpected tool result block: %#v", toolResult)
	}
	fileContent := toolResult["content"].([]any)
	document := fileContent[0].(map[string]any)
	if document["type"] != "document" {
		t.Fatalf("expected document content block, got %#v", document)
	}
	source := document["source"].(map[string]any)
	if source["type"] != "base64" || source["media_type"] != "text/plain" || source["data"] == "" {
		t.Fatalf("unexpected document source: %#v", source)
	}
}

func TestDoGenerateReturnsStatusBodyOnAnthropicError(t *testing.T) {
	client := fakeDoer{do: func(r *http.Request) (*http.Response, error) {
		return statusResponse(http.StatusBadRequest, `{"error":{"type":"invalid_request_error","message":"bad request"}}`), nil
	}}
	provider := New(Settings{APIKey: "test-key", BaseURL: "https://anthropic.test/v1", Client: client})
	_, err := provider.LanguageModel("claude-test").DoGenerate(context.Background(), ai.LanguageModelCallOptions{
		Prompt: []ai.Message{ai.UserMessage("hi")},
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	if got := err.Error(); !bytes.Contains([]byte(got), []byte("status 400")) || !bytes.Contains([]byte(got), []byte("bad request")) {
		t.Fatalf("expected status and body in error, got %q", got)
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

func statusResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}
}
