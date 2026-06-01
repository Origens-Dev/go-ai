package ai

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestParseToolCallRefinesInputAndValidatesRefinedValue(t *testing.T) {
	call, repaired, err := parseToolCall(context.Background(), ToolCallPart{
		ToolCallID: "call-1",
		ToolName:   "trim",
		InputRaw:   `{"value":" raw "}`,
	}, parseToolCallsOptions{
		Tools: map[string]Tool{
			"trim": {
				InputSchema: map[string]any{
					"type":       "object",
					"properties": map[string]any{"value": map[string]any{"type": "string", "pattern": "^raw$"}},
				},
			},
		},
		RefineToolInput: func(_ context.Context, opts ToolInputRefineOptions) (any, error) {
			input := opts.ToolCall.Input.(map[string]any)
			return map[string]any{"value": "raw", "original": input["value"]}, nil
		},
	})
	if err != nil {
		t.Fatalf("parseToolCall failed: %v", err)
	}
	if call.Invalid {
		t.Fatalf("refined call should be valid: %#v", call)
	}
	if call.Input.(map[string]any)["value"] != "raw" {
		t.Fatalf("input was not refined: %#v", call.Input)
	}
	if repaired == nil || repaired.InputRaw == "" {
		t.Fatalf("expected repaired/refined part, got %#v", repaired)
	}
}

func TestParseToolCallMarksInvalidWhenRefinementBreaksSchema(t *testing.T) {
	call, _, err := parseToolCall(context.Background(), ToolCallPart{
		ToolCallID: "call-1",
		ToolName:   "trim",
		InputRaw:   `{"value":"raw"}`,
	}, parseToolCallsOptions{
		Tools: map[string]Tool{
			"trim": {
				InputSchema: map[string]any{
					"type":       "object",
					"properties": map[string]any{"value": map[string]any{"type": "string"}},
				},
			},
		},
		RefineToolInput: func(context.Context, ToolInputRefineOptions) (any, error) {
			return map[string]any{"value": 42}, nil
		},
	})
	if err != nil {
		t.Fatalf("parseToolCall failed: %v", err)
	}
	if !call.Invalid || call.Error == nil {
		t.Fatalf("expected invalid refined call, got %#v", call)
	}
}

func TestParseToolCallAllowsProviderExecutedDynamicWithoutTools(t *testing.T) {
	call, _, err := parseToolCall(context.Background(), ToolCallPart{
		ToolCallID:       "call-1",
		ToolName:         "server_tool",
		InputRaw:         `{"query":"x"}`,
		ProviderExecuted: true,
		Dynamic:          true,
		ProviderMetadata: ProviderMetadata{"provider": map[string]any{"signature": "sig"}},
	}, parseToolCallsOptions{})
	if err != nil {
		t.Fatalf("parseToolCall failed: %v", err)
	}
	if call.Invalid || !call.ProviderExecuted || !call.Dynamic {
		t.Fatalf("unexpected provider-executed call: %#v", call)
	}
	if call.ProviderMetadata["provider"] == nil {
		t.Fatalf("provider metadata was not preserved: %#v", call)
	}
}

func TestExecuteToolPropagatesSandboxAndMetadata(t *testing.T) {
	sandbox := testSandbox{}
	result := executeTool(context.Background(), 0, ToolCall{
		ToolCallID:       "call-1",
		ToolName:         "run",
		Input:            map[string]any{"value": "ok"},
		ToolMetadata:     ProviderMetadata{"client": "mcp"},
		ProviderMetadata: ProviderMetadata{"provider": map[string]any{"signature": "sig"}},
	}, Tool{
		Execute: func(_ context.Context, call ToolCall, opts ToolExecutionOptions) (any, error) {
			if opts.Sandbox == nil {
				t.Fatalf("expected sandbox")
			}
			if call.ToolMetadata["client"] != "mcp" {
				t.Fatalf("missing tool metadata: %#v", call)
			}
			return "done", nil
		},
	}, nil, nil, sandbox, nil, nil)
	if result.Output.Type != "text" || result.Output.Value != "done" {
		t.Fatalf("unexpected output: %#v", result.Output)
	}
	if result.ToolMetadata["client"] != "mcp" || result.ProviderMetadata["provider"] == nil {
		t.Fatalf("metadata was not preserved: %#v", result)
	}
}

func TestExecuteToolReturnsErrorOutput(t *testing.T) {
	result := executeTool(context.Background(), 0, ToolCall{
		ToolCallID: "call-1",
		ToolName:   "fail",
		Input:      map[string]any{},
	}, Tool{
		Execute: func(context.Context, ToolCall, ToolExecutionOptions) (any, error) {
			return nil, errors.New("boom")
		},
	}, nil, nil, nil, nil, nil)
	if !result.IsError || result.Output.Type != "error-text" || result.Output.Value != "boom" {
		t.Fatalf("unexpected error output: %#v", result)
	}
}

func TestPerformanceFromTimingsComputesThroughput(t *testing.T) {
	start := time.Unix(0, 0)
	response := start.Add(500 * time.Millisecond)
	finished := start.Add(time.Second)
	in, out, total := 2, 10, 12
	perf := performanceFromTimings(start, response, finished, Usage{
		InputTokens:  &in,
		OutputTokens: &out,
		TotalTokens:  &total,
	}, 100*time.Millisecond, []time.Duration{10 * time.Millisecond, 20 * time.Millisecond})
	if perf.OutputTokensPerSecond == nil || *perf.OutputTokensPerSecond != 10 {
		t.Fatalf("unexpected output tps: %#v", perf.OutputTokensPerSecond)
	}
	if perf.InputTokensPerSecond == nil || *perf.InputTokensPerSecond != 2 {
		t.Fatalf("unexpected input tps: %#v", perf.InputTokensPerSecond)
	}
	if perf.TimeToFirstOutputToken != 100*time.Millisecond {
		t.Fatalf("unexpected ttft: %#v", perf.TimeToFirstOutputToken)
	}
	if perf.TimeToFirstOutput != 100*time.Millisecond {
		t.Fatalf("unexpected time to first output: %#v", perf.TimeToFirstOutput)
	}
	if perf.TimeBetweenOutputChunks == nil || perf.TimeBetweenOutputChunks.Min != 10*time.Millisecond || perf.TimeBetweenOutputChunks.Max != 20*time.Millisecond {
		t.Fatalf("unexpected output chunk timing stats: %#v", perf.TimeBetweenOutputChunks)
	}
}

func TestSandboxSpawnerOptionalInterface(t *testing.T) {
	sandbox := testSandbox{}
	spawner, ok := any(sandbox).(SandboxSpawner)
	if !ok {
		t.Fatalf("test sandbox should implement optional spawner")
	}
	process, err := spawner.SpawnCommand(context.Background(), SandboxCommand{Command: []string{"echo", "ok"}})
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := io.ReadAll(process.Stdout())
	if err != nil {
		t.Fatal(err)
	}
	if string(stdout) != "ok" {
		t.Fatalf("stdout = %q", stdout)
	}
	result, err := process.Wait(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("result = %#v", result)
	}
}

type testSandbox struct{}

func (testSandbox) RunCommand(context.Context, SandboxCommand) (SandboxCommandResult, error) {
	return SandboxCommandResult{Stdout: "ok"}, nil
}

func (testSandbox) ReadFile(context.Context, string) ([]byte, error) {
	return []byte("ok"), nil
}

func (testSandbox) WriteFile(context.Context, string, []byte) error {
	return nil
}

func (testSandbox) SpawnCommand(context.Context, SandboxCommand) (SandboxProcess, error) {
	return testSandboxProcess{}, nil
}

type testSandboxProcess struct{}

func (testSandboxProcess) Stdout() io.Reader {
	return strings.NewReader("ok")
}

func (testSandboxProcess) Stderr() io.Reader {
	return strings.NewReader("")
}

func (testSandboxProcess) Wait(context.Context) (SandboxCommandResult, error) {
	return SandboxCommandResult{ExitCode: 0, Stdout: "ok"}, nil
}

func (testSandboxProcess) Kill(context.Context) error {
	return nil
}
