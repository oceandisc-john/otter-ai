package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"otter-ai/internal/config"
)

// OllamaProvider implements the Ollama LLM provider
type OllamaProvider struct {
	endpoint string
	model    string
	client   *http.Client
}

// NewOllamaProvider creates a new Ollama provider
func NewOllamaProvider(cfg config.LLMConfig) (*OllamaProvider, error) {
	return &OllamaProvider{
		endpoint: cfg.Endpoint,
		model:    cfg.Model,
		client:   &http.Client{},
	}, nil
}

// Complete generates a completion
func (p *OllamaProvider) Complete(ctx context.Context, request *CompletionRequest) (*CompletionResponse, error) {
	prompt := request.Prompt
	if request.SystemPrompt != "" {
		prompt = request.SystemPrompt + "\n\n" + prompt
	}

	reqBody := map[string]interface{}{
		"model":  p.model,
		"prompt": prompt,
		"stream": false,
	}

	if request.MaxTokens > 0 {
		reqBody["options"] = map[string]interface{}{
			"num_predict": request.MaxTokens,
		}
	}

	if request.Temperature > 0 {
		if options, ok := reqBody["options"].(map[string]interface{}); ok {
			options["temperature"] = request.Temperature
		} else {
			reqBody["options"] = map[string]interface{}{
				"temperature": request.Temperature,
			}
		}
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.endpoint+"/api/generate", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: %s", string(body))
	}

	var result struct {
		Response string `json:"response"`
		Done     bool   `json:"done"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &CompletionResponse{
		Text:         result.Response,
		FinishReason: "stop",
	}, nil
}

// Embed generates embeddings
func (p *OllamaProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	reqBody := map[string]interface{}{
		"model":  p.model,
		"prompt": text,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.endpoint+"/api/embeddings", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: %s", string(body))
	}

	var result struct {
		Embedding []float32 `json:"embedding"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return result.Embedding, nil
}

// Name returns the provider name
func (p *OllamaProvider) Name() string {
	return "ollama"
}

// NewOpenWebUIProvider creates a new OpenWebUI provider (same as Ollama)
func NewOpenWebUIProvider(cfg config.LLMConfig) (*OllamaProvider, error) {
	return NewOllamaProvider(cfg)
}

// Stubs for other providers

// OpenAIProvider stub
type OpenAIProvider struct{}

func NewOpenAIProvider(cfg config.LLMConfig) (*OpenAIProvider, error) {
	return nil, fmt.Errorf("OpenAI provider not yet implemented")
}

func (p *OpenAIProvider) Complete(ctx context.Context, request *CompletionRequest) (*CompletionResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (p *OpenAIProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	return nil, fmt.Errorf("not implemented")
}

func (p *OpenAIProvider) Name() string {
	return "openai"
}

// AnthropicProvider stub
type AnthropicProvider struct{}

func NewAnthropicProvider(cfg config.LLMConfig) (*AnthropicProvider, error) {
	return nil, fmt.Errorf("Anthropic provider not yet implemented")
}

func (p *AnthropicProvider) Complete(ctx context.Context, request *CompletionRequest) (*CompletionResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (p *AnthropicProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	return nil, fmt.Errorf("not implemented")
}

func (p *AnthropicProvider) Name() string {
	return "anthropic"
}
