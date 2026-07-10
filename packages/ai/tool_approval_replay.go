package ai

import (
	"context"
	"encoding/json"
)

type collectedToolApproval struct {
	Request  ToolApprovalRequestPart
	Response ToolApprovalResponsePart
	Call     ToolCall
}

func collectToolApprovals(messages []Message) (approved, denied []collectedToolApproval, err error) {
	if len(messages) == 0 || messages[len(messages)-1].Role != RoleTool {
		return nil, nil, nil
	}
	calls := map[string]ToolCall{}
	requests := map[string]ToolApprovalRequestPart{}
	for _, message := range messages {
		if message.Role != RoleAssistant {
			continue
		}
		for _, part := range message.Content {
			switch part := part.(type) {
			case ToolCallPart:
				input := part.Input
				if input == nil {
					input = parseJSONValue(part.InputRaw)
				}
				calls[part.ToolCallID] = ToolCall{
					ToolCallID: part.ToolCallID, ToolName: part.ToolName, Input: input,
					ProviderExecuted: part.ProviderExecuted, Dynamic: part.Dynamic, ToolMetadata: part.ToolMetadata, ProviderMetadata: part.ProviderMetadata,
				}
			case ToolApprovalRequestPart:
				requests[part.ApprovalID] = part
			}
		}
	}
	last := messages[len(messages)-1]
	results := map[string]bool{}
	for _, part := range last.Content {
		if result, ok := part.(ToolResultPart); ok {
			results[result.ToolCallID] = true
		}
	}
	for _, part := range last.Content {
		response, ok := part.(ToolApprovalResponsePart)
		if !ok {
			continue
		}
		request, ok := requests[response.ApprovalID]
		if !ok {
			return nil, nil, NewInvalidToolApprovalError(response.ApprovalID)
		}
		if results[request.ToolCallID] {
			continue
		}
		call, ok := calls[request.ToolCallID]
		if !ok {
			return nil, nil, NewToolCallNotFoundForApprovalError(request.ToolCallID, request.ApprovalID)
		}
		item := collectedToolApproval{Request: request, Response: response, Call: call}
		if response.Approved {
			approved = append(approved, item)
		} else {
			denied = append(denied, item)
		}
	}
	return approved, denied, nil
}

func replayToolApprovals(ctx context.Context, messages []Message, tools map[string]Tool, approval *ToolApprovalConfiguration, secret []byte, toolsContext map[string]any, sandbox Sandbox, timeout TimeoutConfig, mode ToolExecutionMode, onStart func(ToolExecutionStartEvent), onEnd func(ToolExecutionEndEvent)) ([]ToolResultPart, error) {
	approved, denied, err := collectToolApprovals(messages)
	if err != nil {
		return nil, err
	}
	validated := make([]ToolCall, 0, len(approved))
	for _, item := range approved {
		if item.Call.ProviderExecuted {
			continue
		}
		tool, ok := tools[item.Call.ToolName]
		if !ok {
			return nil, NewNoSuchToolError(item.Call.ToolName, availableToolNames(tools))
		}
		if len(secret) > 0 {
			if item.Request.Signature == "" {
				return nil, NewInvalidToolApprovalSignatureError(item.Request.ApprovalID, item.Call.ToolCallID, "missing signature")
			}
			valid, verifyErr := VerifyToolApprovalSignature(secret, item.Request.Signature, item.Request.ApprovalID, item.Call.ToolCallID, item.Call.ToolName, item.Call.Input)
			if verifyErr != nil {
				return nil, verifyErr
			}
			if !valid {
				return nil, NewInvalidToolApprovalSignatureError(item.Request.ApprovalID, item.Call.ToolCallID, "invalid signature")
			}
		}
		if err := ValidateToolInput(tool, item.Call.Input); err != nil {
			return nil, &SDKError{Kind: ErrInvalidToolInput, Message: "approved tool input validation failed", Cause: err}
		}
		decision, err := resolveToolApproval(ctx, tools, item.Call, approval, messages, cloneAnyMap(toolsContext))
		if err != nil {
			return nil, err
		}
		if decision.Type == ApprovalDecisionDenied {
			item.Response.Approved = false
			if decision.Reason != "" {
				item.Response.Reason = decision.Reason
			}
			denied = append(denied, item)
			continue
		}
		validated = append(validated, item.Call)
	}
	results := executeToolCalls(ctx, validated, tools, messages, toolsContext, sandbox, timeout.Tool, mode, nil, nil, onStart, onEnd)
	for _, item := range denied {
		results = append(results, ToolResultPart{
			ToolCallID: item.Call.ToolCallID, ToolName: item.Call.ToolName, Input: item.Call.Input,
			Output:           ToolResultOutput{Type: "execution-denied", Reason: item.Response.Reason},
			ProviderExecuted: item.Call.ProviderExecuted, Dynamic: item.Call.Dynamic,
		})
	}
	return results, nil
}

func parseJSONValue(raw string) any {
	if raw == "" {
		return nil
	}
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return raw
	}
	return value
}

func approvalPartsForCall(secret []byte, call ToolCall, decision ApprovalDecision) (ToolApprovalRequestPart, *ToolApprovalResponsePart, error) {
	request := ToolApprovalRequestPart{ApprovalID: nextApprovalID(), ToolCallID: call.ToolCallID}
	if len(secret) > 0 {
		signature, err := SignToolApproval(secret, request.ApprovalID, call.ToolCallID, call.ToolName, call.Input)
		if err != nil {
			return ToolApprovalRequestPart{}, nil, err
		}
		request.Signature = signature
	}
	if decision.Type == ApprovalDecisionUserApproval {
		return request, nil, nil
	}
	request.IsAutomatic = true
	approved := decision.Type == ApprovalDecisionApproved
	return request, &ToolApprovalResponsePart{
		ApprovalID: request.ApprovalID, Approved: approved, Reason: decision.Reason, ProviderExecuted: call.ProviderExecuted,
	}, nil
}
