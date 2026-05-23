package ai

import (
	"context"
	"testing"
)

func TestGenerateTextAccumulatesStepsAndFinalStep(t *testing.T) {
	calls := 0
	model := &sequenceModel{generate: func(opts LanguageModelCallOptions) (*LanguageModelGenerateResult, error) {
		calls++
		if calls == 1 {
			return &LanguageModelGenerateResult{
				Content:      []Part{ToolCallPart{ToolCallID: "call-1", ToolName: "lookup", InputRaw: `{}`}},
				FinishReason: FinishReason{Unified: FinishToolCalls, Raw: "tool_use"},
				Usage:        usage(2, 3),
			}, nil
		}
		return &LanguageModelGenerateResult{
			Content:      []Part{TextPart{Text: "done"}, SourcePart{ID: "src-1", SourceType: "url", URL: "https://example.com"}},
			FinishReason: FinishReason{Unified: FinishStop, Raw: "stop"},
			Usage:        usage(5, 7),
		}, nil
	}}

	result, err := GenerateText(context.Background(), GenerateTextOptions{
		Model:    model,
		Prompt:   "run",
		StopWhen: []StopCondition{LoopFinished()},
		Tools: map[string]Tool{
			"lookup": {
				Execute: func(context.Context, ToolCall, ToolExecutionOptions) (any, error) {
					return "ok", nil
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("GenerateText failed: %v", err)
	}
	if result.FinalStep == nil || result.FinalStep.StepNumber != 1 {
		t.Fatalf("expected final step 1, got %#v", result.FinalStep)
	}
	if len(result.Content) != 4 {
		t.Fatalf("expected accumulated content from both steps, got %#v", result.Content)
	}
	if len(result.ToolCalls) != 1 || len(result.ToolResults) != 1 {
		t.Fatalf("expected accumulated tools, got calls=%#v results=%#v", result.ToolCalls, result.ToolResults)
	}
	if len(result.ResponseMessages) != 3 {
		t.Fatalf("expected accumulated response messages, got %#v", result.ResponseMessages)
	}
	if len(result.Sources) != 1 || result.Sources[0].ID != "src-1" {
		t.Fatalf("expected accumulated source, got %#v", result.Sources)
	}
	if result.Usage.TotalTokens == nil || *result.Usage.TotalTokens != 17 {
		t.Fatalf("expected summed total usage 17, got %#v", result.Usage)
	}
}

func TestPrepareStepMessagesCarryForward(t *testing.T) {
	calls := 0
	model := &sequenceModel{generate: func(opts LanguageModelCallOptions) (*LanguageModelGenerateResult, error) {
		calls++
		if calls == 1 {
			return &LanguageModelGenerateResult{
				Content:      []Part{ToolCallPart{ToolCallID: "call-1", ToolName: "lookup", InputRaw: `{}`}},
				FinishReason: FinishReason{Unified: FinishToolCalls, Raw: "tool_use"},
			}, nil
		}
		foundOverride := false
		for _, message := range opts.Prompt {
			if message.Role == RoleUser && TextFromParts(message.Content) == "prepared base" {
				foundOverride = true
			}
		}
		if !foundOverride {
			t.Fatalf("prepared messages did not carry forward: %#v", opts.Prompt)
		}
		return &LanguageModelGenerateResult{Content: []Part{TextPart{Text: "done"}}, FinishReason: FinishReason{Unified: FinishStop}}, nil
	}}

	result, err := GenerateText(context.Background(), GenerateTextOptions{
		Model:    model,
		Prompt:   "original",
		StopWhen: []StopCondition{LoopFinished()},
		Tools: map[string]Tool{
			"lookup": {Execute: func(context.Context, ToolCall, ToolExecutionOptions) (any, error) { return "ok", nil }},
		},
		PrepareStep: func(opts PrepareStepOptions) (*PrepareStepResult, error) {
			if opts.StepNumber == 0 {
				return &PrepareStepResult{Messages: []Message{UserMessage("prepared base")}}, nil
			}
			return nil, nil
		},
	})
	if err != nil {
		t.Fatalf("GenerateText failed: %v", err)
	}
	if len(result.Steps) != 2 {
		t.Fatalf("expected two steps, got %d", len(result.Steps))
	}
	if len(result.Steps[0].Response.Messages) != 2 {
		t.Fatalf("expected first step response messages only, got %#v", result.Steps[0].Response.Messages)
	}
}

func TestInstructionsReachPrepareStepAndRepair(t *testing.T) {
	repairCalled := false
	model := &sequenceModel{generate: func(LanguageModelCallOptions) (*LanguageModelGenerateResult, error) {
		return &LanguageModelGenerateResult{
			Content:      []Part{ToolCallPart{ToolCallID: "call-1", ToolName: "lookup", InputRaw: `{"bad":`}},
			FinishReason: FinishReason{Unified: FinishToolCalls},
		}, nil
	}}
	_, err := GenerateText(context.Background(), GenerateTextOptions{
		Model:        model,
		Instructions: "be precise",
		Prompt:       "run",
		Tools:        map[string]Tool{"lookup": {}},
		PrepareStep: func(opts PrepareStepOptions) (*PrepareStepResult, error) {
			if opts.Instructions != "be precise" {
				t.Fatalf("instructions not visible to prepare step: %#v", opts)
			}
			return nil, nil
		},
		RepairToolCall: func(_ context.Context, opts ToolCallRepairOptions) (*ToolCallPart, error) {
			repairCalled = true
			if opts.Instructions != "be precise" {
				t.Fatalf("instructions not visible to repair: %#v", opts)
			}
			return &ToolCallPart{ToolCallID: "call-1", ToolName: "lookup", InputRaw: `{}`}, nil
		},
	})
	if err != nil {
		t.Fatalf("GenerateText failed: %v", err)
	}
	if !repairCalled {
		t.Fatalf("repair callback was not called")
	}
}
