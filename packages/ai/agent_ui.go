package ai

import (
	"context"
	"encoding/json"
)

type AgentUIStreamOptions struct {
	Agent     Agent
	Call      AgentStreamOptions
	MessageID string
	TextID    string
}

func CreateAgentUIStream(ctx context.Context, opts AgentUIStreamOptions) <-chan UIMessageChunk {
	return CreateUIMessageStream(CreateUIMessageStreamOptions{
		Context: ctx,
		Execute: func(writer UIMessageStreamWriter) error {
			result, err := opts.Agent.Stream(ctx, opts.Call)
			if err != nil {
				return err
			}
			return writeStreamTextResultAsUIMessageChunks(ctx, writer, result, StreamTextUIMessageStreamOptions{
				MessageID: opts.MessageID,
				TextID:    opts.TextID,
				Tools:     opts.Agent.Tools(),
			})
		},
	})
}

type StreamTextUIMessageStreamOptions struct {
	MessageID        string
	TextID           string
	BufferSize       int
	SendReasoning    *bool
	SendSources      *bool
	SendStart        *bool
	SendFinish       *bool
	MessageMetadata  any
	OriginalMessages []UIMessage
	GenerateID       func() string
	OnStepFinish     func(UIMessageStreamStepFinishEvent) error
	OnFinish         func(UIMessageStreamFinishEvent) error
	OnError          func(error) string
	Tools            map[string]Tool
}

func CreateStreamTextUIMessageStream(ctx context.Context, result *StreamTextResult, options ...StreamTextUIMessageStreamOptions) <-chan UIMessageChunk {
	opts := firstStreamTextUIMessageStreamOptions(options)
	if result == nil || result.Stream == nil {
		return CreateUIMessageStream(CreateUIMessageStreamOptions{
			Context: ctx,
			Execute: func(UIMessageStreamWriter) error {
				return &SDKError{Kind: ErrNoOutputGenerated, Message: "stream text result is empty"}
			},
		})
	}
	return ToUIMessageStream(ctx, result.Stream, ToUIMessageStreamOptions{
		ToUIMessageChunkOptions: ToUIMessageChunkOptions{
			Tools:           opts.Tools,
			TextID:          opts.TextID,
			SendReasoning:   opts.SendReasoning,
			SendSources:     opts.SendSources,
			SendStart:       opts.SendStart,
			SendFinish:      opts.SendFinish,
			OnError:         opts.OnError,
			MessageMetadata: opts.MessageMetadata,
			MessageID:       opts.MessageID,
		},
		OriginalMessages: opts.OriginalMessages,
		GenerateID:       opts.GenerateID,
		OnStepFinish:     opts.OnStepFinish,
		OnFinish:         opts.OnFinish,
		BufferSize:       opts.BufferSize,
	})
}

func writeStreamTextResultAsUIMessageChunks(ctx context.Context, writer UIMessageStreamWriter, result *StreamTextResult, opts StreamTextUIMessageStreamOptions) error {
	if result == nil || result.Stream == nil {
		return &SDKError{Kind: ErrNoOutputGenerated, Message: "stream text result is empty"}
	}
	for chunk := range ToUIMessageStream(ctx, result.Stream, ToUIMessageStreamOptions{
		ToUIMessageChunkOptions: ToUIMessageChunkOptions{
			Tools:           opts.Tools,
			TextID:          opts.TextID,
			SendReasoning:   opts.SendReasoning,
			SendSources:     opts.SendSources,
			SendStart:       opts.SendStart,
			SendFinish:      opts.SendFinish,
			OnError:         opts.OnError,
			MessageMetadata: opts.MessageMetadata,
			MessageID:       opts.MessageID,
		},
		OriginalMessages: opts.OriginalMessages,
		GenerateID:       opts.GenerateID,
		OnStepFinish:     opts.OnStepFinish,
		OnFinish:         opts.OnFinish,
		BufferSize:       opts.BufferSize,
	}) {
		if !writer.Write(chunk) {
			return ctx.Err()
		}
	}
	return nil
}

func firstStreamTextUIMessageStreamOptions(options []StreamTextUIMessageStreamOptions) StreamTextUIMessageStreamOptions {
	if len(options) == 0 {
		return StreamTextUIMessageStreamOptions{}
	}
	return options[0]
}

func stringifyToolOutput(value any) string {
	if value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return s
	}
	bytes, err := json.Marshal(value)
	if err != nil {
		return "tool execution failed"
	}
	return string(bytes)
}

func parseUIStreamToolInput(input string) (any, error) {
	if input == "" {
		return nil, nil
	}
	var out any
	if err := json.Unmarshal([]byte(input), &out); err != nil {
		return input, err
	}
	return out, nil
}

func withUIChunkStepMetadata(chunk UIMessageChunk, part StreamPart) UIMessageChunk {
	if part.StepID != "" {
		chunk.StepID = part.StepID
	}
	stepNumber := part.StepNumber
	chunk.StepNumber = &stepNumber
	if part.StepType != "" {
		chunk.StepType = part.StepType
	}
	return chunk
}
