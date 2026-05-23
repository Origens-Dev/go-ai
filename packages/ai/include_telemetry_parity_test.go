package ai

import (
	"context"
	"testing"
)

func TestGenerateTextOmitsBodiesByDefaultAndIncludesWhenRequested(t *testing.T) {
	model := &sequenceModel{generate: func(LanguageModelCallOptions) (*LanguageModelGenerateResult, error) {
		return &LanguageModelGenerateResult{
			Content:      []Part{TextPart{Text: "ok"}},
			FinishReason: FinishReason{Unified: FinishStop},
			Request:      RequestMetadata{Body: map[string]any{"request": true}},
			Response:     ResponseMetadata{Body: map[string]any{"response": true}},
		}, nil
	}}
	result, err := GenerateText(context.Background(), GenerateTextOptions{
		Model:  model,
		Prompt: "hi",
	})
	if err != nil {
		t.Fatalf("GenerateText failed: %v", err)
	}
	if result.Request.Body != nil || result.Response.Body != nil {
		t.Fatalf("bodies should be omitted by default: request=%#v response=%#v", result.Request.Body, result.Response.Body)
	}

	result, err = GenerateText(context.Background(), GenerateTextOptions{
		Model:   model,
		Prompt:  "hi",
		Include: IncludeConfig{RequestBody: true, ResponseBody: true},
	})
	if err != nil {
		t.Fatalf("GenerateText failed: %v", err)
	}
	if result.Request.Body == nil || result.Response.Body == nil {
		t.Fatalf("bodies should be included when requested: request=%#v response=%#v", result.Request.Body, result.Response.Body)
	}
}

func TestStreamTextIncludeRawChunksAlias(t *testing.T) {
	model := &sequenceModel{stream: func(LanguageModelCallOptions) (*LanguageModelStreamResult, error) {
		ch := make(chan StreamPart, 2)
		ch <- StreamPart{Type: "raw", Raw: "raw-event"}
		ch <- StreamPart{Type: "finish", FinishReason: FinishReason{Unified: FinishStop}}
		close(ch)
		return &LanguageModelStreamResult{Stream: ch}, nil
	}}
	result, err := StreamText(context.Background(), StreamTextOptions{
		GenerateTextOptions: GenerateTextOptions{
			Model:   model,
			Prompt:  "hi",
			Include: IncludeConfig{RawChunks: true},
		},
	})
	if err != nil {
		t.Fatalf("StreamText failed: %v", err)
	}
	seenRaw := false
	for part := range result.Stream {
		if part.Type == "raw" {
			seenRaw = true
		}
	}
	if !seenRaw {
		t.Fatalf("expected raw stream part when Include.RawChunks is true")
	}
}

func TestStreamTextOnChunkDoesNotEmitTelemetryChunkEvent(t *testing.T) {
	telemetry := &capturingTelemetry{}
	model := &sequenceModel{stream: func(LanguageModelCallOptions) (*LanguageModelStreamResult, error) {
		ch := make(chan StreamPart, 2)
		ch <- StreamPart{Type: "text-delta", TextDelta: "ok"}
		ch <- StreamPart{Type: "finish", FinishReason: FinishReason{Unified: FinishStop}}
		close(ch)
		return &LanguageModelStreamResult{Stream: ch}, nil
	}}
	chunks := 0
	result, err := StreamText(context.Background(), StreamTextOptions{
		GenerateTextOptions: GenerateTextOptions{
			Model:     model,
			Prompt:    "hi",
			Telemetry: telemetry,
		},
		OnChunk: func(ChunkEvent) {
			chunks++
		},
	})
	if err != nil {
		t.Fatalf("StreamText failed: %v", err)
	}
	for range result.Stream {
	}
	if chunks == 0 {
		t.Fatalf("expected chunk callback")
	}
	for _, event := range telemetry.events {
		if event.Name == EventOnChunk {
			t.Fatalf("did not expect telemetry chunk event: %#v", telemetry.events)
		}
	}
}

type capturingTelemetry struct {
	events []Event
}

func (t *capturingTelemetry) RecordEvent(_ context.Context, event Event) {
	t.events = append(t.events, event)
}
