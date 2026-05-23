package anthropic

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
	"time"

	"github.com/holbrookab/go-ai/internal/httputil"
	"github.com/holbrookab/go-ai/packages/ai"
)

const defaultBaseURL = "https://api.anthropic.com/v1"

type Settings struct {
	APIKey     string
	AuthToken  string
	BaseURL    string
	Headers    map[string]string
	Client     httputil.Doer
	GenerateID func() string
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

type LanguageModel struct {
	modelID  string
	provider *Provider
}

func (m *LanguageModel) Provider() string { return "anthropic" }
func (m *LanguageModel) ModelID() string  { return m.modelID }
func (m *LanguageModel) SupportedURLs(context.Context) (map[string][]string, error) {
	return map[string][]string{"*": {"^https?://.*$"}}, nil
}

func (m *LanguageModel) DoGenerate(ctx context.Context, opts ai.LanguageModelCallOptions) (*ai.LanguageModelGenerateResult, error) {
	body, warnings := m.buildBody(opts, false)
	data, headers, err := m.post(ctx, body, opts.Headers)
	if err != nil {
		return nil, err
	}
	var response messageResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}
	content := m.parseContent(response.Content)
	return &ai.LanguageModelGenerateResult{
		Content:          content,
		FinishReason:     ai.FinishReason{Unified: mapFinish(response.StopReason, hasToolCall(content)), Raw: response.StopReason},
		Usage:            convertUsage(response.Usage),
		Warnings:         warnings,
		ProviderMetadata: anthropicMetadata(response),
		Request:          ai.RequestMetadata{Body: body},
		Response:         ai.ResponseMetadata{ID: response.ID, ModelID: response.Model, Headers: headers, Body: cloneRaw(response)},
	}, nil
}

func (m *LanguageModel) DoStream(ctx context.Context, opts ai.LanguageModelCallOptions) (*ai.LanguageModelStreamResult, error) {
	body, warnings := m.buildBody(opts, true)
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.url("/messages"), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range m.headers(opts.Headers) {
		req.Header.Set(k, v)
	}
	resp, err := httputil.Client(m.provider.settings.Client).Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("anthropic stream failed: status %d: %s", resp.StatusCode, string(data))
	}
	out := make(chan ai.StreamPart)
	state := newStreamState(m.generateID)
	go func() {
		defer resp.Body.Close()
		defer close(out)
		out <- ai.StreamPart{Type: "stream-start", Warnings: warnings}
		state.scan(resp.Body, out)
	}()
	return &ai.LanguageModelStreamResult{
		Stream:   out,
		Request:  ai.RequestMetadata{Body: body},
		Response: ai.ResponseMetadata{Headers: responseHeaders(resp.Header), ModelID: m.modelID},
	}, nil
}

func (m *LanguageModel) buildBody(opts ai.LanguageModelCallOptions, streaming bool) (map[string]any, []ai.Warning) {
	system, messages := convertMessages(opts.Prompt)
	body := map[string]any{
		"model":    m.modelID,
		"messages": messages,
		"stream":   streaming,
	}
	if len(system) > 0 {
		body["system"] = system
	}
	if opts.MaxOutputTokens != nil {
		body["max_tokens"] = *opts.MaxOutputTokens
	} else {
		body["max_tokens"] = 1024
	}
	if opts.Temperature != nil {
		body["temperature"] = *opts.Temperature
	}
	if opts.TopP != nil {
		body["top_p"] = *opts.TopP
	}
	if len(opts.StopSequences) > 0 {
		body["stop_sequences"] = opts.StopSequences
	}
	if len(opts.Tools) > 0 {
		body["tools"] = anthropicTools(opts.Tools)
		if choice := anthropicToolChoice(opts.ToolChoice); choice != nil {
			body["tool_choice"] = choice
		}
	}
	for k, v := range anthropicOptions(opts.ProviderOptions) {
		body[k] = v
	}
	return body, nil
}

func convertMessages(messages []ai.Message) ([]map[string]any, []map[string]any) {
	var system []map[string]any
	var out []map[string]any
	for _, message := range messages {
		switch message.Role {
		case ai.RoleSystem:
			if message.Text != "" {
				system = append(system, map[string]any{"type": "text", "text": message.Text})
			}
		case ai.RoleUser, ai.RoleTool:
			content := userContent(messageContent(message))
			if len(content) > 0 {
				out = append(out, map[string]any{"role": "user", "content": content})
			}
		case ai.RoleAssistant:
			content := assistantContent(messageContent(message))
			if len(content) > 0 {
				out = append(out, map[string]any{"role": "assistant", "content": content})
			}
		}
	}
	return system, out
}

func messageContent(message ai.Message) []ai.Part {
	if len(message.Content) > 0 || message.Text == "" {
		return message.Content
	}
	return []ai.Part{ai.TextPart{Text: message.Text}}
}

func userContent(parts []ai.Part) []map[string]any {
	var out []map[string]any
	for _, part := range parts {
		switch p := part.(type) {
		case ai.TextPart:
			out = append(out, map[string]any{"type": "text", "text": p.Text})
		case ai.FilePart:
			out = append(out, fileBlock(p))
		case ai.ToolResultPart:
			block := map[string]any{"type": "tool_result", "tool_use_id": p.ToolCallID, "is_error": p.IsError}
			if len(p.Output.Files) > 0 {
				var content []map[string]any
				for _, file := range p.Output.Files {
					content = append(content, fileResultBlock(file))
				}
				block["content"] = content
			} else {
				block["content"] = toolOutputText(p.Output)
			}
			out = append(out, block)
		}
	}
	return out
}

func assistantContent(parts []ai.Part) []map[string]any {
	var out []map[string]any
	for _, part := range parts {
		switch p := part.(type) {
		case ai.TextPart:
			out = append(out, map[string]any{"type": "text", "text": p.Text})
		case ai.ReasoningPart:
			block := map[string]any{"type": "thinking", "thinking": p.Text}
			if meta, ok := p.ProviderMetadata["anthropic"].(map[string]any); ok && meta["signature"] != nil {
				block["signature"] = meta["signature"]
			}
			out = append(out, block)
		case ai.ToolCallPart:
			out = append(out, map[string]any{"type": "tool_use", "id": p.ToolCallID, "name": p.ToolName, "input": p.Input})
		}
	}
	return out
}

func fileBlock(p ai.FilePart) map[string]any {
	if p.Data.URL != "" {
		if strings.HasPrefix(p.MediaType, "image/") {
			return map[string]any{"type": "image", "source": map[string]any{"type": "url", "url": p.Data.URL}}
		}
		return map[string]any{"type": "document", "source": map[string]any{"type": "url", "url": p.Data.URL}, "media_type": p.MediaType}
	}
	data := p.Data.Data
	if p.Data.Text != "" {
		data = []byte(p.Data.Text)
	}
	source := map[string]any{"type": "base64", "media_type": p.MediaType, "data": base64.StdEncoding.EncodeToString(data)}
	if strings.HasPrefix(p.MediaType, "image/") {
		return map[string]any{"type": "image", "source": source}
	}
	return map[string]any{"type": "document", "source": source}
}

func fileResultBlock(file ai.ToolResultFile) map[string]any {
	if file.URL != "" {
		return map[string]any{"type": "document", "source": map[string]any{"type": "url", "url": file.URL}, "media_type": file.MediaType}
	}
	return map[string]any{"type": "document", "source": map[string]any{"type": "base64", "media_type": file.MediaType, "data": base64.StdEncoding.EncodeToString(file.Data)}}
}

func anthropicTools(tools []ai.ModelTool) []map[string]any {
	var out []map[string]any
	for _, tool := range tools {
		if tool.Type == "provider" {
			item := map[string]any{"type": tool.ID}
			if tool.Name != "" {
				item["name"] = tool.Name
			}
			if args, ok := tool.Args.(map[string]any); ok {
				for k, v := range args {
					item[k] = v
				}
			}
			out = append(out, item)
			continue
		}
		item := map[string]any{"name": tool.Name, "input_schema": tool.InputSchema}
		if tool.Description != "" {
			item["description"] = tool.Description
		}
		if tool.ToolMetadata != nil {
			item["metadata"] = tool.ToolMetadata
		}
		out = append(out, item)
	}
	return out
}

func anthropicToolChoice(choice ai.ToolChoice) map[string]any {
	switch choice.Type {
	case "none":
		return map[string]any{"type": "none"}
	case "required":
		return map[string]any{"type": "any"}
	case "tool":
		return map[string]any{"type": "tool", "name": choice.ToolName}
	default:
		return nil
	}
}

func (m *LanguageModel) parseContent(content []contentBlock) []ai.Part {
	var out []ai.Part
	for _, block := range content {
		switch block.Type {
		case "text":
			if block.Text != "" {
				out = append(out, ai.TextPart{Text: block.Text})
			}
		case "thinking":
			meta := ai.ProviderMetadata(nil)
			if block.Signature != "" {
				meta = ai.ProviderMetadata{"anthropic": map[string]any{"signature": block.Signature}}
			}
			out = append(out, ai.ReasoningPart{Text: block.Thinking, ProviderMetadata: meta})
		case "redacted_thinking":
			out = append(out, ai.ReasoningPart{ProviderMetadata: ai.ProviderMetadata{"anthropic": map[string]any{"data": block.Data}}})
		case "tool_use", "server_tool_use":
			id := block.ID
			if id == "" {
				id = m.generateID()
			}
			out = append(out, ai.ToolCallPart{ToolCallID: id, ToolName: block.Name, InputRaw: mustJSON(block.Input), ProviderExecuted: block.Type == "server_tool_use"})
		case "web_search_tool_result", "code_execution_tool_result":
			out = append(out, ai.ToolResultPart{ToolCallID: block.ToolUseID, ToolName: block.Name, ProviderExecuted: true, Output: ai.ToolResultOutput{Type: "json", Value: block.Content}})
		}
	}
	return out
}

func (m *LanguageModel) post(ctx context.Context, body any, headers map[string]string) ([]byte, map[string]string, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.url("/messages"), bytes.NewReader(payload))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range m.headers(headers) {
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
		return nil, responseHeaders(resp.Header), fmt.Errorf("anthropic request failed: status %d: %s", resp.StatusCode, string(data))
	}
	return data, responseHeaders(resp.Header), nil
}

func (m *LanguageModel) headers(headers map[string]string) map[string]string {
	out := httputil.CloneHeaders(m.provider.settings.Headers)
	for k, v := range headers {
		out[k] = v
	}
	if out["anthropic-version"] == "" && out["Anthropic-Version"] == "" {
		out["anthropic-version"] = "2023-06-01"
	}
	if out["User-Agent"] == "" {
		out["User-Agent"] = "ai-sdk/anthropic/" + ai.Version
	} else {
		out["User-Agent"] += " ai-sdk/anthropic/" + ai.Version
	}
	if out["Authorization"] == "" {
		token := strings.TrimSpace(firstString(m.provider.settings.AuthToken, os.Getenv("ANTHROPIC_AUTH_TOKEN")))
		if token != "" {
			out["Authorization"] = "Bearer " + token
		}
	}
	if out["x-api-key"] == "" && out["X-Api-Key"] == "" && out["Authorization"] == "" {
		key := strings.TrimSpace(firstString(m.provider.settings.APIKey, os.Getenv("ANTHROPIC_API_KEY")))
		if key != "" {
			out["x-api-key"] = key
		}
	}
	return out
}

func (m *LanguageModel) url(path string) string {
	base := strings.TrimRight(m.provider.settings.BaseURL, "/")
	if base == "" {
		base = defaultBaseURL
	}
	return base + path
}

func (m *LanguageModel) generateID() string {
	if m.provider.settings.GenerateID != nil {
		return m.provider.settings.GenerateID()
	}
	return fmt.Sprintf("call-%d", time.Now().UnixNano())
}

type streamState struct {
	generateID func() string
	finish     ai.FinishReason
	usage      ai.Usage
	toolIDs    map[int]string
	toolNames  map[int]string
	toolInput  map[int]*strings.Builder
}

func newStreamState(generateID func() string) *streamState {
	return &streamState{
		generateID: generateID,
		finish:     ai.FinishReason{Unified: ai.FinishUnknown},
		toolIDs:    map[int]string{},
		toolNames:  map[int]string{},
		toolInput:  map[int]*strings.Builder{},
	}
}

func (s *streamState) scan(r io.Reader, out chan<- ai.StreamPart) {
	scanner := bufio.NewScanner(r)
	var event string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event:") {
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		out <- ai.StreamPart{Type: "raw", Raw: json.RawMessage(data)}
		s.handle(event, []byte(data), out)
	}
	if err := scanner.Err(); err != nil {
		out <- ai.StreamPart{Type: "error", Err: err}
		return
	}
	out <- ai.StreamPart{Type: "finish", FinishReason: s.finish, Usage: s.usage}
}

func (s *streamState) handle(event string, data []byte, out chan<- ai.StreamPart) {
	switch event {
	case "content_block_start":
		var value struct {
			Index        int          `json:"index"`
			ContentBlock contentBlock `json:"content_block"`
		}
		_ = json.Unmarshal(data, &value)
		if value.ContentBlock.Type == "tool_use" || value.ContentBlock.Type == "server_tool_use" {
			id := value.ContentBlock.ID
			if id == "" && s.generateID != nil {
				id = s.generateID()
			}
			s.toolIDs[value.Index] = id
			s.toolNames[value.Index] = value.ContentBlock.Name
			s.toolInput[value.Index] = &strings.Builder{}
			out <- ai.StreamPart{Type: "tool-input-start", ToolCallID: id, ToolName: value.ContentBlock.Name}
		}
	case "content_block_delta":
		var value struct {
			Index int `json:"index"`
			Delta struct {
				Type        string `json:"type"`
				Text        string `json:"text"`
				Thinking    string `json:"thinking"`
				PartialJSON string `json:"partial_json"`
			} `json:"delta"`
		}
		_ = json.Unmarshal(data, &value)
		switch value.Delta.Type {
		case "text_delta":
			out <- ai.StreamPart{Type: "text-delta", TextDelta: value.Delta.Text}
		case "thinking_delta":
			out <- ai.StreamPart{Type: "reasoning-delta", ReasoningDelta: value.Delta.Thinking}
		case "input_json_delta":
			if b := s.toolInput[value.Index]; b != nil {
				b.WriteString(value.Delta.PartialJSON)
			}
			out <- ai.StreamPart{Type: "tool-input-delta", ToolCallID: s.toolIDs[value.Index], ToolName: s.toolNames[value.Index], ToolInputDelta: value.Delta.PartialJSON}
		}
	case "content_block_stop":
		var value struct {
			Index int `json:"index"`
		}
		_ = json.Unmarshal(data, &value)
		if b := s.toolInput[value.Index]; b != nil {
			input := b.String()
			out <- ai.StreamPart{Type: "tool-input-end", ToolCallID: s.toolIDs[value.Index], ToolName: s.toolNames[value.Index], ToolInput: input}
			out <- ai.StreamPart{Type: "tool-call", ToolCallID: s.toolIDs[value.Index], ToolName: s.toolNames[value.Index], ToolInput: input}
		}
	case "message_delta":
		var value struct {
			Delta struct {
				StopReason string `json:"stop_reason"`
			} `json:"delta"`
			Usage usage `json:"usage"`
		}
		_ = json.Unmarshal(data, &value)
		s.finish = ai.FinishReason{Unified: mapFinish(value.Delta.StopReason, false), Raw: value.Delta.StopReason}
		s.usage = convertUsage(value.Usage)
	case "error":
		var value struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(data, &value)
		out <- ai.StreamPart{Type: "error", Err: fmt.Errorf("anthropic stream error: %s", value.Error.Message)}
	}
}

type messageResponse struct {
	ID         string         `json:"id"`
	Model      string         `json:"model"`
	Content    []contentBlock `json:"content"`
	StopReason string         `json:"stop_reason"`
	Usage      usage          `json:"usage"`
}

type contentBlock struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	Signature string          `json:"signature"`
	Data      string          `json:"data"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   any             `json:"content"`
}

type usage struct {
	InputTokens              *int `json:"input_tokens"`
	OutputTokens             *int `json:"output_tokens"`
	CacheReadInputTokens     *int `json:"cache_read_input_tokens"`
	CacheCreationInputTokens *int `json:"cache_creation_input_tokens"`
}

func convertUsage(in usage) ai.Usage {
	total := sumPtrs(in.InputTokens, in.OutputTokens)
	return ai.Usage{
		InputTokens:       in.InputTokens,
		OutputTokens:      in.OutputTokens,
		TotalTokens:       total,
		CachedInputTokens: in.CacheReadInputTokens,
	}
}

func sumPtrs(values ...*int) *int {
	var total int
	seen := false
	for _, value := range values {
		if value != nil {
			total += *value
			seen = true
		}
	}
	if !seen {
		return nil
	}
	return &total
}

func anthropicMetadata(response messageResponse) ai.ProviderMetadata {
	return ai.ProviderMetadata{"anthropic": map[string]any{"id": response.ID, "model": response.Model, "usage": response.Usage}}
}

func anthropicOptions(options ai.ProviderOptions) map[string]any {
	out := map[string]any{}
	for _, key := range []string{"anthropic", "anthropic.messages"} {
		if raw, ok := options[key].(map[string]any); ok {
			for k, v := range raw {
				out[k] = v
			}
		}
	}
	return out
}

func mapFinish(raw string, toolCall bool) string {
	switch raw {
	case "end_turn", "stop_sequence":
		return ai.FinishStop
	case "max_tokens":
		return ai.FinishLength
	case "tool_use":
		return ai.FinishToolCalls
	case "pause_turn":
		return ai.FinishOther
	case "refusal":
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

func mustJSON(v any) string {
	if len, ok := v.(json.RawMessage); ok && json.Valid(len) {
		return string(len)
	}
	data, err := json.Marshal(v)
	if err != nil {
		return "{}"
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
