package ai

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
)

func TestGenerateTextLifecycleEvents(t *testing.T) {
	telemetry := &recordingTelemetry{}
	var callbacks []string
	result, err := GenerateText(context.Background(), GenerateTextOptions{
		Model:     mockModel{},
		Prompt:    "hello",
		Telemetry: telemetry,
		OnStart: func(event StartEvent) {
			callbacks = append(callbacks, event.Operation+":start")
		},
		OnStepFinish: func(event StepFinishEvent) {
			if event.Step == nil || event.Step.Text != "ok" {
				t.Fatalf("unexpected step event: %#v", event)
			}
			callbacks = append(callbacks, event.Operation+":step")
		},
		OnFinish: func(event FinishEvent) {
			if event.Result == nil {
				t.Fatalf("finish event missing result")
			}
			callbacks = append(callbacks, event.Operation+":finish")
		},
	})
	if err != nil {
		t.Fatalf("GenerateText failed: %v", err)
	}
	if result.Text != "ok" {
		t.Fatalf("unexpected text: %q", result.Text)
	}
	if want := []string{"generate_text:start", "generate_text:step", "generate_text:finish"}; !reflect.DeepEqual(callbacks, want) {
		t.Fatalf("callbacks = %#v, want %#v", callbacks, want)
	}
	if want := []string{EventGenerateTextStart, EventOnLanguageModelCallStart, EventOnLanguageModelCallEnd, EventGenerateTextStepFinish, EventGenerateTextEnd}; !reflect.DeepEqual(telemetry.names(), want) {
		t.Fatalf("telemetry = %#v, want %#v", telemetry.names(), want)
	}
	events := telemetry.snapshot()
	if events[0].OperationID != OperationIDGenerateText || events[1].Name != EventOnLanguageModelCallStart || events[1].CallID == "" {
		t.Fatalf("unexpected diagnostic telemetry events: %#v", events)
	}
}

func TestGenerateTextStableLifecycleCallbacksAndAliasPrecedence(t *testing.T) {
	var order []string
	finishAliasCalled := false
	result, err := GenerateText(context.Background(), GenerateTextOptions{
		Model: mockModel{}, Prompt: "hello",
		OnStart: func(StartEvent) { order = append(order, "start") },
		OnStepStart: func(event StepStartEvent) {
			if event.StepNumber != 0 || len(event.Messages) == 0 {
				t.Fatalf("step start = %#v", event)
			}
			order = append(order, "step-start")
		},
		OnLanguageModelCallStart: func(LanguageModelCallStartEvent) { order = append(order, "model-start") },
		OnLanguageModelCallEnd:   func(LanguageModelCallEndEvent) { order = append(order, "model-end") },
		OnStepEnd:                func(StepFinishEvent) { order = append(order, "step-end") },
		OnStepFinish:             func(StepFinishEvent) { t.Fatal("OnStepFinish should not run when OnStepEnd is set") },
		OnEnd:                    func(FinishEvent) { order = append(order, "end") },
		OnFinish:                 func(FinishEvent) { finishAliasCalled = true },
	})
	if err != nil || result == nil {
		t.Fatalf("GenerateText failed: %v", err)
	}
	want := []string{"start", "step-start", "model-start", "model-end", "step-end", "end"}
	if !reflect.DeepEqual(order, want) || finishAliasCalled {
		t.Fatalf("callback order = %#v finishAlias=%v", order, finishAliasCalled)
	}
}

func TestEmbedOnEndPrecedesOnFinishAlias(t *testing.T) {
	model := &mockEmbeddingModel{}
	endCalled := false
	finishCalled := false
	_, err := Embed(context.Background(), EmbedOptions{
		Model: model, Value: "hello",
		OnEnd:    func(FinishEvent) { endCalled = true },
		OnFinish: func(FinishEvent) { finishCalled = true },
	})
	if err != nil {
		t.Fatal(err)
	}
	if !endCalled || finishCalled {
		t.Fatalf("end=%v finish=%v", endCalled, finishCalled)
	}
}

func TestStreamTextLifecycleChunkAndErrorEvents(t *testing.T) {
	telemetry := &recordingTelemetry{}
	var chunks []string
	model := &sequenceModel{stream: func(opts LanguageModelCallOptions) (*LanguageModelStreamResult, error) {
		ch := make(chan StreamPart, 2)
		ch <- StreamPart{Type: "text-delta", TextDelta: "hi"}
		ch <- StreamPart{Type: "finish", FinishReason: FinishReason{Unified: FinishStop}}
		close(ch)
		return &LanguageModelStreamResult{Stream: ch}, nil
	}}
	result, err := StreamText(context.Background(), StreamTextOptions{
		GenerateTextOptions: GenerateTextOptions{
			Model:     model,
			Prompt:    "hello",
			Telemetry: telemetry,
		},
		OnChunk: func(event ChunkEvent) {
			chunks = append(chunks, event.Chunk.Type)
		},
	})
	if err != nil {
		t.Fatalf("StreamText failed: %v", err)
	}
	for range result.Stream {
	}
	if want := []string{"start-step", "text-delta", "finish-step", "finish"}; !reflect.DeepEqual(chunks, want) {
		t.Fatalf("chunks = %#v, want %#v", chunks, want)
	}
	if !containsEvent(telemetry.names(), EventStreamTextEnd) {
		t.Fatalf("expected stream finish telemetry, got %#v", telemetry.names())
	}

	boom := errors.New("boom")
	var errorEvent error
	_, err = GenerateText(context.Background(), GenerateTextOptions{
		Model:  &sequenceModel{generate: func(opts LanguageModelCallOptions) (*LanguageModelGenerateResult, error) { return nil, boom }},
		Prompt: "hello",
		OnError: func(event ErrorEvent) {
			errorEvent = event.Err
		},
	})
	if !errors.Is(err, boom) {
		t.Fatalf("expected boom, got %v", err)
	}
	if !errors.Is(errorEvent, boom) {
		t.Fatalf("expected error callback to receive boom, got %v", errorEvent)
	}
}

func TestOnEndAliasAndLanguageModelCallTelemetryWrapper(t *testing.T) {
	telemetry := &wrappingTelemetry{}
	var callbacks []string
	_, err := GenerateText(context.Background(), GenerateTextOptions{
		Model:     mockModel{},
		Prompt:    "hello",
		Telemetry: telemetry,
		OnEnd: func(event FinishEvent) {
			callbacks = append(callbacks, "onEnd")
		},
		OnFinish: func(event FinishEvent) {
			callbacks = append(callbacks, "onFinish")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(callbacks, []string{"onEnd"}) {
		t.Fatalf("callbacks = %#v", callbacks)
	}
	if telemetry.wrapped != 1 || telemetry.callEvents[0].Operation != OperationGenerateText {
		t.Fatalf("language model call was not wrapped: %#v", telemetry)
	}
	if !containsEvent(telemetry.names(), EventGenerateTextEnd) {
		t.Fatalf("expected onEnd telemetry, got %#v", telemetry.names())
	}
}

func TestStreamTextOnAbortCallbackAndTelemetry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	telemetry := &recordingTelemetry{}
	var aborts []AbortEvent
	result, err := StreamText(ctx, StreamTextOptions{
		GenerateTextOptions: GenerateTextOptions{
			Model: &sequenceModel{stream: func(opts LanguageModelCallOptions) (*LanguageModelStreamResult, error) {
				return nil, errors.New("should not call model")
			}},
			Prompt:    "hello",
			Telemetry: telemetry,
			OnAbort: func(event AbortEvent) {
				aborts = append(aborts, event)
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for range result.Stream {
	}
	if len(aborts) != 1 || aborts[0].Reason == nil {
		t.Fatalf("expected abort callback with reason, got %#v", aborts)
	}
	if !containsEvent(telemetry.names(), EventStreamTextAbort) {
		t.Fatalf("expected abort telemetry, got %#v", telemetry.names())
	}
}

func TestEmbedManyAndGenerateObjectEvents(t *testing.T) {
	embedTelemetry := &recordingTelemetry{}
	_, err := EmbedMany(context.Background(), EmbedManyOptions{
		Model:     &mockEmbeddingModel{max: 2},
		Values:    []string{"a", "b"},
		Telemetry: embedTelemetry,
	})
	if err != nil {
		t.Fatalf("EmbedMany failed: %v", err)
	}
	if want := []string{EventEmbedManyStart, EventEmbedManyFinish}; !reflect.DeepEqual(embedTelemetry.names(), want) {
		t.Fatalf("embed telemetry = %#v, want %#v", embedTelemetry.names(), want)
	}

	objectTelemetry := &recordingTelemetry{}
	_, err = GenerateObject(context.Background(), GenerateObjectOptions{
		Model:     mockModel{},
		Prompt:    "json",
		Schema:    map[string]any{"type": "object"},
		Telemetry: objectTelemetry,
	})
	if err == nil {
		t.Fatalf("expected JSON parse error")
	}
	if want := []string{EventGenerateObjectStart, EventGenerateObjectError}; !reflect.DeepEqual(objectTelemetry.names(), want) {
		t.Fatalf("object telemetry = %#v, want %#v", objectTelemetry.names(), want)
	}
}

func TestTelemetryOptionsFilterAttributes(t *testing.T) {
	recordInputs := false
	recordOutputs := false
	telemetry := &recordingTelemetry{}
	_, err := GenerateText(context.Background(), GenerateTextOptions{
		Model:     mockModel{},
		Prompt:    "hello",
		Telemetry: telemetry,
		TelemetryOptions: TelemetryOptions{
			RecordInputs:  &recordInputs,
			RecordOutputs: &recordOutputs,
			FunctionID:    "fn",
			AttributeFilter: func(_ Event, key string, _ any) bool {
				return key != "usage"
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range telemetry.snapshot() {
		if event.FunctionID != "fn" || event.RecordInputs == nil || *event.RecordInputs || event.RecordOutputs == nil || *event.RecordOutputs {
			t.Fatalf("telemetry options not applied to event: %#v", event)
		}
		if _, ok := event.Attributes["input.prompt"]; ok {
			t.Fatalf("input attribute should be filtered: %#v", event.Attributes)
		}
		if _, ok := event.Attributes["output.content"]; ok {
			t.Fatalf("output attribute should be filtered: %#v", event.Attributes)
		}
		if _, ok := event.Attributes["usage"]; ok {
			t.Fatalf("custom attribute filter should remove usage: %#v", event.Attributes)
		}
	}
}

func TestTelemetryDisabledSuppressesRecordEvent(t *testing.T) {
	enabled := false
	telemetry := &recordingTelemetry{}
	_, err := GenerateText(context.Background(), GenerateTextOptions{
		Model:     mockModel{},
		Prompt:    "hello",
		Telemetry: telemetry,
		TelemetryOptions: TelemetryOptions{
			IsEnabled: &enabled,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := telemetry.names(); len(got) != 0 {
		t.Fatalf("expected no telemetry when disabled, got %#v", got)
	}
}

type recordingTelemetry struct {
	mu     sync.Mutex
	events []Event
}

type wrappingTelemetry struct {
	recordingTelemetry
	wrapped    int
	callEvents []LanguageModelCallEvent
}

func (w *wrappingTelemetry) ExecuteLanguageModelCall(ctx context.Context, event LanguageModelCallEvent, execute func(context.Context) error) error {
	w.wrapped++
	w.callEvents = append(w.callEvents, event)
	return execute(ctx)
}

func (r *recordingTelemetry) RecordEvent(_ context.Context, event Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func (r *recordingTelemetry) names() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	names := make([]string, len(r.events))
	for i, event := range r.events {
		names[i] = event.Name
	}
	return names
}

func (r *recordingTelemetry) snapshot() []Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Event(nil), r.events...)
}

func containsEvent(names []string, want string) bool {
	for _, name := range names {
		if name == want {
			return true
		}
	}
	return false
}
