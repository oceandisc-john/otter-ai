package agent

import (
	"context"
	"fmt"

	"otter-ai/internal/governance"
	"otter-ai/internal/llm"
	"otter-ai/internal/memory"
	"otter-ai/internal/plugins"
)

// Agent represents the Otter-AI agent
type Agent struct {
	memory     *memory.Memory
	governance *governance.Governance
	llm        llm.Provider
	plugins    *plugins.Manager
}

// Config holds agent configuration
type Config struct {
	Memory     *memory.Memory
	Governance *governance.Governance
	LLM        llm.Provider
	Plugins    *plugins.Manager
}

// New creates a new agent
func New(cfg Config) *Agent {
	return &Agent{
		memory:     cfg.Memory,
		governance: cfg.Governance,
		llm:        cfg.LLM,
		plugins:    cfg.Plugins,
	}
}

// ProcessMessage processes an incoming message
func (a *Agent) ProcessMessage(ctx context.Context, message string) (string, error) {
	// Generate embedding for the message
	embedding, err := a.llm.Embed(ctx, message)
	if err != nil {
		return "", fmt.Errorf("failed to generate embedding: %w", err)
	}

	// Search for relevant memories
	memories, err := a.memory.Search(ctx, embedding, memory.MemoryTypeLongTerm, 5)
	if err != nil {
		return "", fmt.Errorf("failed to search memories: %w", err)
	}

	// Build context from memories
	context_str := ""
	for _, mem := range memories {
		context_str += mem.Content + "\n"
	}

	// Get active governance rules
	rules := a.governance.GetActiveRules()
	rulesContext := ""
	for _, rule := range rules {
		rulesContext += fmt.Sprintf("Rule [%s]: %s\n", rule.Scope, rule.Body)
	}

	// Build prompt
	systemPrompt := fmt.Sprintf(`You are Otter-AI, a governed AI agent operating under the following active rules:

%s

Relevant context from memory:
%s

Respond thoughtfully and in accordance with the governance rules.`, rulesContext, context_str)

	// Generate response
	response, err := a.llm.Complete(ctx, &llm.CompletionRequest{
		SystemPrompt: systemPrompt,
		Prompt:       message,
		MaxTokens:    500,
		Temperature:  0.7,
	})
	if err != nil {
		return "", fmt.Errorf("failed to generate response: %w", err)
	}

	// Store the interaction as memory
	interactionMemory := &memory.MemoryRecord{
		Type:       memory.MemoryTypeLongTerm,
		Content:    fmt.Sprintf("User: %s\nOtter: %s", message, response.Text),
		Embedding:  embedding,
		Importance: 0.5,
		Metadata: map[string]interface{}{
			"user_message": message,
			"response":     response.Text,
		},
	}

	if err := a.memory.Store(ctx, interactionMemory); err != nil {
		// Log but don't fail the response
		fmt.Printf("Warning: failed to store memory: %v\n", err)
	}

	return response.Text, nil
}

// GetMemory returns the memory layer
func (a *Agent) GetMemory() *memory.Memory {
	return a.memory
}

// GetGovernance returns the governance system
func (a *Agent) GetGovernance() *governance.Governance {
	return a.governance
}

// GetPlugins returns the plugin manager
func (a *Agent) GetPlugins() *plugins.Manager {
	return a.plugins
}
