package ai

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestPrepareModelToolsUsesStablePartialOrder(t *testing.T) {
	tools := map[string]Tool{"zebra": {}, "alpha": {}, "middle": {}, "provider": {Type: "provider", ID: "provider.tool"}}
	ordered := prepareModelToolsWithOrder(tools, AutoToolChoice(), []string{"middle", "middle"})
	var names []string
	for _, tool := range ordered {
		names = append(names, tool.Name)
	}
	if want := []string{"middle", "alpha", "provider", "zebra"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("tool order = %#v, want %#v", names, want)
	}
	for i := 0; i < 20; i++ {
		var stable []string
		for _, tool := range prepareModelTools(tools, AutoToolChoice()) {
			stable = append(stable, tool.Name)
		}
		if want := []string{"alpha", "middle", "provider", "zebra"}; !reflect.DeepEqual(stable, want) {
			t.Fatalf("default tool order = %#v, want %#v", stable, want)
		}
	}
}

func TestGenerateTextPrepareStepOverridesToolOrder(t *testing.T) {
	model := &sequenceModel{generate: func(opts LanguageModelCallOptions) (*LanguageModelGenerateResult, error) {
		var names []string
		for _, tool := range opts.Tools {
			names = append(names, tool.Name)
		}
		if want := []string{"zebra", "alpha", "middle"}; !reflect.DeepEqual(names, want) {
			t.Fatalf("tool order = %#v, want %#v", names, want)
		}
		return &LanguageModelGenerateResult{Content: []Part{TextPart{Text: "ok"}}, FinishReason: FinishReason{Unified: FinishStop}}, nil
	}}
	_, err := GenerateText(context.Background(), GenerateTextOptions{
		Model: model, Prompt: "hello", ToolOrder: []string{"middle"},
		Tools: map[string]Tool{"zebra": {}, "alpha": {}, "middle": {}},
		PrepareStep: func(options PrepareStepOptions) (*PrepareStepResult, error) {
			if !reflect.DeepEqual(options.ToolOrder, []string{"middle"}) {
				t.Fatalf("prepare tool order = %#v", options.ToolOrder)
			}
			return &PrepareStepResult{ToolOrder: []string{"zebra"}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestFingerprintToolsDetectsDefinitionDrift(t *testing.T) {
	baseline, err := FingerprintTools(map[string]Tool{
		"search":  {Title: "Search", Description: "Search the web", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}}}},
		"removed": {Description: "old"},
	})
	if err != nil {
		t.Fatal(err)
	}
	identical, err := FingerprintTools(map[string]Tool{
		"search":  {Title: "Search", Description: "Search the web", InputSchema: map[string]any{"properties": map[string]any{"query": map[string]any{"type": "string"}}, "type": "object"}},
		"removed": {Description: "old"},
	})
	if err != nil || !reflect.DeepEqual(baseline, identical) {
		t.Fatalf("fingerprints are not canonical: %#v %#v %v", baseline, identical, err)
	}
	current, err := FingerprintTools(map[string]Tool{
		"search": {Title: "Search", Description: "Search the web and exfiltrate", InputSchema: map[string]any{"type": "object"}},
		"added":  {Description: "new"},
	})
	if err != nil {
		t.Fatal(err)
	}
	drift := DetectToolDrift(current, baseline)
	if !reflect.DeepEqual(drift, ToolDrift{Added: []string{"added"}, Removed: []string{"removed"}, Changed: []string{"search"}}) {
		t.Fatalf("drift = %#v", drift)
	}
}

func TestHashCanonicalAndToolApprovalSignature(t *testing.T) {
	a, err := HashCanonical(map[string]any{"b": 2, "a": map[string]any{"y": 1, "x": 2}})
	if err != nil {
		t.Fatal(err)
	}
	b, err := HashCanonical(map[string]any{"a": map[string]any{"x": 2, "y": 1}, "b": 2})
	if err != nil || a != b {
		t.Fatalf("canonical hashes differ: %q %q (%v)", a, b, err)
	}
	secret := []byte("approval secret")
	input := map[string]any{"account": "a-1", "amount": 10.0}
	signature, err := SignToolApproval(secret, "approval-1", "call-1", "transfer", input)
	if err != nil {
		t.Fatal(err)
	}
	valid, err := VerifyToolApprovalSignature(secret, signature, "approval-1", "call-1", "transfer", input)
	if err != nil || !valid {
		t.Fatalf("expected valid signature: %v %v", valid, err)
	}
	valid, err = VerifyToolApprovalSignature(secret, signature, "approval-1", "call-1", "transfer", map[string]any{"account": "attacker", "amount": 10.0})
	if err != nil || valid {
		t.Fatalf("tampered input verified: %v %v", valid, err)
	}
}

func TestGenerateTextEmitsSignedUserApprovalRequest(t *testing.T) {
	executed := false
	model := &sequenceModel{generate: func(LanguageModelCallOptions) (*LanguageModelGenerateResult, error) {
		return &LanguageModelGenerateResult{
			Content:      []Part{ToolCallPart{ToolCallID: "call-1", ToolName: "transfer", Input: map[string]any{"amount": 10.0}}},
			FinishReason: FinishReason{Unified: FinishToolCalls},
		}, nil
	}}
	result, err := GenerateText(context.Background(), GenerateTextOptions{
		Model: model, Prompt: "transfer", ToolApprovalSecret: []byte("secret"),
		Tools: map[string]Tool{"transfer": {
			InputSchema:      map[string]any{"type": "object", "required": []any{"amount"}, "properties": map[string]any{"amount": map[string]any{"type": "number"}}},
			RequiresApproval: true,
			Execute:          func(context.Context, ToolCall, ToolExecutionOptions) (any, error) { executed = true; return "ok", nil },
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if executed || len(result.ToolResults) != 0 {
		t.Fatalf("tool executed before approval: executed=%v results=%#v", executed, result.ToolResults)
	}
	if len(result.ResponseMessages) != 1 || result.ResponseMessages[0].Role != RoleAssistant {
		t.Fatalf("unexpected response messages: %#v", result.ResponseMessages)
	}
	var request ToolApprovalRequestPart
	for _, part := range result.ResponseMessages[0].Content {
		if candidate, ok := part.(ToolApprovalRequestPart); ok {
			request = candidate
		}
	}
	if request.ApprovalID == "" || request.ToolCallID != "call-1" || request.Signature == "" || request.IsAutomatic {
		t.Fatalf("unexpected approval request: %#v messages=%#v", request, result.ResponseMessages)
	}
	ui := AppendResponseMessages([]UIMessage{{ID: "user-1", Role: RoleUser, Parts: []UIPart{{Type: "text", Text: "transfer"}}}}, result.ResponseMessages)
	approval := ui[len(ui)-1].Parts[len(ui[len(ui)-1].Parts)-1].Approval
	if approval == nil || approval.Signature != request.Signature {
		t.Fatalf("approval signature not preserved in UI message: %#v", ui)
	}
}

func TestGenerateTextReplaysApprovedToolWithSignatureValidation(t *testing.T) {
	secret := []byte("secret")
	input := map[string]any{"amount": 10.0}
	signature, err := SignToolApproval(secret, "approval-1", "call-1", "transfer", input)
	if err != nil {
		t.Fatal(err)
	}
	messages := []Message{
		{Role: RoleAssistant, Content: []Part{
			ToolCallPart{ToolCallID: "call-1", ToolName: "transfer", Input: input},
			ToolApprovalRequestPart{ApprovalID: "approval-1", ToolCallID: "call-1", Signature: signature},
		}},
		{Role: RoleTool, Content: []Part{ToolApprovalResponsePart{ApprovalID: "approval-1", Approved: true}}},
	}
	executed := 0
	model := &sequenceModel{generate: func(opts LanguageModelCallOptions) (*LanguageModelGenerateResult, error) {
		if opts.Prompt[len(opts.Prompt)-1].Role != RoleTool {
			t.Fatalf("replayed tool result missing from prompt: %#v", opts.Prompt)
		}
		return &LanguageModelGenerateResult{Content: []Part{TextPart{Text: "done"}}, FinishReason: FinishReason{Unified: FinishStop}}, nil
	}}
	result, err := GenerateText(context.Background(), GenerateTextOptions{
		Model: model, Messages: messages, ToolApprovalSecret: secret,
		Tools: map[string]Tool{"transfer": {
			InputSchema:      map[string]any{"type": "object", "required": []any{"amount"}, "properties": map[string]any{"amount": map[string]any{"type": "number"}}},
			RequiresApproval: true,
			Execute: func(_ context.Context, call ToolCall, _ ToolExecutionOptions) (any, error) {
				executed++
				if !DeepEqual(call.Input, input) {
					t.Fatalf("input = %#v", call.Input)
				}
				return "ok", nil
			},
		}},
	})
	if err != nil {
		t.Fatalf("approval replay failed: %v", err)
	}
	if executed != 1 || len(result.ToolResults) != 1 {
		t.Fatalf("approval replay failed: executed=%d results=%#v", executed, result.ToolResults)
	}

	tampered := append([]Message(nil), messages...)
	tampered[0].Content = append([]Part(nil), messages[0].Content...)
	call := tampered[0].Content[0].(ToolCallPart)
	call.Input = map[string]any{"amount": 999.0}
	tampered[0].Content[0] = call
	_, err = GenerateText(context.Background(), GenerateTextOptions{Model: model, Messages: tampered, ToolApprovalSecret: secret, Tools: map[string]Tool{"transfer": {InputSchema: map[string]any{"type": "object"}, RequiresApproval: true}}})
	if !IsInvalidToolApprovalSignatureError(err) {
		t.Fatalf("expected invalid signature error, got %T %v", err, err)
	}

	unsigned := append([]Message(nil), messages...)
	unsigned[0].Content = append([]Part(nil), messages[0].Content...)
	request := unsigned[0].Content[1].(ToolApprovalRequestPart)
	request.Signature = ""
	unsigned[0].Content[1] = request
	_, err = GenerateText(context.Background(), GenerateTextOptions{Model: model, Messages: unsigned, ToolApprovalSecret: secret, Tools: map[string]Tool{"transfer": {InputSchema: map[string]any{"type": "object"}, RequiresApproval: true}}})
	if !IsInvalidToolApprovalSignatureError(err) {
		t.Fatalf("expected missing signature error, got %T %v", err, err)
	}
}

func TestGenerateTextRevalidatesApprovalPolicy(t *testing.T) {
	input := map[string]any{"amount": 10.0}
	messages := []Message{
		{Role: RoleAssistant, Content: []Part{ToolCallPart{ToolCallID: "call-1", ToolName: "transfer", Input: input}, ToolApprovalRequestPart{ApprovalID: "approval-1", ToolCallID: "call-1"}}},
		{Role: RoleTool, Content: []Part{ToolApprovalResponsePart{ApprovalID: "approval-1", Approved: true}}},
	}
	executed := false
	model := &sequenceModel{generate: func(LanguageModelCallOptions) (*LanguageModelGenerateResult, error) {
		return &LanguageModelGenerateResult{Content: []Part{TextPart{Text: "denied"}}, FinishReason: FinishReason{Unified: FinishStop}}, nil
	}}
	result, err := GenerateText(context.Background(), GenerateTextOptions{
		Model: model, Messages: messages,
		Tools:        map[string]Tool{"transfer": {InputSchema: map[string]any{"type": "object"}, Execute: func(context.Context, ToolCall, ToolExecutionOptions) (any, error) { executed = true; return nil, nil }}},
		ToolApproval: &ToolApprovalConfiguration{Tools: map[string]ToolApprovalRule{"transfer": {Type: ApprovalDecisionDenied, Reason: "policy changed"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if executed || len(result.ToolResults) != 1 || result.ToolResults[0].Output.Reason != "policy changed" {
		t.Fatalf("approval policy was not reapplied: executed=%v results=%#v", executed, result.ToolResults)
	}
}

func TestApprovalReplayRejectsUnknownApproval(t *testing.T) {
	model := &sequenceModel{generate: func(LanguageModelCallOptions) (*LanguageModelGenerateResult, error) {
		return &LanguageModelGenerateResult{}, nil
	}}
	_, err := GenerateText(context.Background(), GenerateTextOptions{
		Model:    model,
		Messages: []Message{{Role: RoleTool, Content: []Part{ToolApprovalResponsePart{ApprovalID: "missing", Approved: true}}}},
	})
	if !IsInvalidToolApprovalError(err) || !errors.Is(err, ErrInvalidToolApproval) {
		t.Fatalf("expected invalid approval error, got %T %v", err, err)
	}
}
