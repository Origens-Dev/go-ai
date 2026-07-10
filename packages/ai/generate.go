package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/holbrookab/go-ai/internal/retry"
)

func GenerateText(ctx context.Context, opts GenerateTextOptions) (result *GenerateTextResult, err error) {
	emitStart(ctx, opts.Telemetry, opts.TelemetryOptions, opts.OnStart, EventGenerateTextStart, OperationGenerateText, opts.Model, nil)
	defer func() {
		if err != nil {
			emitError(ctx, opts.Telemetry, opts.TelemetryOptions, opts.OnError, EventGenerateTextError, OperationGenerateText, err)
		}
	}()
	if opts.Model == nil {
		return nil, &SDKError{Kind: ErrInvalidArgument, Message: "model is required"}
	}
	if opts.Timeout.Total > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout.Total)
		defer cancel()
	}

	initialPrompt, err := standardizePrompt(opts.Instructions, opts.System, opts.Prompt, opts.Messages, opts.AllowSystemInMessages)
	if err != nil {
		return nil, err
	}
	stopWhen := opts.StopWhen
	if len(stopWhen) == 0 {
		stopWhen = []StopCondition{StepCount(1)}
	}
	maxRetries := 2
	if opts.MaxRetries != nil {
		maxRetries = *opts.MaxRetries
	}

	responseMessages := []Message{}
	replayedToolResults, err := replayToolApprovals(ctx, initialPrompt.Messages, opts.Tools, opts.ToolApproval, opts.ToolApprovalSecret, nil, opts.Sandbox, opts.Timeout, opts.ToolExecution, opts.OnToolExecutionStart, opts.OnToolExecutionEnd)
	if err != nil {
		return nil, err
	}
	if len(replayedToolResults) > 0 {
		parts := make([]Part, len(replayedToolResults))
		for i := range replayedToolResults {
			parts[i] = replayedToolResults[i]
		}
		responseMessages = append(responseMessages, Message{Role: RoleTool, Content: parts})
	}
	steps := []*StepResult{}
	currentMessages := append([]Message{}, initialPrompt.Messages...)
	var last *StepResult
	var clientToolCalls []ToolCall
	var clientToolResults []ToolResultPart
	pendingProviderResults := map[string]ToolCall{}
	toolsContext := map[string]any{}

	for {
		stepCtx := ctx
		var cancelStep context.CancelFunc
		if opts.Timeout.Step > 0 {
			stepCtx, cancelStep = context.WithTimeout(ctx, opts.Timeout.Step)
		}
		if cancelStep != nil {
			defer cancelStep()
		}

		stepNumber := len(steps)
		stepStarted := time.Now()
		stepID := defaultStepID(stepNumber)
		stepType := defaultStepType(stepNumber)
		model := opts.Model
		instructions := opts.Instructions
		system := opts.System
		stepMessages := append([]Message{}, currentMessages...)
		stepTools := opts.Tools
		stepToolOrder := append([]string(nil), opts.ToolOrder...)
		if len(opts.ActiveTools) > 0 {
			stepTools = FilterActiveTools(stepTools, opts.ActiveTools)
		}
		toolChoice := opts.ToolChoice
		if err := validatePreparedTools(opts.Tools, opts.ActiveTools, toolChoice); err != nil {
			return nil, err
		}
		providerOptions := cloneProviderOptions(opts.ProviderOptions)
		if opts.PrepareStep != nil {
			prepared, err := opts.PrepareStep(PrepareStepOptions{
				Model:        model,
				Instructions: instructions,
				System:       system,
				Steps:        steps,
				StepNumber:   stepNumber,
				Messages:     append(stepMessages, responseMessages...),
				ToolsContext: cloneAnyMap(toolsContext),
				ToolOrder:    append([]string(nil), stepToolOrder...),
				Sandbox:      opts.Sandbox,
			})
			if err != nil {
				return nil, err
			}
			if prepared != nil {
				if prepared.Model != nil {
					model = prepared.Model
				}
				if prepared.Instructions != "" {
					instructions = prepared.Instructions
				}
				if prepared.System != "" {
					system = prepared.System
				}
				if prepared.Messages != nil {
					currentMessages = append([]Message{}, prepared.Messages...)
					stepMessages = append([]Message{}, currentMessages...)
				}
				if prepared.Tools != nil {
					stepTools = prepared.Tools
				}
				if prepared.ToolOrder != nil {
					stepToolOrder = append([]string(nil), prepared.ToolOrder...)
				}
				if prepared.ToolChoice.Type != "" {
					toolChoice = prepared.ToolChoice
				}
				providerOptions = mergeProviderOptions(providerOptions, prepared.ProviderOptions)
				toolsContext = mergeToolsContext(toolsContext, prepared.ToolsContext)
				if prepared.Sandbox != nil {
					opts.Sandbox = prepared.Sandbox
				}
			}
		}
		if err := validatePreparedTools(stepTools, nil, toolChoice); err != nil {
			return nil, err
		}

		conversionOptions, err := promptConversionOptionsForModel(stepCtx, model)
		if err != nil {
			return nil, err
		}
		if opts.Download != nil {
			conversionOptions.Download = opts.Download
		}
		promptMessages, err := toLanguageModelPromptWithOptions(stepCtx, standardizedPrompt{
			System:   systemMessagesFor(instructions, system, initialPrompt.System),
			Messages: stepMessages,
		}, responseMessages, conversionOptions)
		if err != nil {
			return nil, err
		}

		callOptions := LanguageModelCallOptions{
			Prompt:           promptMessages,
			MaxOutputTokens:  opts.MaxOutputTokens,
			Temperature:      opts.Temperature,
			TopP:             opts.TopP,
			TopK:             opts.TopK,
			PresencePenalty:  opts.PresencePenalty,
			FrequencyPenalty: opts.FrequencyPenalty,
			StopSequences:    opts.StopSequences,
			Seed:             opts.Seed,
			Reasoning:        opts.Reasoning,
			ResponseFormat:   outputResponseFormat(opts.Output, opts.ResponseFormat),
			Tools:            prepareModelToolsWithOrder(stepTools, toolChoice, stepToolOrder),
			ToolChoice:       normalizeToolChoice(toolChoice),
			ProviderOptions:  providerOptions,
			Headers:          withUserAgent(opts.Headers, "go-ai/"+Version),
		}

		var modelResult *LanguageModelGenerateResult
		callID := emitLanguageModelCallStart(ctx, opts.Telemetry, opts.TelemetryOptions, OperationGenerateText, model, stepNumber, callOptions)
		if opts.OnStepStart != nil {
			opts.OnStepStart(StepStartEvent{Operation: OperationGenerateText, CallID: callID, StepNumber: stepNumber, Provider: model.Provider(), ModelID: model.ModelID(), Messages: append([]Message(nil), promptMessages...)})
		}
		if opts.OnLanguageModelCallStart != nil {
			opts.OnLanguageModelCallStart(LanguageModelCallStartEvent{Operation: OperationGenerateText, CallID: callID, StepNumber: stepNumber, Provider: model.Provider(), ModelID: model.ModelID(), Options: callOptions})
		}
		err = retry.Do(stepCtx, maxRetries, func() error {
			err := executeLanguageModelCall(stepCtx, opts.Telemetry, OperationGenerateText, model, stepNumber, callID, func(callCtx context.Context) error {
				result, err := model.DoGenerate(callCtx, callOptions)
				if err != nil {
					return err
				}
				modelResult = result
				return nil
			})
			if err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		if modelResult == nil {
			return nil, &SDKError{Kind: ErrNoOutputGenerated, Message: "model returned nil result"}
		}
		emitLanguageModelCallEnd(ctx, opts.Telemetry, opts.TelemetryOptions, OperationGenerateText, model, stepNumber, callID, modelResult)
		if opts.OnLanguageModelCallEnd != nil {
			opts.OnLanguageModelCallEnd(LanguageModelCallEndEvent{
				Operation: OperationGenerateText, CallID: callID, StepNumber: stepNumber, Provider: model.Provider(), ModelID: model.ModelID(), Result: modelResult,
				Content: append([]Part(nil), modelResult.Content...), FinishReason: modelResult.FinishReason, Usage: modelResult.Usage, Response: modelResult.Response,
			})
		}
		responseFinished := time.Now()

		toolCalls, parsedContent, err := parseToolCalls(stepCtx, modelResult.Content, parseToolCallsOptions{
			Tools:           stepTools,
			RepairToolCall:  opts.RepairToolCall,
			RefineToolInput: opts.RefineToolInput,
			System:          system,
			Instructions:    instructions,
			Messages:        promptMessages,
			ToolsContext:    cloneAnyMap(toolsContext),
		})
		if err != nil {
			return nil, err
		}
		clientToolCalls = clientToolCalls[:0]
		clientToolResults = clientToolResults[:0]
		blocked := map[string]bool{}
		precomputedToolResults := map[string]ToolResultPart{}
		var approvalResponses []Part

		for _, call := range toolCalls {
			if call.ProviderExecuted {
				if !hasToolResult(parsedContent, call.ToolCallID) {
					pendingProviderResults[call.ToolCallID] = call
				}
				continue
			}
			clientToolCalls = append(clientToolCalls, call)
			tool, ok := stepTools[call.ToolName]
			if !ok || tool.Execute == nil || call.Invalid {
				blocked[call.ToolCallID] = true
				continue
			}
			if opts.ToolApproval != nil || tool.RequiresApproval || tool.NeedsApproval != nil {
				decision, err := resolveToolApproval(stepCtx, stepTools, call, opts.ToolApproval, promptMessages, cloneAnyMap(toolsContext))
				if err != nil {
					return nil, err
				}
				if decision.Type != ApprovalDecisionNotApplicable {
					request, response, err := approvalPartsForCall(opts.ToolApprovalSecret, call, decision)
					if err != nil {
						return nil, err
					}
					parsedContent = append(parsedContent, request)
					if response != nil {
						approvalResponses = append(approvalResponses, *response)
					}
				}
				if ApprovalBlocksToolExecution(decision) {
					blocked[call.ToolCallID] = true
					if decision.Type == ApprovalDecisionDenied {
						output := ToolResultOutput{Type: "execution-denied", Reason: decision.Reason}
						precomputedToolResults[call.ToolCallID] = ToolResultPart{
							ToolCallID: call.ToolCallID, ToolName: call.ToolName, Input: call.Input, Output: output,
						}
					}
				}
			}
		}

		clientToolResults = executeToolCalls(stepCtx, clientToolCalls, stepTools, promptMessages, cloneAnyMap(toolsContext), opts.Sandbox, opts.Timeout.Tool, opts.ToolExecution, blocked, precomputedToolResults, opts.OnToolExecutionStart, opts.OnToolExecutionEnd)
		toolsFinished := time.Now()
		for _, part := range parsedContent {
			if result, ok := part.(ToolResultPart); ok {
				delete(pendingProviderResults, result.ToolCallID)
			}
		}

		stepContent := append([]Part{}, parsedContent...)
		stepContent = append(stepContent, approvalResponses...)
		for _, result := range clientToolResults {
			stepContent = append(stepContent, result)
		}

		stepResponseMessages := toResponseMessages(stepContent, clientToolResults)
		responseMessages = append(responseMessages, stepResponseMessages...)
		stepResponse := modelResult.Response
		stepResponse.Messages = append([]Message{}, stepResponseMessages...)
		if stepResponse.ID == "" {
			stepResponse.ID = fmt.Sprintf("resp-%d", time.Now().UnixNano())
		}
		if stepResponse.Timestamp.IsZero() {
			stepResponse.Timestamp = time.Now()
		}
		if stepResponse.ModelID == "" {
			stepResponse.ModelID = model.ModelID()
		}

		last = &StepResult{
			CallID:           callID,
			StepID:           stepID,
			StepNumber:       stepNumber,
			StepType:         stepType,
			Provider:         model.Provider(),
			ModelID:          model.ModelID(),
			Content:          stepContent,
			Text:             TextFromParts(stepContent),
			FinishReason:     modelResult.FinishReason.Unified,
			RawFinishReason:  modelResult.FinishReason.Raw,
			Usage:            modelResult.Usage,
			Performance:      performanceFromTimings(stepStarted, responseFinished, toolsFinished, modelResult.Usage, 0, nil),
			Warnings:         modelResult.Warnings,
			ProviderMetadata: modelResult.ProviderMetadata,
			Request:          applyIncludeToRequest(modelResult.Request, opts.Include),
			Response:         stepResponse,
			ToolCalls:        toolCalls,
			ToolResults:      clientToolResults,
			Files:            filesFromParts(stepContent),
			Sources:          sourcesFromParts(stepContent),
		}
		applyIncludeToStep(last, opts.Include)
		LogWarnings(modelResult.Warnings, model.Provider(), model.ModelID())
		steps = append(steps, last)
		emitStepFinish(ctx, opts.Telemetry, opts.TelemetryOptions, stepEndCallback(opts.OnStepEnd, opts.OnStepFinish), EventGenerateTextStepFinish, OperationGenerateText, last)

		shouldStop, err := stopConditionMet(ctx, stopWhen, steps)
		if err != nil {
			return nil, err
		}
		shouldContinue := ((len(clientToolCalls) > 0 && len(clientToolResults) == len(clientToolCalls)) || len(pendingProviderResults) > 0) && !shouldStop
		if !shouldContinue {
			break
		}
	}

	if last == nil {
		return nil, &SDKError{Kind: ErrNoOutputGenerated, Message: "no generation step completed"}
	}
	totalUsage := Usage{}
	var warnings []Warning
	var content []Part
	var toolCalls []ToolCall
	var toolResults []ToolResultPart
	var files []GeneratedFile
	var sources []SourcePart
	for _, step := range steps {
		totalUsage = AddUsage(totalUsage, step.Usage)
		warnings = append(warnings, step.Warnings...)
		content = append(content, step.Content...)
		toolCalls = append(toolCalls, step.ToolCalls...)
		toolResults = append(toolResults, step.ToolResults...)
		files = append(files, step.Files...)
		sources = append(sources, step.Sources...)
	}
	result = &GenerateTextResult{
		Text:             last.Text,
		Content:          content,
		FinishReason:     last.FinishReason,
		RawFinishReason:  last.RawFinishReason,
		Usage:            totalUsage,
		Warnings:         warnings,
		ProviderMetadata: last.ProviderMetadata,
		Request:          last.Request,
		Response:         applyIncludeToResponse(last.Response, opts.Include),
		Steps:            steps,
		ResponseMessages: append([]Message{}, responseMessages...),
		FinalStep:        last,
		ToolCalls:        toolCalls,
		ToolResults:      append(append([]ToolResultPart{}, replayedToolResults...), toolResults...),
		Files:            files,
		Sources:          sources,
	}
	output, outputGenerated, outputErr := parseCompleteTextOutput(opts.Output, result.Text, result.Response, result.Usage, FinishReason{Unified: result.FinishReason, Raw: result.RawFinishReason})
	result.Output = output
	result.OutputGenerated = outputGenerated
	result.OutputErr = outputErr
	if outputErr != nil && !IsNoOutputGeneratedError(outputErr) {
		return nil, outputErr
	}
	emitFinish(ctx, opts.Telemetry, opts.TelemetryOptions, endCallback(opts.OnEnd, opts.OnFinish), EventGenerateTextEnd, OperationGenerateText, result, map[string]any{
		"finish_reason":     result.FinishReason,
		"raw_finish_reason": result.RawFinishReason,
		"usage":             result.Usage,
		"step_count":        len(result.Steps),
	})
	return result, nil
}

func prepareModelTools(tools map[string]Tool, choice ToolChoice) []ModelTool {
	return prepareModelToolsWithOrder(tools, choice, nil)
}

func prepareModelToolsWithOrder(tools map[string]Tool, choice ToolChoice, toolOrder []string) []ModelTool {
	if len(tools) == 0 || choice.Type == "none" {
		return nil
	}
	out := make([]ModelTool, 0, len(tools))
	seen := map[string]bool{}
	names := make([]string, 0, len(tools))
	for _, name := range toolOrder {
		if _, ok := tools[name]; ok && !seen[name] {
			names = append(names, name)
			seen[name] = true
		}
	}
	var remaining []string
	for name := range tools {
		if !seen[name] {
			remaining = append(remaining, name)
		}
	}
	sort.Strings(remaining)
	names = append(names, remaining...)
	for _, name := range names {
		tool := tools[name]
		if choice.Type == "tool" && choice.ToolName != "" && choice.ToolName != name {
			continue
		}
		out = append(out, tool.toModelTool(name))
	}
	return out
}

func validatePreparedTools(tools map[string]Tool, activeTools []string, choice ToolChoice) error {
	if len(activeTools) > 0 {
		for _, name := range activeTools {
			if _, ok := tools[name]; !ok {
				return &SDKError{Kind: ErrNoSuchTool, Message: fmt.Sprintf("active tool %q is not defined", name)}
			}
		}
	}
	if choice.Type == "tool" && choice.ToolName != "" {
		if _, ok := tools[choice.ToolName]; !ok {
			return NewNoSuchToolError(choice.ToolName, availableToolNames(tools))
		}
	}
	for name, tool := range tools {
		if name == "" {
			return &SDKError{Kind: ErrInvalidArgument, Message: "tool name must not be empty"}
		}
		if tool.Type == "provider" && tool.ID == "" {
			return &SDKError{Kind: ErrInvalidArgument, Message: fmt.Sprintf("provider tool %q requires an id", name)}
		}
		if tool.Type != "provider" && tool.InputSchema != nil {
			if _, ok := normalizeSchema(tool.InputSchema).(map[string]any); !ok {
				return &SDKError{Kind: ErrInvalidArgument, Message: fmt.Sprintf("tool %q input schema must be a JSON schema object", name)}
			}
		}
	}
	return nil
}

func normalizeToolChoice(choice ToolChoice) ToolChoice {
	if choice.Type == "" {
		return AutoToolChoice()
	}
	return choice
}

type parseToolCallsOptions struct {
	Tools           map[string]Tool
	RepairToolCall  ToolCallRepairFunc
	RefineToolInput ToolInputRefineFunc
	Instructions    string
	System          string
	Messages        []Message
	ToolsContext    map[string]any
}

func parseToolCalls(ctx context.Context, parts []Part, opts parseToolCallsOptions) ([]ToolCall, []Part, error) {
	var calls []ToolCall
	parsedParts := append([]Part(nil), parts...)
	for _, part := range parts {
		toolPart, ok := part.(ToolCallPart)
		if !ok {
			continue
		}
		call, repairedPart, err := parseToolCall(ctx, toolPart, opts)
		if err != nil {
			return nil, nil, err
		}
		calls = append(calls, call)
		if repairedPart != nil {
			for i := range parsedParts {
				if existing, ok := parsedParts[i].(ToolCallPart); ok && existing.ToolCallID == toolPart.ToolCallID {
					parsedParts[i] = *repairedPart
					break
				}
			}
		}
	}
	return calls, parsedParts, nil
}

func parseToolCall(ctx context.Context, toolPart ToolCallPart, opts parseToolCallsOptions) (ToolCall, *ToolCallPart, error) {
	call := parseToolCallWithoutRepair(toolPart, opts.Tools)
	if !call.Invalid {
		return refineToolCallInput(ctx, call, toolPart, opts)
	}
	if !call.Invalid || opts.RepairToolCall == nil || !isRepairableToolCallError(call.Error) {
		return call, nil, nil
	}

	repairedPart, err := opts.RepairToolCall(ctx, ToolCallRepairOptions{
		Instructions: opts.Instructions,
		System:       opts.System,
		Messages:     append([]Message(nil), opts.Messages...),
		ToolCall:     toolPart,
		Tools:        opts.Tools,
		InputSchema:  inputSchemaLookup(opts.Tools),
		Error:        call.Error,
	})
	if err != nil {
		return ToolCall{}, nil, NewToolCallRepairError(err, call.Error)
	}
	if repairedPart == nil {
		return call, nil, nil
	}

	repairedCall := parseToolCallWithoutRepair(*repairedPart, opts.Tools)
	if repairedCall.Invalid {
		return call, nil, nil
	}
	refinedCall, refinedPart, err := refineToolCallInput(ctx, repairedCall, *repairedPart, opts)
	if err != nil {
		return ToolCall{}, nil, err
	}
	if refinedPart != nil {
		return refinedCall, refinedPart, nil
	}
	return refinedCall, repairedPart, nil
}

func parseToolCallWithoutRepair(toolPart ToolCallPart, tools map[string]Tool) ToolCall {
	call := ToolCall{
		ToolCallID:       toolPart.ToolCallID,
		ToolName:         toolPart.ToolName,
		ProviderExecuted: toolPart.ProviderExecuted,
		Dynamic:          toolPart.Dynamic,
		ToolMetadata:     toolPart.ToolMetadata,
		ProviderMetadata: toolPart.ProviderMetadata,
	}
	input := toolPart.Input
	if toolPart.InputRaw != "" {
		if err := json.Unmarshal([]byte(toolPart.InputRaw), &input); err != nil {
			call.Invalid = true
			call.Error = &SDKError{Kind: ErrInvalidToolInput, Message: "tool input is not valid JSON", Cause: err}
			input = toolPart.InputRaw
		}
	}
	if input == nil {
		input = map[string]any{}
	}
	call.Input = input
	if tool, ok := tools[call.ToolName]; ok {
		call.ProviderMetadata = mergeMetadata(tool.ProviderMetadata, call.ProviderMetadata)
		if !call.Invalid {
			if err := ValidateToolInput(tool, input); err != nil {
				call.Invalid = true
				call.Error = &SDKError{Kind: ErrInvalidToolInput, Message: "tool input validation failed", Cause: err}
			}
		}
	} else if !call.ProviderExecuted {
		call.Invalid = true
		call.Dynamic = true
		call.Error = NewNoSuchToolError(call.ToolName, availableToolNames(tools))
	}
	return call
}

func refineToolCallInput(ctx context.Context, call ToolCall, toolPart ToolCallPart, opts parseToolCallsOptions) (ToolCall, *ToolCallPart, error) {
	if call.Invalid || opts.RefineToolInput == nil {
		return call, nil, nil
	}
	tool, ok := opts.Tools[call.ToolName]
	if !ok {
		return call, nil, nil
	}
	refined, err := opts.RefineToolInput(ctx, ToolInputRefineOptions{
		ToolCall: call,
		Tool:     tool,
		Messages: append([]Message(nil), opts.Messages...),
		Context:  cloneAnyMap(opts.ToolsContext),
	})
	if err != nil {
		call.Invalid = true
		call.Error = &SDKError{Kind: ErrInvalidToolInput, Message: "tool input refinement failed", Cause: err}
		return call, &toolPart, nil
	}
	toolPart.Input = refined
	toolPart.InputRaw = mustJSON(refined)
	refinedCall := parseToolCallWithoutRepair(toolPart, opts.Tools)
	return refinedCall, &toolPart, nil
}

func isRepairableToolCallError(err error) bool {
	return IsNoSuchToolError(err) || errors.Is(err, ErrInvalidToolInput)
}

func inputSchemaLookup(tools map[string]Tool) func(string) (any, bool) {
	return func(toolName string) (any, bool) {
		tool, ok := tools[toolName]
		if !ok {
			return nil, false
		}
		return normalizeSchema(tool.InputSchema), true
	}
}

func availableToolNames(tools map[string]Tool) []string {
	if len(tools) == 0 {
		return nil
	}
	names := make([]string, 0, len(tools))
	for name := range tools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func executeTool(ctx context.Context, timeout time.Duration, call ToolCall, tool Tool, messages []Message, toolsContext map[string]any, sandbox Sandbox, onStart func(ToolExecutionStartEvent), onEnd func(ToolExecutionEndEvent)) ToolResultPart {
	execCtx := ctx
	var cancel context.CancelFunc
	if timeout > 0 {
		execCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	if onStart != nil {
		onStart(ToolExecutionStartEvent{ToolCall: call, Messages: append([]Message(nil), messages...)})
	}
	output, err := tool.Execute(execCtx, call, ToolExecutionOptions{ToolCallID: call.ToolCallID, Messages: messages, Context: toolsContext, Sandbox: sandbox})
	modelOutput, modelErr := CreateToolModelOutput(tool, call.ToolCallID, call.Input, firstNonNil(output, errString(err)), err != nil)
	if modelErr != nil {
		err = modelErr
		modelOutput = ToolResultOutput{Type: "error-text", Value: err.Error()}
	}
	result := ToolResultPart{
		ToolCallID:       call.ToolCallID,
		ToolName:         call.ToolName,
		Input:            call.Input,
		Output:           modelOutput,
		Result:           output,
		IsError:          err != nil,
		Dynamic:          call.Dynamic,
		ToolMetadata:     call.ToolMetadata,
		ProviderMetadata: call.ProviderMetadata,
	}
	if onEnd != nil {
		onEnd(ToolExecutionEndEvent{ToolCall: call, Result: result, Err: err})
	}
	return result
}

func executeToolCalls(ctx context.Context, calls []ToolCall, tools map[string]Tool, messages []Message, toolsContext map[string]any, sandbox Sandbox, timeout time.Duration, mode ToolExecutionMode, blocked map[string]bool, precomputed map[string]ToolResultPart, onStart func(ToolExecutionStartEvent), onEnd func(ToolExecutionEndEvent)) []ToolResultPart {
	if len(calls) == 0 {
		return nil
	}
	if mode == ToolExecutionSequential {
		results := make([]ToolResultPart, 0, len(calls))
		for _, call := range calls {
			if result, ok := precomputed[call.ToolCallID]; ok {
				results = append(results, result)
				continue
			}
			if blocked[call.ToolCallID] {
				continue
			}
			tool, ok := tools[call.ToolName]
			if !ok || tool.Execute == nil || call.Invalid {
				continue
			}
			results = append(results, executeTool(ctx, timeout, call, tool, messages, cloneAnyMap(toolsContext), sandbox, onStart, onEnd))
		}
		return results
	}

	results := make([]ToolResultPart, len(calls))
	var wg sync.WaitGroup
	for index, call := range calls {
		if result, ok := precomputed[call.ToolCallID]; ok {
			results[index] = result
			continue
		}
		if blocked[call.ToolCallID] {
			continue
		}
		tool, ok := tools[call.ToolName]
		if !ok || tool.Execute == nil || call.Invalid {
			continue
		}
		wg.Add(1)
		go func(index int, call ToolCall, tool Tool) {
			defer wg.Done()
			results[index] = executeTool(ctx, timeout, call, tool, messages, cloneAnyMap(toolsContext), sandbox, onStart, onEnd)
		}(index, call, tool)
	}
	wg.Wait()

	out := make([]ToolResultPart, 0, len(results))
	for _, result := range results {
		if result.ToolCallID == "" {
			continue
		}
		out = append(out, result)
	}
	return out
}

func defaultStepID(stepNumber int) string {
	return fmt.Sprintf("step-%d", stepNumber)
}

func defaultStepType(stepNumber int) string {
	if stepNumber == 0 {
		return "initial"
	}
	return "tool-result"
}

func mergeToolsContext(base map[string]any, overrides map[string]any) map[string]any {
	if len(base) == 0 && len(overrides) == 0 {
		return map[string]any{}
	}
	out := cloneAnyMap(base)
	for key, value := range overrides {
		out[key] = value
	}
	return out
}

func toResponseMessages(content []Part, toolResults []ToolResultPart) []Message {
	var messages []Message
	assistantContent := []Part{}
	for _, part := range content {
		switch p := part.(type) {
		case TextPart, ReasoningPart, ReasoningFilePart, FilePart, ToolCallPart, ToolApprovalRequestPart:
			assistantContent = append(assistantContent, p)
		}
	}
	if len(assistantContent) > 0 {
		messages = append(messages, Message{Role: RoleAssistant, Content: assistantContent})
	}
	var parts []Part
	for _, part := range content {
		if response, ok := part.(ToolApprovalResponsePart); ok {
			parts = append(parts, response)
		}
	}
	for i := range toolResults {
		parts = append(parts, toolResults[i])
	}
	if len(parts) > 0 {
		messages = append(messages, Message{Role: RoleTool, Content: parts})
	}
	return messages
}

func hasToolResult(parts []Part, id string) bool {
	for _, part := range parts {
		if result, ok := part.(ToolResultPart); ok && result.ToolCallID == id {
			return true
		}
	}
	return false
}

func systemMessages(system string, fallback []Message) []Message {
	if system != "" {
		return []Message{SystemMessage(system)}
	}
	return fallback
}

func withUserAgent(headers map[string]string, suffix string) map[string]string {
	out := map[string]string{}
	for k, v := range headers {
		out[k] = v
	}
	if out["User-Agent"] == "" {
		out["User-Agent"] = suffix
	} else {
		out["User-Agent"] += " " + suffix
	}
	return out
}

func cloneProviderOptions(in ProviderOptions) ProviderOptions {
	if in == nil {
		return nil
	}
	out := ProviderOptions{}
	for k, v := range in {
		out[k] = cloneJSONValue(v)
	}
	return out
}

func mergeProviderOptions(a, b ProviderOptions) ProviderOptions {
	if len(b) == 0 {
		return a
	}
	if a == nil {
		a = ProviderOptions{}
	}
	for k, v := range b {
		if existing, ok := a[k]; ok {
			if merged, ok := mergeObjectValues(existing, v); ok {
				a[k] = merged
				continue
			}
		}
		a[k] = cloneJSONValue(v)
	}
	return a
}

func mergeObjectValues(a, b any) (any, bool) {
	left, leftOK := a.(map[string]any)
	right, rightOK := b.(map[string]any)
	if !leftOK || !rightOK {
		return nil, false
	}
	out := map[string]any{}
	for k, v := range left {
		out[k] = cloneJSONValue(v)
	}
	for k, v := range right {
		if existing, ok := out[k]; ok {
			if merged, ok := mergeObjectValues(existing, v); ok {
				out[k] = merged
				continue
			}
		}
		out[k] = cloneJSONValue(v)
	}
	return out, true
}

func mergeMetadata(a, b ProviderMetadata) ProviderMetadata {
	if len(a) == 0 {
		return b
	}
	out := ProviderMetadata{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func errString(err error) any {
	if err == nil {
		return nil
	}
	return err.Error()
}
