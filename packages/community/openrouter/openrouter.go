package openrouter

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/Origens-Dev/go-ai/internal/httputil"
	"github.com/Origens-Dev/go-ai/packages/ai"
)

const defaultBaseURL = "https://openrouter.ai/api/v1"

type Settings struct {
	APIKey        string
	BaseURL       string
	SessionID     string
	AppName       string
	AppURL        string
	AppCategories []string
	Headers       map[string]string
	Client        httputil.Doer
}

type Provider struct {
	settings Settings
}

func New(settings Settings) *Provider {
	return &Provider{settings: settings}
}

func (p *Provider) LanguageModel(modelID string) ai.LanguageModel {
	return &LanguageModel{modelID: modelID, provider: p}
}

func (p *Provider) EmbeddingModel(modelID string) ai.EmbeddingModel {
	return &EmbeddingModel{modelID: modelID, provider: p}
}

type LanguageModel struct {
	modelID  string
	provider *Provider
}

func (m *LanguageModel) Provider() string { return "openrouter.chat" }
func (m *LanguageModel) ModelID() string  { return m.modelID }
func (m *LanguageModel) SupportedURLs(context.Context) (map[string][]string, error) {
	return map[string][]string{"*": {"^https?://.*$"}}, nil
}

func (m *LanguageModel) DoGenerate(ctx context.Context, opts ai.LanguageModelCallOptions) (*ai.LanguageModelGenerateResult, error) {
	body := m.buildBody(opts, false)
	data, headers, err := m.post(ctx, "/chat/completions", body, opts.Headers)
	if err != nil {
		return nil, err
	}
	var response chatResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}
	content := parseMessageContent(response.firstMessage())
	finishRaw := response.firstFinishReason()
	return &ai.LanguageModelGenerateResult{
		Content:          content,
		FinishReason:     ai.FinishReason{Unified: mapFinish(finishRaw, hasToolCall(content)), Raw: finishRaw},
		Usage:            convertUsage(response.Usage),
		ProviderMetadata: openRouterMetadata(response),
		Request:          ai.RequestMetadata{Body: body},
		Response:         ai.ResponseMetadata{ID: response.ID, ModelID: response.Model, Headers: headers, Body: cloneRaw(response)},
	}, nil
}

func (m *LanguageModel) DoStream(ctx context.Context, opts ai.LanguageModelCallOptions) (*ai.LanguageModelStreamResult, error) {
	body := m.buildBody(opts, true)
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.url("/chat/completions"), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range m.provider.headers(opts.Headers) {
		req.Header.Set(k, v)
	}
	resp, err := httputil.Client(m.provider.settings.Client).Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openrouter stream failed: status %d: %s", resp.StatusCode, string(data))
	}
	out := make(chan ai.StreamPart)
	go func() {
		defer resp.Body.Close()
		defer close(out)
		scanChatStream(resp.Body, out)
	}()
	return &ai.LanguageModelStreamResult{
		Stream:   out,
		Request:  ai.RequestMetadata{Body: body},
		Response: ai.ResponseMetadata{Headers: responseHeaders(resp.Header), ModelID: m.modelID},
	}, nil
}

func (m *LanguageModel) buildBody(opts ai.LanguageModelCallOptions, stream bool) map[string]any {
	body := map[string]any{
		"model":    m.modelID,
		"messages": openAIMessages(opts.Prompt),
		"stream":   stream,
	}
	if sessionID := strings.TrimSpace(m.provider.settings.SessionID); sessionID != "" {
		body["session_id"] = sessionID
	}
	if opts.MaxOutputTokens != nil {
		body["max_tokens"] = *opts.MaxOutputTokens
	}
	if opts.Temperature != nil {
		body["temperature"] = *opts.Temperature
	}
	if opts.TopP != nil {
		body["top_p"] = *opts.TopP
	}
	if len(opts.StopSequences) > 0 {
		body["stop"] = opts.StopSequences
	}
	if responseFormat := openAIResponseFormat(opts.ResponseFormat, opts.ProviderOptions); responseFormat != nil {
		body["response_format"] = responseFormat
	}
	if len(opts.Tools) > 0 {
		body["tools"] = openAITools(opts.Tools)
		if choice := openAIToolChoice(opts.ToolChoice); choice != nil {
			body["tool_choice"] = choice
		}
	}
	for k, v := range openRouterOptions(opts.ProviderOptions) {
		body[k] = v
	}
	return body
}

func openAIResponseFormat(format *ai.ResponseFormat, options ai.ProviderOptions) any {
	if format == nil || format.Type != "json" {
		return nil
	}
	if format.Schema == nil {
		return map[string]any{"type": "json_object"}
	}
	name := format.Name
	if name == "" {
		name = "response"
	}
	jsonSchema := map[string]any{
		"name":   name,
		"schema": format.Schema,
		"strict": openRouterStructuredOutputStrict(options),
	}
	if format.Description != "" {
		jsonSchema["description"] = format.Description
	}
	return map[string]any{
		"type":        "json_schema",
		"json_schema": jsonSchema,
	}
}

func openRouterStructuredOutputStrict(options ai.ProviderOptions) bool {
	strictValue := true
	for _, key := range []string{"openrouter", "openrouter.chat"} {
		raw, ok := options[key].(map[string]any)
		if !ok {
			continue
		}
		for _, optionKey := range []string{"structuredOutputs", "structured_outputs"} {
			structured, ok := raw[optionKey].(map[string]any)
			if !ok {
				continue
			}
			if strict, ok := structured["strict"].(bool); ok {
				strictValue = strict
			}
		}
	}
	return strictValue
}

func (m *LanguageModel) post(ctx context.Context, path string, body any, headers map[string]string) ([]byte, map[string]string, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.url(path), bytes.NewReader(payload))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range m.provider.headers(headers) {
		req.Header.Set(k, v)
	}
	resp, err := httputil.Client(m.provider.settings.Client).Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, responseHeaders(resp.Header), fmt.Errorf("openrouter request failed: status %d: %s", resp.StatusCode, string(data))
	}
	return data, responseHeaders(resp.Header), nil
}

func (m *LanguageModel) url(path string) string {
	return m.provider.url(path)
}

type EmbeddingModel struct {
	modelID  string
	provider *Provider
}

func (m *EmbeddingModel) Provider() string          { return "openrouter.embedding" }
func (m *EmbeddingModel) ModelID() string           { return m.modelID }
func (m *EmbeddingModel) MaxEmbeddingsPerCall() int { return 2048 }

func (m *EmbeddingModel) DoEmbed(ctx context.Context, opts ai.EmbeddingModelCallOptions) (*ai.EmbeddingModelResult, error) {
	body := map[string]any{"model": m.modelID, "input": opts.Values}
	for k, v := range openRouterOptions(opts.ProviderOptions) {
		body[k] = v
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.provider.url("/embeddings"), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range m.provider.headers(opts.Headers) {
		req.Header.Set(k, v)
	}
	resp, err := httputil.Client(m.provider.settings.Client).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("openrouter embeddings failed: status %d: %s", resp.StatusCode, string(data))
	}
	var response embeddingResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}
	embeddings := make([][]float64, len(response.Data))
	for _, item := range response.Data {
		if item.Index >= 0 && item.Index < len(embeddings) {
			embeddings[item.Index] = item.Embedding
		}
	}
	return &ai.EmbeddingModelResult{
		Embeddings:       embeddings,
		Usage:            ai.EmbeddingUsage{Tokens: response.Usage.TotalTokens},
		ProviderMetadata: ai.ProviderMetadata{"openrouter": map[string]any{"model": response.Model, "usage": response.Usage}},
		Response:         ai.ResponseMetadata{ID: response.ID, ModelID: response.Model, Headers: responseHeaders(resp.Header), Body: cloneRaw(response)},
	}, nil
}

func (p *Provider) url(path string) string {
	base := strings.TrimRight(p.settings.BaseURL, "/")
	if base == "" {
		base = defaultBaseURL
	}
	return base + path
}

func (p *Provider) headers(headers map[string]string) map[string]string {
	out := httputil.CloneHeaders(p.settings.Headers)
	for k, v := range headers {
		out[k] = v
	}
	setHeaderDefault(out, "User-Agent", "go-ai/openrouter/"+ai.Version)
	setHeaderDefault(out, "HTTP-Referer", p.settings.AppURL)
	setHeaderDefault(out, "X-OpenRouter-Title", p.settings.AppName)
	if len(p.settings.AppCategories) > 0 && !hasHeader(out, "X-OpenRouter-Categories") {
		categories := make([]string, 0, len(p.settings.AppCategories))
		for _, category := range p.settings.AppCategories {
			if category = strings.TrimSpace(category); category != "" {
				categories = append(categories, category)
			}
		}
		if len(categories) > 0 {
			out["X-OpenRouter-Categories"] = strings.Join(categories, ",")
		}
	}
	if !hasHeader(out, "Authorization") {
		key := strings.TrimSpace(firstString(p.settings.APIKey, os.Getenv("OPENROUTER_API_KEY")))
		if key != "" {
			out["Authorization"] = "Bearer " + key
		}
	}
	return out
}

func setHeaderDefault(headers map[string]string, key, value string) {
	value = strings.TrimSpace(value)
	if value == "" || hasHeader(headers, key) {
		return
	}
	headers[key] = value
}

func hasHeader(headers map[string]string, key string) bool {
	for candidate := range headers {
		if strings.EqualFold(candidate, key) {
			return true
		}
	}
	return false
}

func openAIMessages(messages []ai.Message) []map[string]any {
	var out []map[string]any
	for _, message := range messages {
		item := map[string]any{"role": string(message.Role)}
		switch message.Role {
		case ai.RoleSystem:
			item["content"] = message.Text
		case ai.RoleUser:
			item["content"] = openAIUserContent(messageContent(message))
		case ai.RoleAssistant:
			content, toolCalls := openAIAssistantContent(messageContent(message))
			item["content"] = content
			if len(toolCalls) > 0 {
				item["tool_calls"] = toolCalls
			}
		case ai.RoleTool:
			for _, part := range messageContent(message) {
				if result, ok := part.(ai.ToolResultPart); ok {
					out = append(out, map[string]any{"role": "tool", "tool_call_id": result.ToolCallID, "name": result.ToolName, "content": toolOutputText(result.Output)})
				}
			}
			continue
		}
		out = append(out, item)
	}
	return out
}

func messageContent(message ai.Message) []ai.Part {
	if len(message.Content) > 0 || message.Text == "" {
		return message.Content
	}
	return []ai.Part{ai.TextPart{Text: message.Text}}
}

func openAIUserContent(parts []ai.Part) any {
	if len(parts) == 1 {
		if text, ok := parts[0].(ai.TextPart); ok {
			return text.Text
		}
	}
	var out []map[string]any
	for _, part := range parts {
		switch p := part.(type) {
		case ai.TextPart:
			out = append(out, map[string]any{"type": "text", "text": p.Text})
		case ai.FilePart:
			if strings.HasPrefix(p.MediaType, "image/") {
				out = append(out, map[string]any{"type": "image_url", "image_url": map[string]any{"url": fileDataURL(p)}})
			} else {
				out = append(out, map[string]any{"type": "file", "file": map[string]any{"filename": p.Filename, "file_data": fileDataURL(p)}})
			}
		}
	}
	return out
}

func fileDataURL(p ai.FilePart) string {
	if p.Data.URL != "" {
		return p.Data.URL
	}
	data := p.Data.Data
	if p.Data.Text != "" {
		data = []byte(p.Data.Text)
	}
	return "data:" + p.MediaType + ";base64," + base64.StdEncoding.EncodeToString(data)
}

func openAIAssistantContent(parts []ai.Part) (string, []map[string]any) {
	var text strings.Builder
	var toolCalls []map[string]any
	for _, part := range parts {
		switch p := part.(type) {
		case ai.TextPart:
			text.WriteString(p.Text)
		case ai.ReasoningPart:
			text.WriteString(p.Text)
		case ai.ToolCallPart:
			toolCalls = append(toolCalls, map[string]any{"id": p.ToolCallID, "type": "function", "function": map[string]any{"name": p.ToolName, "arguments": p.InputJSON()}})
		}
	}
	return text.String(), toolCalls
}

func openAITools(tools []ai.ModelTool) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        tool.Name,
				"description": tool.Description,
				"parameters":  tool.InputSchema,
			},
		})
	}
	return out
}

func openAIToolChoice(choice ai.ToolChoice) any {
	switch choice.Type {
	case "none":
		return "none"
	case "required":
		return "required"
	case "tool":
		return map[string]any{"type": "function", "function": map[string]any{"name": choice.ToolName}}
	default:
		return nil
	}
}

func parseMessageContent(message chatMessage) []ai.Part {
	var out []ai.Part
	if message.Reasoning != "" {
		out = append(out, ai.ReasoningPart{Text: message.Reasoning, ProviderMetadata: ai.ProviderMetadata{"openrouter": map[string]any{"reasoning_details": message.ReasoningDetails}}})
	}
	if message.Content != "" {
		out = append(out, ai.TextPart{Text: message.Content})
	}
	for _, call := range message.ToolCalls {
		out = append(out, ai.ToolCallPart{ToolCallID: call.ID, ToolName: call.Function.Name, InputRaw: call.Function.Arguments})
	}
	return out
}

func scanChatStream(r io.Reader, out chan<- ai.StreamPart) {
	scanner := bufio.NewScanner(r)
	toolNames := map[int]string{}
	toolIDs := map[int]string{}
	toolInputs := map[int]*strings.Builder{}
	var usage ai.Usage
	var finish ai.FinishReason
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		out <- ai.StreamPart{Type: "raw", Raw: json.RawMessage(data)}
		var chunk chatChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			out <- ai.StreamPart{Type: "error", Err: err}
			continue
		}
		usage = convertUsage(chunk.Usage)
		for _, choice := range chunk.Choices {
			if choice.Delta.Reasoning != "" {
				out <- ai.StreamPart{Type: "reasoning-delta", ReasoningDelta: choice.Delta.Reasoning}
			}
			if choice.Delta.Content != "" {
				out <- ai.StreamPart{Type: "text-delta", TextDelta: choice.Delta.Content}
			}
			for _, call := range choice.Delta.ToolCalls {
				id := call.ID
				if id == "" {
					id = toolIDs[call.Index]
				}
				name := call.Function.Name
				if name == "" {
					name = toolNames[call.Index]
				}
				if _, ok := toolInputs[call.Index]; !ok {
					toolInputs[call.Index] = &strings.Builder{}
					toolIDs[call.Index] = id
					toolNames[call.Index] = name
					out <- ai.StreamPart{Type: "tool-input-start", ToolCallID: id, ToolName: name}
				}
				toolInputs[call.Index].WriteString(call.Function.Arguments)
				out <- ai.StreamPart{Type: "tool-input-delta", ToolCallID: id, ToolName: name, ToolInputDelta: call.Function.Arguments}
			}
			if choice.FinishReason != "" {
				finish = ai.FinishReason{Unified: mapFinish(choice.FinishReason, false), Raw: choice.FinishReason}
			}
		}
	}
	for index, input := range toolInputs {
		id := toolIDs[index]
		name := toolNames[index]
		out <- ai.StreamPart{Type: "tool-input-end", ToolCallID: id, ToolName: name, ToolInput: input.String()}
		out <- ai.StreamPart{Type: "tool-call", ToolCallID: id, ToolName: name, ToolInput: input.String()}
	}
	if err := scanner.Err(); err != nil {
		out <- ai.StreamPart{Type: "error", Err: err}
		return
	}
	out <- ai.StreamPart{Type: "finish", FinishReason: finish, Usage: usage}
}

type chatResponse struct {
	ID      string       `json:"id"`
	Model   string       `json:"model"`
	Choices []chatChoice `json:"choices"`
	Usage   usage        `json:"usage"`
}

func (r chatResponse) firstMessage() chatMessage {
	if len(r.Choices) == 0 {
		return chatMessage{}
	}
	return r.Choices[0].Message
}

func (r chatResponse) firstFinishReason() string {
	if len(r.Choices) == 0 {
		return ""
	}
	return r.Choices[0].FinishReason
}

type chatChunk struct {
	ID      string        `json:"id"`
	Model   string        `json:"model"`
	Choices []chunkChoice `json:"choices"`
	Usage   usage         `json:"usage"`
}

type chatChoice struct {
	Message      chatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

type chunkChoice struct {
	Delta        chatDelta `json:"delta"`
	FinishReason string    `json:"finish_reason"`
}

type chatMessage struct {
	Content          string         `json:"content"`
	Reasoning        string         `json:"reasoning"`
	ReasoningDetails any            `json:"reasoning_details"`
	ToolCalls        []toolCallJSON `json:"tool_calls"`
}

type chatDelta struct {
	Content   string         `json:"content"`
	Reasoning string         `json:"reasoning"`
	ToolCalls []toolCallJSON `json:"tool_calls"`
}

type toolCallJSON struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type usage struct {
	PromptTokens     *int    `json:"prompt_tokens"`
	CompletionTokens *int    `json:"completion_tokens"`
	TotalTokens      *int    `json:"total_tokens"`
	Cost             float64 `json:"cost"`
}

type embeddingResponse struct {
	ID    string `json:"id"`
	Model string `json:"model"`
	Data  []struct {
		Index     int       `json:"index"`
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
	Usage usage `json:"usage"`
}

func convertUsage(in usage) ai.Usage {
	return ai.Usage{InputTokens: in.PromptTokens, OutputTokens: in.CompletionTokens, TotalTokens: in.TotalTokens}
}

func openRouterMetadata(response chatResponse) ai.ProviderMetadata {
	return ai.ProviderMetadata{"openrouter": map[string]any{"model": response.Model, "usage": response.Usage}}
}

func openRouterOptions(options ai.ProviderOptions) map[string]any {
	out := map[string]any{}
	for _, key := range []string{"openrouter", "openrouter.chat"} {
		if raw, ok := options[key].(map[string]any); ok {
			if sessionID, ok := raw["sessionId"]; ok {
				if _, hasSnakeCase := raw["session_id"]; !hasSnakeCase {
					out["session_id"] = sessionID
				}
			}
			for k, v := range raw {
				if k == "structuredOutputs" || k == "structured_outputs" {
					continue
				}
				if k == "sessionId" {
					continue
				}
				out[k] = v
			}
		}
	}
	return out
}

func mapFinish(raw string, toolCall bool) string {
	switch raw {
	case "stop":
		return ai.FinishStop
	case "length":
		return ai.FinishLength
	case "tool_calls":
		return ai.FinishToolCalls
	case "content_filter":
		return ai.FinishContentFilter
	default:
		if toolCall {
			return ai.FinishToolCalls
		}
		if raw == "" {
			return ai.FinishUnknown
		}
		return ai.FinishOther
	}
}

func hasToolCall(parts []ai.Part) bool {
	for _, part := range parts {
		if _, ok := part.(ai.ToolCallPart); ok {
			return true
		}
	}
	return false
}

func toolOutputText(output ai.ToolResultOutput) string {
	if s, ok := output.Value.(string); ok {
		return s
	}
	data, err := json.Marshal(output.Value)
	if err != nil {
		return fmt.Sprint(output.Value)
	}
	return string(data)
}

func cloneRaw(v any) any {
	data, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var out any
	if err := json.Unmarshal(data, &out); err != nil {
		return v
	}
	return out
}

func responseHeaders(headers http.Header) map[string]string {
	out := map[string]string{}
	for k, values := range headers {
		if len(values) > 0 {
			out[k] = values[0]
		}
	}
	return out
}

func firstString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
