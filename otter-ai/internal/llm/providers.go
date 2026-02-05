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

// OpenAIProvider implements the OpenAI LLM provider
type OpenAIProvider struct {
	endpoint string
	model    string
	apiKey   string
	client   *http.Client
}

// NewOpenAIProvider creates a new OpenAI provider
func NewOpenAIProvider(cfg config.LLMConfig) (*OpenAIProvider, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("OpenAI API key is required")
	}

	return &OpenAIProvider{
		endpoint: cfg.Endpoint,
		model:    cfg.Model,
		apiKey:   cfg.APIKey,
		client:   &http.Client{},
	}, nil
}

// Complete generates a completion using OpenAI's chat completions API
func (p *OpenAIProvider) Complete(ctx context.Context, request *CompletionRequest) (*CompletionResponse, error) {
	messages := []map[string]string{}

	if request.SystemPrompt != "" {
		messages = append(messages, map[string]string{
			"role":    "system",
			"content": request.SystemPrompt,
		})
	}

	messages = append(messages, map[string]string{
		"role":    "user",
		"content": request.Prompt,
	})

	reqBody := map[string]interface{}{
		"model":    p.model,
		"messages": messages,
	}

	if request.MaxTokens > 0 {
		// Use max_completion_tokens for newer models (GPT-4 and later)
		reqBody["max_completion_tokens"] = request.MaxTokens
	}

	// Only set temperature if it's explicitly different from 1.0
	// Some models (like o1) don't support custom temperature
	if request.Temperature > 0 && request.Temperature != 1.0 {
		reqBody["temperature"] = request.Temperature
	}

	if len(request.StopTokens) > 0 {
		reqBody["stop"] = request.StopTokens
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.endpoint+"/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

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
		return nil, fmt.Errorf("OpenAI API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			TotalTokens int `json:"total_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("no choices returned from OpenAI")
	}

	return &CompletionResponse{
		Text:         result.Choices[0].Message.Content,
		TokensUsed:   result.Usage.TotalTokens,
		FinishReason: result.Choices[0].FinishReason,
	}, nil
}

// Embed generates embeddings using OpenAI's embeddings API
func (p *OpenAIProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	// Use text-embedding-3-small as default embedding model
	embeddingModel := "text-embedding-3-small"

	reqBody := map[string]interface{}{
		"input": text,
		"model": embeddingModel,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.endpoint+"/embeddings", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

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
		return nil, fmt.Errorf("OpenAI API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(result.Data) == 0 {
		return nil, fmt.Errorf("no embeddings returned from OpenAI")
	}

	return result.Data[0].Embedding, nil
}

// Name returns the provider name
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
