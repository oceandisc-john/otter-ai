package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"otter-ai/internal/governance"
	"otter-ai/internal/llm"
	"otter-ai/internal/memory"
)

// ToolHandler executes a tool call and returns the result text.
type ToolHandler func(ctx context.Context, args map[string]string) (string, error)

// agentTools returns the tool definitions exposed to the LLM, filtered to
// what the agent actually supports (e.g. governance tools are omitted when
// no governance system is configured).
func (a *Agent) agentTools() []llm.ToolDefinition {
	tools := []llm.ToolDefinition{
		{
			Name:        "search_memories",
			Description: "Search the agent's stored memories by a natural-language query. Use when the user asks you to recall past interactions or look something up in memory.",
			Parameters: []llm.ToolParameter{
				{Name: "query", Type: "string", Description: "The search query", Required: true},
			},
		},
		{
			Name:        "get_last_memory",
			Description: "Retrieve the most recently stored memory record.",
			Parameters:  []llm.ToolParameter{},
		},
		{
			Name:        "compare_memories",
			Description: "Compare and contrast recent memory entries, noting patterns, shifts, or changes over time.",
			Parameters:  []llm.ToolParameter{},
		},
		{
			Name:        "get_health_status",
			Description: "Report the agent's current container and runtime health metrics including uptime, goroutines, memory usage, and CPU.",
			Parameters:  []llm.ToolParameter{},
		},
	}

	if a.governance != nil {
		tools = append(tools,
			llm.ToolDefinition{
				Name:        "list_governance_state",
				Description: "List the current governance state: active rules, open proposals, and their vote tallies.",
				Parameters:  []llm.ToolParameter{},
			},
			llm.ToolDefinition{
				Name:        "propose_rule",
				Description: "Propose a new governance rule for the raft to vote on.",
				Parameters: []llm.ToolParameter{
					{Name: "rule_body", Type: "string", Description: "The text of the rule to propose", Required: true},
					{Name: "scope", Type: "string", Description: "The scope of the rule (default: general)", Required: false},
				},
			},
			llm.ToolDefinition{
				Name:        "vote_on_proposal",
				Description: "Cast a vote on an open governance proposal.",
				Parameters: []llm.ToolParameter{
					{Name: "proposal_id", Type: "string", Description: "The ID of the proposal to vote on", Required: true},
					{Name: "vote", Type: "string", Description: "The vote to cast", Required: true, Enum: []string{"yes", "no", "abstain"}},
				},
			},
		)
	}

	return tools
}

// toolHandlers returns a mapping of tool name → execution function.
func (a *Agent) toolHandlers() map[string]ToolHandler {
	handlers := map[string]ToolHandler{
		"search_memories":       a.toolSearchMemories,
		"get_last_memory":       a.toolGetLastMemory,
		"compare_memories":      a.toolCompareMemories,
		"get_health_status":     a.toolGetHealthStatus,
		"list_governance_state": a.toolListGovernanceState,
		"propose_rule":          a.toolProposeRule,
		"vote_on_proposal":      a.toolVoteOnProposal,
	}
	return handlers
}

// executeTool dispatches a single tool call and returns the result.
func (a *Agent) executeTool(ctx context.Context, call llm.ToolCall) string {
	handlers := a.toolHandlers()
	handler, ok := handlers[call.Name]
	if !ok {
		return fmt.Sprintf("Unknown tool: %s", call.Name)
	}
	result, err := handler(ctx, call.Arguments)
	if err != nil {
		return fmt.Sprintf("Tool %s error: %v", call.Name, err)
	}
	return result
}

// --- Tool implementations ---

func (a *Agent) toolSearchMemories(ctx context.Context, args map[string]string) (string, error) {
	query := args["query"]
	if query == "" {
		return "No search query provided.", nil
	}

	embedding, err := a.llm.Embed(ctx, query)
	if err != nil {
		return "", fmt.Errorf("failed to generate embedding: %w", err)
	}

	memories, err := a.memory.Search(ctx, embedding, memory.MemoryTypeLongTerm, DefaultMemorySearchLimit)
	if err != nil {
		return "", fmt.Errorf("failed to search memories: %w", err)
	}

	if len(memories) == 0 {
		return "No relevant memories found.", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d relevant memories:\n", len(memories)))
	for i, mem := range memories {
		if i >= 5 {
			break
		}
		content := strings.TrimSpace(mem.Content)
		if len(content) > MaxMemoryPreviewLength {
			content = content[:MaxMemoryPreviewLength] + "..."
		}
		sb.WriteString(fmt.Sprintf("%d. [%s] %s\n", i+1, mem.Timestamp.Format(time.RFC3339), content))
	}
	return sb.String(), nil
}

func (a *Agent) toolGetLastMemory(_ context.Context, _ map[string]string) (string, error) {
	ctx := context.Background()
	records, err := a.memory.List(ctx, memory.MemoryTypeLongTerm, 1, 0)
	if err != nil {
		return "", fmt.Errorf("failed to read memory: %w", err)
	}
	if len(records) == 0 {
		return "No stored memories yet.", nil
	}

	last := strings.TrimSpace(records[0].Content)
	if len(last) > MaxMemoryPreviewLength {
		last = last[:MaxMemoryPreviewLength] + "..."
	}
	ts := ""
	if !records[0].Timestamp.IsZero() {
		ts = records[0].Timestamp.Format(time.RFC3339) + " — "
	}
	return fmt.Sprintf("Last memory: %s%s", ts, last), nil
}

func (a *Agent) toolCompareMemories(_ context.Context, _ map[string]string) (string, error) {
	ctx := context.Background()
	return a.getMemoryComparisonResponse(ctx), nil
}

func (a *Agent) toolGetHealthStatus(_ context.Context, _ map[string]string) (string, error) {
	snapshot := a.captureContainerHealthSnapshot()

	jsonBytes, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal health: %w", err)
	}
	return fmt.Sprintf("Current health metrics:\n%s", string(jsonBytes)), nil
}

func (a *Agent) toolListGovernanceState(_ context.Context, _ map[string]string) (string, error) {
	if a.governance == nil {
		return "Governance system is not configured.", nil
	}
	return a.buildGovernanceContext(), nil
}

func (a *Agent) toolProposeRule(ctx context.Context, args map[string]string) (string, error) {
	if a.governance == nil {
		return "Governance system is not configured.", nil
	}

	ruleBody := strings.TrimSpace(args["rule_body"])
	if ruleBody == "" {
		return "No rule body provided.", nil
	}
	if len(ruleBody) > MaxRuleBodyLength {
		return fmt.Sprintf("Rule body too long (max %d characters).", MaxRuleBodyLength), nil
	}

	scope := strings.TrimSpace(args["scope"])
	if scope == "" {
		scope = "general"
	}

	otterID := a.governance.GetID()
	rule := &governance.Rule{
		Scope:      scope,
		Body:       ruleBody,
		ProposedBy: otterID,
		Timestamp:  time.Now(),
	}

	raftID := a.governance.GetID()
	proposal, err := a.governance.ProposeRule(ctx, raftID, rule)
	if err != nil {
		if strings.Contains(err.Error(), "proposer must be an active member") {
			return fmt.Sprintf("Rule drafted: \"%s\"\n\nNote: Cannot formally propose — no active raft membership. Initialize the raft membership system first.", ruleBody), nil
		}
		return "", fmt.Errorf("failed to propose rule: %w", err)
	}

	return fmt.Sprintf("Rule proposal submitted.\n\nProposal ID: %s\nRule: \"%s\"\nScope: %s\nStatus: Open for voting", proposal.ProposalID, ruleBody, scope), nil
}

func (a *Agent) toolVoteOnProposal(ctx context.Context, args map[string]string) (string, error) {
	if a.governance == nil {
		return "Governance system is not configured.", nil
	}

	proposalID := strings.TrimSpace(args["proposal_id"])
	if proposalID == "" {
		return "No proposal ID provided.", nil
	}

	voteStr := strings.ToLower(strings.TrimSpace(args["vote"]))
	var voteType governance.VoteType
	switch voteStr {
	case "yes":
		voteType = governance.VoteYes
	case "no":
		voteType = governance.VoteNo
	case "abstain":
		voteType = governance.VoteAbstain
	default:
		return fmt.Sprintf("Invalid vote type: %s (must be yes/no/abstain).", voteStr), nil
	}

	voterID := a.governance.GetID()
	if err := a.governance.Vote(ctx, proposalID, voterID, voteType); err != nil {
		if strings.Contains(err.Error(), "voter must be an active member") {
			return "Cannot vote: not an active raft member.", nil
		}
		return "", fmt.Errorf("failed to vote: %w", err)
	}

	return fmt.Sprintf("Voted %s on proposal %s.", strings.ToUpper(voteStr), proposalID), nil
}
