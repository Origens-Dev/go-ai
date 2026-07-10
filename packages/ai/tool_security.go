package ai

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"sync/atomic"
)

// CanonicalJSON returns deterministic JSON. encoding/json sorts string map keys,
// including nested maps, which gives the tool-security APIs a stable payload.
func CanonicalJSON(value any) (string, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return "", err
	}
	data := buffer.Bytes()
	if len(data) > 0 && data[len(data)-1] == '\n' {
		data = data[:len(data)-1]
	}
	return string(data), nil
}

func HashCanonical(value any) (string, error) {
	canonical, err := CanonicalJSON(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(canonical))
	return base64.RawURLEncoding.EncodeToString(digest[:]), nil
}

type ToolDrift struct {
	Added   []string
	Removed []string
	Changed []string
}

func FingerprintTools(tools map[string]Tool) (map[string]string, error) {
	result := make(map[string]string, len(tools))
	for name, tool := range tools {
		digest, err := HashCanonical(map[string]any{
			"description": map[string]any{"type": "string", "value": tool.Description},
			"inputSchema": normalizeSchema(tool.InputSchema),
			"title":       tool.Title,
		})
		if err != nil {
			return nil, err
		}
		result[name] = digest
	}
	return result, nil
}

func DetectToolDrift(current, baseline map[string]string) ToolDrift {
	var drift ToolDrift
	for name, digest := range current {
		previous, ok := baseline[name]
		if !ok {
			drift.Added = append(drift.Added, name)
		} else if previous != digest {
			drift.Changed = append(drift.Changed, name)
		}
	}
	for name := range baseline {
		if _, ok := current[name]; !ok {
			drift.Removed = append(drift.Removed, name)
		}
	}
	sort.Strings(drift.Added)
	sort.Strings(drift.Removed)
	sort.Strings(drift.Changed)
	return drift
}

func SignToolApproval(secret []byte, approvalID, toolCallID, toolName string, input any) (string, error) {
	inputDigest, err := HashCanonical(input)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = fmt.Fprintf(mac, "%s\n%s\n%s\n%s", approvalID, toolCallID, toolName, inputDigest)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func VerifyToolApprovalSignature(secret []byte, signature, approvalID, toolCallID, toolName string, input any) (bool, error) {
	expected, err := SignToolApproval(secret, approvalID, toolCallID, toolName, input)
	if err != nil {
		return false, err
	}
	actual, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil {
		return false, nil
	}
	want, _ := base64.RawURLEncoding.DecodeString(expected)
	return hmac.Equal(actual, want), nil
}

var approvalIDFallback atomic.Uint64

func nextApprovalID() string {
	data := make([]byte, 12)
	if _, err := rand.Read(data); err == nil {
		return "approval-" + base64.RawURLEncoding.EncodeToString(data)
	}
	return fmt.Sprintf("approval-%d", approvalIDFallback.Add(1))
}
