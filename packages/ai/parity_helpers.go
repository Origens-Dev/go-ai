package ai

import (
	"encoding/json"
	"strings"
	"time"
)

func combinedInstructions(instructions, system string) string {
	var parts []string
	if strings.TrimSpace(instructions) != "" {
		parts = append(parts, instructions)
	}
	if strings.TrimSpace(system) != "" {
		parts = append(parts, system)
	}
	return strings.Join(parts, "\n")
}

func systemMessagesFor(instructions, system string, fallback []Message) []Message {
	if strings.TrimSpace(instructions) == "" && strings.TrimSpace(system) == "" {
		return append([]Message(nil), fallback...)
	}
	var out []Message
	if strings.TrimSpace(instructions) != "" {
		out = append(out, SystemMessage(instructions))
	}
	if strings.TrimSpace(system) != "" {
		out = append(out, SystemMessage(system))
	}
	return out
}

func includeRawChunks(include IncludeConfig, includeRawChunks bool) bool {
	return include.RawChunks || includeRawChunks
}

func applyIncludeToRequest(request RequestMetadata, include IncludeConfig) RequestMetadata {
	if !include.RequestBody {
		request.Body = nil
	}
	return request
}

func applyIncludeToResponse(response ResponseMetadata, include IncludeConfig) ResponseMetadata {
	if !include.ResponseBody {
		response.Body = nil
	}
	return response
}

func applyIncludeToStep(step *StepResult, include IncludeConfig) {
	if step == nil {
		return
	}
	step.Request = applyIncludeToRequest(step.Request, include)
	step.Response = applyIncludeToResponse(step.Response, IncludeConfig{
		RequestBody:     include.RequestBody,
		ResponseBody:    include.ResponseBody,
		RequestMessages: true,
		RawChunks:       include.RawChunks,
	})
}

func filesFromParts(parts []Part) []GeneratedFile {
	var files []GeneratedFile
	for _, part := range parts {
		switch p := part.(type) {
		case FilePart:
			files = append(files, generatedFileFromData(p.Data, p.MediaType, p.Filename))
		case ReasoningFilePart:
			files = append(files, generatedFileFromData(p.Data, p.MediaType, ""))
		case ToolResultPart:
			for _, file := range p.Output.Files {
				files = append(files, GeneratedFile{
					Data:      cloneBytes(file.Data),
					URL:       file.URL,
					MediaType: file.MediaType,
					Filename:  file.Filename,
				})
			}
		}
	}
	return files
}

func generatedFileFromData(data FileData, mediaType, filename string) GeneratedFile {
	return GeneratedFile{
		Data:      cloneBytes(data.Data),
		URL:       data.URL,
		MediaType: mediaType,
		Filename:  filename,
	}
}

func sourcesFromParts(parts []Part) []SourcePart {
	var sources []SourcePart
	for _, part := range parts {
		if source, ok := part.(SourcePart); ok {
			sources = append(sources, source)
		}
	}
	return sources
}

func performanceFromTimings(stepStarted, responseFinished, toolsFinished time.Time, usage Usage, ttft time.Duration) StepPerformance {
	now := time.Now()
	if toolsFinished.IsZero() {
		toolsFinished = responseFinished
	}
	if toolsFinished.IsZero() {
		toolsFinished = now
	}
	if responseFinished.IsZero() {
		responseFinished = toolsFinished
	}
	perf := StepPerformance{
		StepTime:               toolsFinished.Sub(stepStarted),
		ResponseTime:           responseFinished.Sub(stepStarted),
		ToolExecutionTime:      toolsFinished.Sub(responseFinished),
		TimeToFirstOutputToken: ttft,
	}
	seconds := perf.StepTime.Seconds()
	if seconds <= 0 {
		return perf
	}
	if usage.OutputTokens != nil {
		v := float64(*usage.OutputTokens) / seconds
		perf.OutputTokensPerSecond = &v
		perf.EffectiveOutputTokensPerSecond = &v
	}
	if usage.InputTokens != nil {
		v := float64(*usage.InputTokens) / seconds
		perf.InputTokensPerSecond = &v
	}
	if usage.TotalTokens != nil {
		v := float64(*usage.TotalTokens) / seconds
		perf.EffectiveTotalTokensPerSecond = &v
	}
	return perf
}

func mustJSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(data)
}
