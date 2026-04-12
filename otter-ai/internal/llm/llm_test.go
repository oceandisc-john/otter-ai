package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"otter-ai/internal/config"
)

// --- NewProvider Factory ---

func TestNewProvider_Ollama(t *testing.T) {
	p, err := NewProvider(config.LLMConfig{Provider: "ollama", Endpoint: "http://localhost:11434", Model: "llama2"})
	if err != nil {
		t.Fatalf("NewProvider ollama: %v", err)
	}
	if p.Name() != "ollama" {
		t.Errorf("Name() = %q", p.Name())
	}
}

func TestNewProvider_OpenWebUI(t *testing.T) {
	p, err := NewProvider(config.LLMConfig{Provider: "openwebui", Endpoint: "http://localhost:8080", Model: "m"})
	if err != nil {
		t.Fatalf("NewProvider openwebui: %v", err)
	}
	if p.Name() != "openwebui" {
		t.Errorf("Name() = %q", p.Name())
	}
}

func TestNewProvider_OpenAI(t *testing.T) {
	p, err := NewProvider(config.LLMConfig{Provider: "openai", Endpoint: "http://localhost", Model: "gpt-4", APIKey: "sk-test"})
	if err != nil {
		t.Fatalf("NewProvider openai: %v", err)
	}
	if p.Name() != "openai" {
		t.Errorf("Name() = %q", p.Name())
	}
}

func TestNewProvider_OpenAI_MissingKey(t *testing.T) {
	_, err := NewProvider(config.LLMConfig{Provider: "openai", Endpoint: "http://localhost", Model: "gpt-4"})
	if err == nil {
		t.Error("expected error for missing API key")
	}
}

func TestNewProvider_Anthropic(t *testing.T) {
	_, err := NewProvider(config.LLMConfig{Provider: "anthropic"})
	if err == nil {
		t.Error("expected error for anthropic stub")
	}
}

func TestNewProvider_Unknown(t *testing.T) {
	_, err := NewProvider(config.LLMConfig{Provider: "unknown"})
	if err == nil {
		t.Error("expected error for unknown provider")
	}
}

// --- Ollama Provider ---

func TestOllama_Complete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"response": "Hello from Ollama",
			"done":     true,
		})
	}))
	defer srv.Close()

	p, _ := NewOllamaProvider(config.LLMConfig{Endpoint: srv.URL, Model: "test"})
	resp, err := p.Complete(context.Background(), &CompletionRequest{Prompt: "hi"})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Text != "Hello from Ollama" {
		t.Errorf("Text = %q", resp.Text)
	}
}

func TestOllama_Complete_WithSystemPrompt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		prompt := req["prompt"].(string)
		if prompt == "" {
			t.Error("expected non-empty prompt with system prompt prepended")
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"response": "ok", "done": true})
	}))
	defer srv.Close()

	p, _ := NewOllamaProvider(config.LLMConfig{Endpoint: srv.URL, Model: "test"})
	_, err := p.Complete(context.Background(), &CompletionRequest{
		Prompt:       "hello",
		SystemPrompt: "Be helpful",
	})
	if err != nil {
		t.Fatalf("Complete with system prompt: %v", err)
	}
}

func TestOllama_Complete_WithOptions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		opts, ok := req["options"].(map[string]interface{})
		if !ok {
			t.Error("expected options field")
		} else {
			if _, ok := opts["num_predict"]; !ok {
				t.Error("expected num_predict in options")
			}
			if _, ok := opts["temperature"]; !ok {
				t.Error("expected temperature in options")
			}
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"response": "ok", "done": true})
	}))
	defer srv.Close()

	p, _ := NewOllamaProvider(config.LLMConfig{Endpoint: srv.URL, Model: "test"})
	_, err := p.Complete(context.Background(), &CompletionRequest{
		Prompt:      "hi",
		MaxTokens:   100,
		Temperature: 0.5,
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
}

func TestOllama_Complete_TemperatureOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		opts, ok := req["options"].(map[string]interface{})
		if !ok {
			t.Error("expected options field for temperature")
		} else if _, ok := opts["temperature"]; !ok {
			t.Error("expected temperature in options")
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"response": "ok", "done": true})
	}))
	defer srv.Close()

	p, _ := NewOllamaProvider(config.LLMConfig{Endpoint: srv.URL, Model: "test"})
	_, err := p.Complete(context.Background(), &CompletionRequest{
		Prompt:      "hi",
		Temperature: 0.7,
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
}

func TestOllama_Complete_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error"))
	}))
	defer srv.Close()

	p, _ := NewOllamaProvider(config.LLMConfig{Endpoint: srv.URL, Model: "test"})
	_, err := p.Complete(context.Background(), &CompletionRequest{Prompt: "hi"})
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestOllama_Embed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embeddings" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"embedding": []float32{0.1, 0.2, 0.3},
		})
	}))
	defer srv.Close()

	p, _ := NewOllamaProvider(config.LLMConfig{Endpoint: srv.URL, Model: "test"})
	embed, err := p.Embed(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(embed) != 3 {
		t.Errorf("embedding len = %d; want 3", len(embed))
	}
}

func TestOllama_Embed_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("bad"))
	}))
	defer srv.Close()

	p, _ := NewOllamaProvider(config.LLMConfig{Endpoint: srv.URL, Model: "test"})
	_, err := p.Embed(context.Background(), "hello")
	if err == nil {
		t.Error("expected error")
	}
}

// --- OpenWebUI Provider ---

func TestOpenWebUI_Complete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("expected auth header")
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]string{"content": "Hi from OpenWebUI"}, "finish_reason": "stop"},
			},
			"usage": map[string]int{"total_tokens": 10},
		})
	}))
	defer srv.Close()

	p, _ := NewOpenWebUIProvider(config.LLMConfig{Endpoint: srv.URL, Model: "m", APIKey: "test-key"})
	resp, err := p.Complete(context.Background(), &CompletionRequest{Prompt: "hi", SystemPrompt: "sys"})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Text != "Hi from OpenWebUI" {
		t.Errorf("Text = %q", resp.Text)
	}
	if resp.TokensUsed != 10 {
		t.Errorf("TokensUsed = %d", resp.TokensUsed)
	}
}

func TestOpenWebUI_Complete_NoChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"choices": []interface{}{}})
	}))
	defer srv.Close()

	p, _ := NewOpenWebUIProvider(config.LLMConfig{Endpoint: srv.URL, Model: "m"})
	_, err := p.Complete(context.Background(), &CompletionRequest{Prompt: "hi"})
	if err == nil {
		t.Error("expected error for no choices")
	}
}

func TestOpenWebUI_Complete_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	p, _ := NewOpenWebUIProvider(config.LLMConfig{Endpoint: srv.URL, Model: "m"})
	_, err := p.Complete(context.Background(), &CompletionRequest{Prompt: "hi"})
	if err == nil {
		t.Error("expected error")
	}
}

func TestOpenWebUI_Embed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"embedding": []float32{0.5, 0.6}, "index": 0},
			},
		})
	}))
	defer srv.Close()

	p, _ := NewOpenWebUIProvider(config.LLMConfig{Endpoint: srv.URL, Model: "m"})
	embed, err := p.Embed(context.Background(), "test")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(embed) != 2 {
		t.Errorf("len = %d", len(embed))
	}
}

func TestOpenWebUI_Embed_NoData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"data": []interface{}{}})
	}))
	defer srv.Close()

	p, _ := NewOpenWebUIProvider(config.LLMConfig{Endpoint: srv.URL, Model: "m"})
	_, err := p.Embed(context.Background(), "test")
	if err == nil {
		t.Error("expected error for no embeddings")
	}
}

func TestOpenWebUI_Embed_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	p, _ := NewOpenWebUIProvider(config.LLMConfig{Endpoint: srv.URL, Model: "m"})
	_, err := p.Embed(context.Background(), "test")
	if err == nil {
		t.Error("expected error")
	}
}

// --- OpenAI Provider ---

func TestOpenAI_Complete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			t.Error("expected auth header")
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]string{"content": "OpenAI response"}, "finish_reason": "stop"},
			},
			"usage": map[string]int{"total_tokens": 20},
		})
	}))
	defer srv.Close()

	p, _ := NewOpenAIProvider(config.LLMConfig{Endpoint: srv.URL, Model: "gpt-4", APIKey: "sk-test"})
	resp, err := p.Complete(context.Background(), &CompletionRequest{
		Prompt:       "hi",
		SystemPrompt: "sys",
		MaxTokens:    50,
		Temperature:  0.7,
		StopTokens:   []string{"###"},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Text != "OpenAI response" {
		t.Errorf("Text = %q", resp.Text)
	}
}

func TestOpenAI_Complete_Temperature1(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		if _, ok := req["temperature"]; ok {
			t.Error("temperature=1.0 should not be sent for OpenAI")
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]string{"content": "ok"}, "finish_reason": "stop"},
			},
		})
	}))
	defer srv.Close()

	p, _ := NewOpenAIProvider(config.LLMConfig{Endpoint: srv.URL, Model: "gpt-4", APIKey: "sk-test"})
	_, err := p.Complete(context.Background(), &CompletionRequest{Prompt: "hi", Temperature: 1.0})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
}

func TestOpenAI_Complete_NoChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"choices": []interface{}{}})
	}))
	defer srv.Close()

	p, _ := NewOpenAIProvider(config.LLMConfig{Endpoint: srv.URL, Model: "gpt-4", APIKey: "sk-test"})
	_, err := p.Complete(context.Background(), &CompletionRequest{Prompt: "hi"})
	if err == nil {
		t.Error("expected error")
	}
}

func TestOpenAI_Complete_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	p, _ := NewOpenAIProvider(config.LLMConfig{Endpoint: srv.URL, Model: "gpt-4", APIKey: "sk-test"})
	_, err := p.Complete(context.Background(), &CompletionRequest{Prompt: "hi"})
	if err == nil {
		t.Error("expected error")
	}
}

func TestOpenAI_Embed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			t.Errorf("path = %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"embedding": []float32{0.1, 0.2}},
			},
		})
	}))
	defer srv.Close()

	p, _ := NewOpenAIProvider(config.LLMConfig{Endpoint: srv.URL, Model: "gpt-4", APIKey: "sk-test"})
	embed, err := p.Embed(context.Background(), "test")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(embed) != 2 {
		t.Errorf("len = %d", len(embed))
	}
}

func TestOpenAI_Embed_NoData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"data": []interface{}{}})
	}))
	defer srv.Close()

	p, _ := NewOpenAIProvider(config.LLMConfig{Endpoint: srv.URL, Model: "gpt-4", APIKey: "sk-test"})
	_, err := p.Embed(context.Background(), "test")
	if err == nil {
		t.Error("expected error")
	}
}

func TestOpenAI_Embed_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	p, _ := NewOpenAIProvider(config.LLMConfig{Endpoint: srv.URL, Model: "gpt-4", APIKey: "sk-test"})
	_, err := p.Embed(context.Background(), "test")
	if err == nil {
		t.Error("expected error")
	}
}

// --- Provider Names ---

// --- buildOpenAITools ---

func TestBuildOpenAITools_Empty(t *testing.T) {
	result := buildOpenAITools(nil)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
	result = buildOpenAITools([]ToolDefinition{})
	if result != nil {
		t.Errorf("expected nil for empty slice, got %v", result)
	}
}

func TestBuildOpenAITools_WithTools(t *testing.T) {
	tools := []ToolDefinition{
		{
			Name:        "get_weather",
			Description: "Get current weather",
			Parameters: []ToolParameter{
				{Name: "city", Type: "string", Description: "City name", Required: true},
				{Name: "unit", Type: "string", Description: "Temperature unit", Required: false, Enum: []string{"celsius", "fahrenheit"}},
			},
		},
	}
	result := buildOpenAITools(tools)
	if len(result) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result))
	}

	// Verify structure
	tool := result[0]
	if tool["type"] != "function" {
		t.Errorf("expected type=function, got %v", tool["type"])
	}
	fn, ok := tool["function"].(map[string]interface{})
	if !ok {
		t.Fatal("expected function to be a map")
	}
	if fn["name"] != "get_weather" {
		t.Errorf("expected name=get_weather, got %v", fn["name"])
	}

	params, ok := fn["parameters"].(map[string]interface{})
	if !ok {
		t.Fatal("expected parameters to be a map")
	}
	required, ok := params["required"].([]string)
	if !ok {
		t.Fatal("expected required to be []string")
	}
	if len(required) != 1 || required[0] != "city" {
		t.Errorf("expected required=[city], got %v", required)
	}
}

// --- parseOpenAIToolCalls ---

func TestParseOpenAIToolCalls_Empty(t *testing.T) {
	result := parseOpenAIToolCalls(nil)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestParseOpenAIToolCalls_WithCalls(t *testing.T) {
	raw := []openAIToolCallJSON{
		{
			ID:   "call_1",
			Type: "function",
			Function: struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}{
				Name:      "get_weather",
				Arguments: `{"city": "London", "unit": "celsius"}`,
			},
		},
	}
	result := parseOpenAIToolCalls(raw)
	if len(result) != 1 {
		t.Fatalf("expected 1 call, got %d", len(result))
	}
	if result[0].Name != "get_weather" {
		t.Errorf("expected name=get_weather, got %s", result[0].Name)
	}
	if result[0].Arguments["city"] != "London" {
		t.Errorf("expected city=London, got %s", result[0].Arguments["city"])
	}
	if result[0].Arguments["unit"] != "celsius" {
		t.Errorf("expected unit=celsius, got %s", result[0].Arguments["unit"])
	}
}

func TestParseOpenAIToolCalls_InvalidJSON(t *testing.T) {
	raw := []openAIToolCallJSON{
		{
			Function: struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}{
				Name:      "test",
				Arguments: "not json",
			},
		},
	}
	result := parseOpenAIToolCalls(raw)
	if len(result) != 1 {
		t.Fatalf("expected 1 call, got %d", len(result))
	}
	// Should have empty arguments when JSON is invalid
	if len(result[0].Arguments) != 0 {
		t.Errorf("expected empty args for invalid JSON, got %v", result[0].Arguments)
	}
}

// --- OpenAI tool call integration ---

func TestOpenAI_Complete_WithToolCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify tools were sent in the request
		var reqBody map[string]interface{}
		json.NewDecoder(r.Body).Decode(&reqBody)
		if _, ok := reqBody["tools"]; !ok {
			t.Error("expected tools in request body")
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"content": "",
						"tool_calls": []map[string]interface{}{
							{
								"id":   "call_abc",
								"type": "function",
								"function": map[string]interface{}{
									"name":      "get_health_status",
									"arguments": "{}",
								},
							},
						},
					},
					"finish_reason": "tool_calls",
				},
			},
			"usage": map[string]interface{}{"total_tokens": 50},
		})
	}))
	defer srv.Close()

	p, _ := NewOpenAIProvider(config.LLMConfig{Endpoint: srv.URL, Model: "gpt-4", APIKey: "sk-test"})
	resp, err := p.Complete(context.Background(), &CompletionRequest{
		Prompt: "how are you?",
		Tools: []ToolDefinition{
			{Name: "get_health_status", Description: "Get health", Parameters: []ToolParameter{}},
		},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "get_health_status" {
		t.Errorf("expected get_health_status, got %s", resp.ToolCalls[0].Name)
	}
}

func TestOpenWebUI_Complete_WithToolCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"content": "",
						"tool_calls": []map[string]interface{}{
							{
								"id":   "call_xyz",
								"type": "function",
								"function": map[string]interface{}{
									"name":      "search_memories",
									"arguments": `{"query": "hello"}`,
								},
							},
						},
					},
					"finish_reason": "tool_calls",
				},
			},
		})
	}))
	defer srv.Close()

	p, _ := NewOpenWebUIProvider(config.LLMConfig{Endpoint: srv.URL, Model: "test"})
	resp, err := p.Complete(context.Background(), &CompletionRequest{
		Prompt: "search for hello",
		Tools: []ToolDefinition{
			{Name: "search_memories", Description: "Search memories", Parameters: []ToolParameter{
				{Name: "query", Type: "string", Description: "Query", Required: true},
			}},
		},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Arguments["query"] != "hello" {
		t.Errorf("expected query=hello, got %s", resp.ToolCalls[0].Arguments["query"])
	}
}

func TestOllama_CompleteWithTools(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// When tools are present, Ollama uses /api/chat not /api/generate
		if r.URL.Path != "/api/chat" {
			t.Errorf("expected /api/chat, got %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": map[string]interface{}{
				"content": "",
				"tool_calls": []map[string]interface{}{
					{
						"function": map[string]interface{}{
							"name":      "get_last_memory",
							"arguments": "{}",
						},
					},
				},
			},
			"done": true,
		})
	}))
	defer srv.Close()

	p, _ := NewOllamaProvider(config.LLMConfig{Endpoint: srv.URL, Model: "test"})
	resp, err := p.Complete(context.Background(), &CompletionRequest{
		Prompt: "what was my last memory?",
		Tools: []ToolDefinition{
			{Name: "get_last_memory", Description: "Get last memory", Parameters: []ToolParameter{}},
		},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "get_last_memory" {
		t.Errorf("expected get_last_memory, got %s", resp.ToolCalls[0].Name)
	}
}

func TestProviderNames(t *testing.T) {
	cases := []struct {
		provider string
		apiKey   string
		want     string
	}{
		{"ollama", "", "ollama"},
		{"openwebui", "", "openwebui"},
		{"openai", "sk-test", "openai"},
	}
	for _, tc := range cases {
		p, err := NewProvider(config.LLMConfig{Provider: tc.provider, Endpoint: "http://localhost", Model: "m", APIKey: tc.apiKey})
		if err != nil {
			t.Errorf("NewProvider(%s): %v", tc.provider, err)
			continue
		}
		if p.Name() != tc.want {
			t.Errorf("Name() = %q; want %q", p.Name(), tc.want)
		}
	}
}

// --- OpenWebUI Complete with StopTokens and MaxTokens ---

func TestOpenWebUI_Complete_WithAllOptions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		if _, ok := req["max_tokens"]; !ok {
			t.Error("expected max_tokens")
		}
		if _, ok := req["temperature"]; !ok {
			t.Error("expected temperature")
		}
		if _, ok := req["stop"]; !ok {
			t.Error("expected stop")
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]string{"content": "ok"}, "finish_reason": "stop"},
			},
		})
	}))
	defer srv.Close()

	p, _ := NewOpenWebUIProvider(config.LLMConfig{Endpoint: srv.URL, Model: "m"})
	_, err := p.Complete(context.Background(), &CompletionRequest{
		Prompt:      "hi",
		MaxTokens:   100,
		Temperature: 0.5,
		StopTokens:  []string{"END"},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
}
