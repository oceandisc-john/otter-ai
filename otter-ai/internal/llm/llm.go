package llm

import (
	"context"
	"encoding/json"
	"fmt"

	"otter-ai/internal/config"
)

// Provider is the interface for LLM providers
type Provider interface {
	// Complete generates a completion for the given prompt
	Complete(ctx context.Context, request *CompletionRequest) (*CompletionResponse, error)

	// Embed generates embeddings for the given text
	Embed(ctx context.Context, text string) ([]float32, error)

	// Name returns the provider name
	Name() string
}

// ToolParameter describes a single parameter for a tool.
type ToolParameter struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"` // "string", "number", "boolean", "integer"
	Description string   `json:"description"`
	Required    bool     `json:"required"`
	Enum        []string `json:"enum,omitempty"`
}

// ToolDefinition describes a tool the LLM may invoke.
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  []ToolParameter `json:"parameters"`
}

// ToolCall represents the LLM's request to invoke a tool.
type ToolCall struct {
	Name      string            `json:"name"`
	Arguments map[string]string `json:"arguments"`
}

// CompletionRequest represents a completion request
type CompletionRequest struct {
	Prompt       string
	MaxTokens    int
	Temperature  float32
	StopTokens   []string
	SystemPrompt string
	Tools        []ToolDefinition // available tools (optional)
}

// CompletionResponse represents a completion response
type CompletionResponse struct {
	Text         string
	TokensUsed   int
	FinishReason string
	ToolCalls    []ToolCall // tools the model wants to invoke (may be empty)
}

// ProviderType defines supported LLM providers
type ProviderType string

const (
	ProviderOpenWebUI ProviderType = "openwebui"
	ProviderOpenAI    ProviderType = "openai"
	ProviderAnthropic ProviderType = "anthropic"
	ProviderOllama    ProviderType = "ollama"
)

// NewProvider creates a new LLM provider based on configuration
func NewProvider(cfg config.LLMConfig) (Provider, error) {
	switch ProviderType(cfg.Provider) {
	case ProviderOpenWebUI:
		return NewOpenWebUIProvider(cfg)
	case ProviderOpenAI:
		return NewOpenAIProvider(cfg)
	case ProviderAnthropic:
		return NewAnthropicProvider(cfg)
	case ProviderOllama:
		return NewOllamaProvider(cfg)
	default:
		return nil, fmt.Errorf("unsupported LLM provider: %s", cfg.Provider)
	}
}

// buildOpenAITools converts ToolDefinitions to the OpenAI function-calling
// schema used by OpenAI, OpenWebUI and Ollama compatible APIs.
func buildOpenAITools(tools []ToolDefinition) []map[string]interface{} {
	if len(tools) == 0 {
		return nil
	}
	out := make([]map[string]interface{}, len(tools))
	for i, td := range tools {
		properties := map[string]interface{}{}
		required := []string{}
		for _, p := range td.Parameters {
			prop := map[string]interface{}{
				"type":        p.Type,
				"description": p.Description,
			}
			if len(p.Enum) > 0 {
				prop["enum"] = p.Enum
			}
			properties[p.Name] = prop
			if p.Required {
				required = append(required, p.Name)
			}
		}
		out[i] = map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name":        td.Name,
				"description": td.Description,
				"parameters": map[string]interface{}{
					"type":       "object",
					"properties": properties,
					"required":   required,
				},
			},
		}
	}
	return out
}

// parseOpenAIToolCalls extracts ToolCalls from the OpenAI-compatible
// response choice structure.
type openAIToolCallJSON struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"` // JSON-encoded string
	} `json:"function"`
}

func parseOpenAIToolCalls(raw []openAIToolCallJSON) []ToolCall {
	if len(raw) == 0 {
		return nil
	}
	calls := make([]ToolCall, 0, len(raw))
	for _, r := range raw {
		args := map[string]string{}
		// Arguments comes as a JSON string; parse into flat string map.
		var parsed map[string]interface{}
		if json.Unmarshal([]byte(r.Function.Arguments), &parsed) == nil {
			for k, v := range parsed {
				args[k] = fmt.Sprintf("%v", v)
			}
		}
		calls = append(calls, ToolCall{
			Name:      r.Function.Name,
			Arguments: args,
		})
	}
	return calls
}
