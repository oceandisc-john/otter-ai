package llm

import (
	"context"
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

// CompletionRequest represents a completion request
type CompletionRequest struct {
	Prompt       string
	MaxTokens    int
	Temperature  float32
	StopTokens   []string
	SystemPrompt string
}

// CompletionResponse represents a completion response
type CompletionResponse struct {
	Text         string
	TokensUsed   int
	FinishReason string
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
