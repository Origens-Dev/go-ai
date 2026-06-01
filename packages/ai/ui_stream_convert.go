package ai

import (
	"context"
	"encoding/base64"
	"fmt"
)

type ToTextStreamOptions struct {
	BufferSize int
}

func ToTextStream(ctx context.Context, stream <-chan StreamPart, options ...ToTextStreamOptions) <-chan string {
	if ctx == nil {
		ctx = context.Background()
	}
	var opts ToTextStreamOptions
	if len(options) > 0 {
		opts = options[0]
	}
	out := make(chan string, opts.BufferSize)
	go func() {
		defer close(out)
		for {
			var part StreamPart
			var ok bool
			select {
			case <-ctx.Done():
				return
			case part, ok = <-stream:
				if !ok {
					return
				}
			}
			if part.Type != "text-delta" || part.TextDelta == "" {
				continue
			}
			select {
			case <-ctx.Done():
				return
			case out <- part.TextDelta:
			}
		}
	}()
	return out
}

type ToUIMessageChunkOptions struct {
	Tools           map[string]Tool
	TextID          string
	SendReasoning   *bool
	SendSources     *bool
	SendStart       *bool
	SendFinish      *bool
	OnError         func(error) string
	MessageMetadata any
	MessageID       string
}

type ToUIMessageStreamOptions struct {
	ToUIMessageChunkOptions
	OriginalMessages []UIMessage
	GenerateID       func() string
	OnStepFinish     func(UIMessageStreamStepFinishEvent) error
	OnFinish         func(UIMessageStreamFinishEvent) error
	BufferSize       int
}

func ToUIMessageChunk(part StreamPart, options ...ToUIMessageChunkOptions) (UIMessageChunk, bool) {
	var opts ToUIMessageChunkOptions
	if len(options) > 0 {
		opts = options[0]
	}
	textID := opts.TextID
	if textID == "" {
		textID = "text-1"
	}
	onError := opts.OnError
	if onError == nil {
		onError = defaultUIMessageStreamErrorText
	}
	switch part.Type {
	case "text-start":
		return withProviderMetadata(UIMessageChunk{Type: UIMessageChunkTypeTextStart, ID: firstNonEmpty(part.ID, textID)}, part.ProviderMetadata), true
	case "text-delta":
		return withProviderMetadata(UIMessageChunk{Type: UIMessageChunkTypeTextDelta, ID: firstNonEmpty(part.ID, textID), Delta: part.TextDelta}, part.ProviderMetadata), true
	case "text-end":
		return withProviderMetadata(UIMessageChunk{Type: UIMessageChunkTypeTextEnd, ID: firstNonEmpty(part.ID, textID)}, part.ProviderMetadata), true
	case "reasoning-start":
		if !boolDefault(opts.SendReasoning, true) {
			return UIMessageChunk{}, false
		}
		return withProviderMetadata(UIMessageChunk{Type: UIMessageChunkTypeReasoningStart, ID: reasoningID(part)}, part.ProviderMetadata), true
	case "reasoning-delta":
		if !boolDefault(opts.SendReasoning, true) {
			return UIMessageChunk{}, false
		}
		return withProviderMetadata(UIMessageChunk{Type: UIMessageChunkTypeReasoningDelta, ID: reasoningID(part), Delta: part.ReasoningDelta}, part.ProviderMetadata), true
	case "reasoning-end":
		if !boolDefault(opts.SendReasoning, true) {
			return UIMessageChunk{}, false
		}
		return withProviderMetadata(UIMessageChunk{Type: UIMessageChunkTypeReasoningEnd, ID: reasoningID(part)}, part.ProviderMetadata), true
	case "file", "reasoning-file":
		if part.Type == "reasoning-file" && !boolDefault(opts.SendReasoning, true) {
			return UIMessageChunk{}, false
		}
		chunk, ok := fileUIMessageChunk(part)
		return withProviderMetadata(chunk, part.ProviderMetadata), ok
	case "source":
		if !boolDefault(opts.SendSources, false) {
			return UIMessageChunk{}, false
		}
		chunk, ok := sourceUIMessageChunk(part)
		return withProviderMetadata(chunk, part.ProviderMetadata), ok
	case "custom":
		return withProviderMetadata(UIMessageChunk{Type: UIMessageChunkTypeCustom, Kind: firstNonEmpty(part.ID, fmt.Sprint(part.Raw))}, part.ProviderMetadata), true
	case "tool-input-start":
		return withToolChunkMetadata(UIMessageChunk{Type: UIMessageChunkTypeToolInputStart, ToolCallID: part.ToolCallID, ToolName: part.ToolName, InputTextDelta: part.ToolInputDelta}, part), true
	case "tool-input-delta":
		return UIMessageChunk{Type: UIMessageChunkTypeToolInputDelta, ToolCallID: part.ToolCallID, InputTextDelta: part.ToolInputDelta}, true
	case "tool-call":
		input, _ := parseUIStreamToolInput(part.ToolInput)
		chunkType := UIMessageChunkTypeToolInputAvailable
		errorText := ""
		if toolCall, ok := part.Content.(ToolCallPart); ok {
			input = firstAny(toolCall.Input, input)
			if toolCall.Invalid {
				chunkType = UIMessageChunkTypeToolInputError
				errorText = onError(toolCall.Error)
			}
		}
		return withToolChunkMetadata(UIMessageChunk{Type: chunkType, ToolCallID: part.ToolCallID, ToolName: part.ToolName, Input: input, ErrorText: errorText}, part), true
	case "tool-approval-request":
		return UIMessageChunk{Type: UIMessageChunkTypeToolApprovalRequest, ApprovalID: part.ID, ToolCallID: part.ToolCallID}, true
	case "tool-approval-response":
		approved := true
		return withToolChunkMetadata(UIMessageChunk{Type: UIMessageChunkTypeToolApprovalResponse, ApprovalID: part.ID, Approved: &approved, Reason: part.AbortReason}, part), true
	case "tool-result":
		return toolResultUIMessageChunk(part, onError), true
	case "tool-error":
		return withToolChunkMetadata(UIMessageChunk{Type: UIMessageChunkTypeToolOutputError, ToolCallID: part.ToolCallID, ErrorText: onError(part.Err)}, part), true
	case "tool-output-denied":
		return UIMessageChunk{Type: UIMessageChunkTypeToolOutputDenied, ToolCallID: part.ToolCallID}, true
	case "error":
		return UIMessageChunk{Type: UIMessageChunkTypeError, ErrorText: onError(part.Err), Err: part.Err}, true
	case "start-step":
		return UIMessageChunk{Type: UIMessageChunkTypeStartStep}, true
	case "finish-step":
		return withProviderMetadata(UIMessageChunk{Type: UIMessageChunkTypeFinishStep, FinishReason: part.FinishReason.Unified}, part.ProviderMetadata), true
	case "start":
		if !boolDefault(opts.SendStart, true) {
			return UIMessageChunk{}, false
		}
		return UIMessageChunk{Type: UIMessageChunkTypeStart, MessageID: opts.MessageID, MessageMetadata: opts.MessageMetadata}, true
	case "finish":
		if !boolDefault(opts.SendFinish, true) {
			return UIMessageChunk{}, false
		}
		return withProviderMetadata(UIMessageChunk{Type: UIMessageChunkTypeFinish, FinishReason: part.FinishReason.Unified, MessageMetadata: opts.MessageMetadata}, part.ProviderMetadata), true
	case "abort":
		return UIMessageChunk{Type: UIMessageChunkTypeAbort, Reason: part.AbortReason}, true
	case "tool-input-end", "raw", "":
		return UIMessageChunk{}, false
	default:
		return withProviderMetadata(UIMessageChunk{Type: UIMessageChunkTypeCustom, Kind: part.Type}, part.ProviderMetadata), true
	}
}

func ToUIMessageStream(ctx context.Context, stream <-chan StreamPart, options ...ToUIMessageStreamOptions) <-chan UIMessageChunk {
	if ctx == nil {
		ctx = context.Background()
	}
	var opts ToUIMessageStreamOptions
	if len(options) > 0 {
		opts = options[0]
	}
	messageID, hasMessageID := GetResponseUIMessageID(GetResponseUIMessageIDOptions{
		OriginalMessages:  opts.OriginalMessages,
		ResponseMessageID: opts.MessageID,
		GenerateID:        opts.GenerateID,
	})
	if !hasMessageID {
		messageID = opts.MessageID
	}
	if messageID == "" {
		messageID = "message-1"
	}
	textID := opts.TextID
	if textID == "" {
		textID = "text-1"
	}
	out := CreateUIMessageStream(CreateUIMessageStreamOptions{
		Context:           ctx,
		BufferSize:        opts.BufferSize,
		OriginalMessages:  opts.OriginalMessages,
		ResponseMessageID: messageID,
		GenerateID:        opts.GenerateID,
		OnStepFinish:      opts.OnStepFinish,
		OnFinish:          opts.OnFinish,
		OnError:           opts.OnError,
		Execute: func(writer UIMessageStreamWriter) error {
			if boolDefault(opts.SendStart, true) {
				writer.Write(StartUIMessageChunk(messageID))
			}
			textStarted := false
			reasoningStarted := map[string]bool{}
			write := func(chunk UIMessageChunk, part StreamPart) bool {
				return writer.Write(withUIChunkStepMetadata(chunk, part))
			}
			for {
				var part StreamPart
				var ok bool
				select {
				case <-ctx.Done():
					return ctx.Err()
				case part, ok = <-stream:
					if !ok {
						return nil
					}
				}
				if part.Type == "text-delta" && !textStarted {
					write(UIMessageChunk{Type: UIMessageChunkTypeTextStart, ID: textID, ProviderMetadata: part.ProviderMetadata}, part)
					textStarted = true
				}
				if part.Type == "reasoning-delta" && boolDefault(opts.SendReasoning, true) {
					id := reasoningID(part)
					if !reasoningStarted[id] {
						write(UIMessageChunk{Type: UIMessageChunkTypeReasoningStart, ID: id, ProviderMetadata: part.ProviderMetadata}, part)
						reasoningStarted[id] = true
					}
				}
				if (part.Type == "finish" || part.Type == "abort") && textStarted {
					writer.Write(TextEndUIMessageChunk(textID))
				}
				if part.Type == "finish" || part.Type == "abort" {
					for id := range reasoningStarted {
						writer.Write(UIMessageChunk{Type: UIMessageChunkTypeReasoningEnd, ID: id})
					}
				}
				chunk, ok := ToUIMessageChunk(part, opts.ToUIMessageChunkOptions)
				if ok {
					write(chunk, part)
				}
			}
		},
	})
	return out
}

func fileUIMessageChunk(part StreamPart) (UIMessageChunk, bool) {
	chunkType := UIMessageChunkTypeFile
	if part.Type == "reasoning-file" {
		chunkType = UIMessageChunkTypeReasoningFile
	}
	switch file := part.Content.(type) {
	case FilePart:
		return UIMessageChunk{Type: chunkType, URL: fileURL(file.Data, file.MediaType), MediaType: file.MediaType, Filename: file.Filename, ProviderMetadata: mergeMetadata(ProviderMetadata(file.ProviderOptions), file.ProviderMetadata)}, true
	case ReasoningFilePart:
		return UIMessageChunk{Type: chunkType, URL: fileURL(file.Data, file.MediaType), MediaType: file.MediaType, ProviderMetadata: mergeMetadata(ProviderMetadata(file.ProviderOptions), file.ProviderMetadata)}, true
	default:
		return UIMessageChunk{}, false
	}
}

func sourceUIMessageChunk(part StreamPart) (UIMessageChunk, bool) {
	source, ok := part.Content.(SourcePart)
	if !ok {
		if part.ID == "" {
			return UIMessageChunk{}, false
		}
		return UIMessageChunk{Type: UIMessageChunkTypeSourceURL, SourceID: part.ID}, true
	}
	if source.SourceType == "document" {
		return UIMessageChunk{Type: UIMessageChunkTypeSourceDocument, SourceID: source.ID, URL: source.URL, Title: source.Title}, true
	}
	return UIMessageChunk{Type: UIMessageChunkTypeSourceURL, SourceID: source.ID, URL: source.URL, Title: source.Title}, true
}

func toolResultUIMessageChunk(part StreamPart, onError func(error) string) UIMessageChunk {
	result, ok := part.Content.(ToolResultPart)
	if !ok {
		return withToolChunkMetadata(UIMessageChunk{Type: UIMessageChunkTypeToolOutputAvailable, ToolCallID: part.ToolCallID, ToolName: part.ToolName, Output: part.Content}, part)
	}
	if len(part.ProviderMetadata) == 0 {
		part.ProviderMetadata = result.ProviderMetadata
	}
	if len(part.ToolMetadata) == 0 {
		part.ToolMetadata = result.ToolMetadata
	}
	if part.ProviderExecuted == nil {
		part.ProviderExecuted = boolPtr(result.ProviderExecuted)
	}
	if part.Dynamic == nil {
		part.Dynamic = boolPtr(result.Dynamic)
	}
	if part.Preliminary == nil {
		part.Preliminary = boolPtr(result.Preliminary)
	}
	chunk := UIMessageChunk{ToolCallID: result.ToolCallID, ToolName: result.ToolName}
	switch result.Output.Type {
	case "execution-denied":
		chunk.Type = UIMessageChunkTypeToolOutputDenied
		chunk.Reason = result.Output.Reason
	case "error-text", "error-json":
		chunk.Type = UIMessageChunkTypeToolOutputError
		chunk.ErrorText = stringifyToolOutput(result.Output.Value)
	default:
		if result.IsError {
			chunk.Type = UIMessageChunkTypeToolOutputError
			chunk.ErrorText = stringifyToolOutput(firstAny(result.Output.Value, result.Result))
		} else {
			chunk.Type = UIMessageChunkTypeToolOutputAvailable
			chunk.Output = firstAny(result.Output.Value, result.Result)
		}
	}
	return withToolChunkMetadata(chunk, part)
}

func withProviderMetadata(chunk UIMessageChunk, metadata ProviderMetadata) UIMessageChunk {
	if len(metadata) > 0 {
		chunk.ProviderMetadata = metadata
	}
	return chunk
}

func withToolChunkMetadata(chunk UIMessageChunk, part StreamPart) UIMessageChunk {
	chunk = withProviderMetadata(chunk, part.ProviderMetadata)
	if len(part.ToolMetadata) > 0 {
		chunk.ToolMetadata = part.ToolMetadata
	}
	if part.ProviderExecuted != nil {
		chunk.ProviderExecuted = part.ProviderExecuted
	}
	if part.Dynamic != nil {
		chunk.Dynamic = part.Dynamic
	}
	if part.Preliminary != nil {
		chunk.Preliminary = part.Preliminary
	}
	if part.Title != "" {
		chunk.Title = part.Title
	}
	return chunk
}

func fileURL(data FileData, mediaType string) string {
	if data.URL != "" {
		return data.URL
	}
	if len(data.Data) > 0 {
		return "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(data.Data)
	}
	if data.Text != "" {
		return "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString([]byte(data.Text))
	}
	return data.Reference
}

func reasoningID(part StreamPart) string {
	return firstNonEmpty(part.ID, "reasoning-1")
}

func boolDefault(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func firstAny(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}
