package ai

import (
	"context"
	"time"
)

type GenerateTextOptions struct {
	Model                 LanguageModel
	Instructions          string
	System                string
	Prompt                string
	Messages              []Message
	AllowSystemInMessages bool
	Tools                 map[string]Tool
	ActiveTools           []string
	ToolChoice            ToolChoice
	ToolExecution         ToolExecutionMode
	ToolApproval          *ToolApprovalConfiguration
	StopWhen              []StopCondition
	MaxRetries            *int
	Timeout               TimeoutConfig
	Headers               map[string]string
	Include               IncludeConfig
	ProviderOptions       ProviderOptions
	MaxOutputTokens       *int
	Temperature           *float64
	TopP                  *float64
	TopK                  *float64
	PresencePenalty       *float64
	FrequencyPenalty      *float64
	StopSequences         []string
	Seed                  *int
	Reasoning             string
	Download              DownloadFunction
	Output                *OutputStrategy
	ResponseFormat        *ResponseFormat
	Sandbox               Sandbox
	PrepareStep           func(PrepareStepOptions) (*PrepareStepResult, error)
	RepairToolCall        ToolCallRepairFunc
	RefineToolInput       ToolInputRefineFunc
	Telemetry             Telemetry
	TelemetryOptions      TelemetryOptions
	OnStart               func(StartEvent)
	OnToolExecutionStart  func(ToolExecutionStartEvent)
	OnToolExecutionEnd    func(ToolExecutionEndEvent)
	OnStepFinish          func(StepFinishEvent)
	OnFinish              func(FinishEvent)
	OnError               func(ErrorEvent)
}

type StreamTextOptions struct {
	GenerateTextOptions
	IncludeRawChunks bool
	OnChunk          func(ChunkEvent)
	Transforms       []StreamTransform
}

type ToolExecutionMode string

const (
	ToolExecutionParallel   ToolExecutionMode = "parallel"
	ToolExecutionSequential ToolExecutionMode = "sequential"
)

type StreamTransform func(context.Context, <-chan StreamPart, StreamTransformOptions) <-chan StreamPart

type StreamTransformOptions struct {
	Tools      map[string]Tool
	StopStream func()
}

type ChunkDetector func(buffer string) (chunk string, ok bool, err error)

type SmoothStreamChunking string

const (
	SmoothStreamChunkByWord SmoothStreamChunking = "word"
	SmoothStreamChunkByLine SmoothStreamChunking = "line"
)

type SmoothStreamOptions struct {
	Delay       *time.Duration
	Chunking    SmoothStreamChunking
	DetectChunk ChunkDetector
}

type TimeoutConfig struct {
	Total time.Duration
	Step  time.Duration
	Tool  time.Duration
	Chunk time.Duration
}

type IncludeConfig struct {
	RequestBody     bool
	ResponseBody    bool
	RequestMessages bool
	RawChunks       bool
}

type Sandbox interface {
	RunCommand(context.Context, SandboxCommand) (SandboxCommandResult, error)
	ReadFile(context.Context, string) ([]byte, error)
	WriteFile(context.Context, string, []byte) error
}

type SandboxCommand struct {
	Command []string
	Stdin   string
	Env     map[string]string
	Dir     string
	Timeout time.Duration
}

type SandboxCommandResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

type PrepareStepOptions struct {
	Model        LanguageModel
	Instructions string
	System       string
	Steps        []*StepResult
	StepNumber   int
	Messages     []Message
	ToolsContext map[string]any
	Sandbox      Sandbox
}

type PrepareStepResult struct {
	Model           LanguageModel
	Instructions    string
	System          string
	Messages        []Message
	Tools           map[string]Tool
	ToolChoice      ToolChoice
	ProviderOptions ProviderOptions
	ToolsContext    map[string]any
	Sandbox         Sandbox
}

type GenerateTextResult struct {
	Text             string
	Output           any
	OutputGenerated  bool
	OutputErr        error
	Content          []Part
	FinishReason     string
	RawFinishReason  string
	Usage            Usage
	Warnings         []Warning
	ProviderMetadata ProviderMetadata
	Request          RequestMetadata
	Response         ResponseMetadata
	Steps            []*StepResult
	ResponseMessages []Message
	FinalStep        *StepResult
	ToolCalls        []ToolCall
	ToolResults      []ToolResultPart
	Files            []GeneratedFile
	Sources          []SourcePart
}

type StreamTextResult struct {
	Stream           <-chan StreamPart
	Text             string
	Output           any
	OutputGenerated  bool
	OutputErr        error
	Content          []Part
	FinishReason     string
	RawFinishReason  string
	Usage            Usage
	Warnings         []Warning
	ProviderMetadata ProviderMetadata
	Request          RequestMetadata
	Response         ResponseMetadata
	Steps            []*StepResult
	ResponseMessages []Message
	FinalStep        *StepResult
	ToolCalls        []ToolCall
	ToolResults      []ToolResultPart
	Files            []GeneratedFile
	Sources          []SourcePart
	Aborted          bool
	AbortReason      string
}

type GenerateObjectOptions struct {
	Model                 LanguageModel
	Output                string
	Mode                  string
	Schema                any
	SchemaName            string
	SchemaDescription     string
	Enum                  []string
	Instructions          string
	System                string
	Prompt                string
	Messages              []Message
	AllowSystemInMessages bool
	MaxRetries            *int
	Timeout               TimeoutConfig
	Headers               map[string]string
	ProviderOptions       ProviderOptions
	MaxOutputTokens       *int
	Temperature           *float64
	TopP                  *float64
	TopK                  *float64
	PresencePenalty       *float64
	FrequencyPenalty      *float64
	StopSequences         []string
	Seed                  *int
	Reasoning             string
	Download              DownloadFunction
	RepairText            func(RepairTextOptions) (string, error)
	Telemetry             Telemetry
	TelemetryOptions      TelemetryOptions
	OnStart               func(StartEvent)
	OnFinish              func(FinishEvent)
	OnError               func(ErrorEvent)
}

type StreamObjectOptions struct {
	GenerateObjectOptions
	IncludeRawChunks bool
}

type RepairTextOptions struct {
	Text  string
	Error error
}

type ToolCallRepairFunc func(context.Context, ToolCallRepairOptions) (*ToolCallPart, error)

type ToolCallRepairOptions struct {
	Instructions string
	System       string
	Messages     []Message
	ToolCall     ToolCallPart
	Tools        map[string]Tool
	InputSchema  func(toolName string) (any, bool)
	Error        error
}

type GenerateObjectResult struct {
	Object           any
	FinishReason     string
	RawFinishReason  string
	Usage            Usage
	Warnings         []Warning
	ProviderMetadata ProviderMetadata
	Request          RequestMetadata
	Response         ResponseMetadata
	Reasoning        string
	Text             string
}

type StreamObjectResult struct {
	Stream   <-chan ObjectStreamPart
	Elements <-chan any
	Request  RequestMetadata
	Response ResponseMetadata
}

type ObjectStreamPart struct {
	Type             string
	TextDelta        string
	Object           any
	Element          any
	FinishReason     FinishReason
	Usage            Usage
	Warnings         []Warning
	ProviderMetadata ProviderMetadata
	Raw              any
	AbortReason      string
	Err              error
}

type EmbedOptions struct {
	Model            EmbeddingModel
	Value            string
	MaxRetries       *int
	Headers          map[string]string
	ProviderOptions  ProviderOptions
	Telemetry        Telemetry
	TelemetryOptions TelemetryOptions
	OnStart          func(StartEvent)
	OnFinish         func(FinishEvent)
	OnError          func(ErrorEvent)
}

type EmbedManyOptions struct {
	Model            EmbeddingModel
	Values           []string
	MaxRetries       *int
	Headers          map[string]string
	ProviderOptions  ProviderOptions
	MaxParallelCalls int
	Telemetry        Telemetry
	TelemetryOptions TelemetryOptions
	OnStart          func(StartEvent)
	OnFinish         func(FinishEvent)
	OnError          func(ErrorEvent)
}

type EmbedResult struct {
	Value            string
	Embedding        []float64
	Usage            EmbeddingUsage
	Warnings         []Warning
	ProviderMetadata ProviderMetadata
	Response         ResponseMetadata
}

type EmbedManyResult struct {
	Values           []string
	Embeddings       [][]float64
	Usage            EmbeddingUsage
	Warnings         []Warning
	ProviderMetadata ProviderMetadata
	Responses        []ResponseMetadata
}

type StepResult struct {
	CallID           string
	StepID           string
	StepNumber       int
	StepType         string
	Provider         string
	ModelID          string
	Content          []Part
	Text             string
	FinishReason     string
	RawFinishReason  string
	Usage            Usage
	Performance      StepPerformance
	Warnings         []Warning
	ProviderMetadata ProviderMetadata
	Request          RequestMetadata
	Response         ResponseMetadata
	ToolCalls        []ToolCall
	ToolResults      []ToolResultPart
	Files            []GeneratedFile
	Sources          []SourcePart
}

type StepPerformance struct {
	EffectiveOutputTokensPerSecond *float64
	OutputTokensPerSecond          *float64
	InputTokensPerSecond           *float64
	EffectiveTotalTokensPerSecond  *float64
	StepTime                       time.Duration
	ResponseTime                   time.Duration
	ToolExecutionTime              time.Duration
	TimeToFirstOutputToken         time.Duration
}
