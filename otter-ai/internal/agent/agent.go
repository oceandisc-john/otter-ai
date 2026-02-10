package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"otter-ai/internal/governance"
	"otter-ai/internal/llm"
	"otter-ai/internal/memory"
	"otter-ai/internal/plugins"
)

// Constants for agent configuration
const (
	DefaultMemorySearchLimit = 5
	DefaultMaxTokens         = 300
	DefaultTemperature       = 1.0
	MaxVoteInstructions      = 200
	MaxMessageLength         = 10000
	MaxRuleBodyLength        = 1000
	ConversationHistoryLimit = 10 // Keep last 10 messages in conversation context
)

// ConversationMessage represents a single message in conversation history
type ConversationMessage struct {
	Role      string // "user" or "assistant"
	Content   string
	Timestamp time.Time
}

// ConversationHistory tracks recent messages for context
type ConversationHistory struct {
	mutex    sync.RWMutex
	messages []ConversationMessage
}

// Agent represents the Otter-AI agent
type Agent struct {
	memory       *memory.Memory
	governance   *governance.Governance
	llm          llm.Provider
	plugins      *plugins.Manager
	conversation *ConversationHistory
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
		conversation: &ConversationHistory{
			messages: make([]ConversationMessage, 0, ConversationHistoryLimit),
		},
	}
}

// AddToConversation adds a message to the conversation history
func (ch *ConversationHistory) Add(role, content string) {
	ch.mutex.Lock()
	defer ch.mutex.Unlock()

	ch.messages = append(ch.messages, ConversationMessage{
		Role:      role,
		Content:   content,
		Timestamp: time.Now(),
	})

	// Keep only the last N messages
	if len(ch.messages) > ConversationHistoryLimit {
		ch.messages = ch.messages[len(ch.messages)-ConversationHistoryLimit:]
	}
}

// GetRecent returns recent messages for context
func (ch *ConversationHistory) GetRecent(limit int) []ConversationMessage {
	ch.mutex.RLock()
	defer ch.mutex.RUnlock()

	if len(ch.messages) == 0 {
		return nil
	}

	start := len(ch.messages) - limit
	if start < 0 {
		start = 0
	}
	return append([]ConversationMessage{}, ch.messages[start:]...)
}

// Clear clears the conversation history
func (ch *ConversationHistory) Clear() {
	ch.mutex.Lock()
	defer ch.mutex.Unlock()
	ch.messages = make([]ConversationMessage, 0, ConversationHistoryLimit)
}

// ProcessMessage processes an incoming message
func (a *Agent) ProcessMessage(ctx context.Context, message string) (string, error) {
	// Validate message length
	if len(message) > MaxMessageLength {
		return "", fmt.Errorf("message too long (max %d characters)", MaxMessageLength)
	}

	// Generate embedding for the message
	embedding, err := a.llm.Embed(ctx, message)
	if err != nil {
		return "", fmt.Errorf("failed to generate embedding: %w", err)
	}

	// Check if this requires a special governance action (propose rule or vote)
	if action, handled := a.detectGovernanceAction(ctx, message); handled {
		// Store the interaction as memory
		interactionMemory := &memory.MemoryRecord{
			Type:       memory.MemoryTypeLongTerm,
			Content:    fmt.Sprintf("User: %s\nOtter: %s", message, action),
			Embedding:  embedding,
			Importance: 0.7,
			Metadata: map[string]interface{}{
				"user_message": message,
				"response":     action,
				"type":         "governance_action",
			},
		}
		a.memory.Store(ctx, interactionMemory)
		return action, nil
	}

	// Check if governance context is relevant for this message
	governanceRelevant := a.isGovernanceRelevant(message)

	// Build conversation history context
	recentMessages := a.conversation.GetRecent(6)
	conversationContext := a.buildConversationContext()

	// Search for relevant memories only if conversation history is sparse
	memoryContext := ""
	if len(recentMessages) < 2 { // Only search memories if conversation just started
		memories, err := a.memory.Search(ctx, embedding, memory.MemoryTypeLongTerm, DefaultMemorySearchLimit)
		if err != nil {
			return "", fmt.Errorf("failed to search memories: %w", err)
		}

		fmt.Printf("Retrieved %d memories for query: %s\n", len(memories), message)

		// Build context from relevant memories with similarity threshold
		if len(memories) > 0 {
			memoryContext = "\nRelevant past interactions:\n"
			for i, mem := range memories {
				// Limit to top 3 most relevant memories
				if i >= 3 {
					break
				}
				memoryContext += "- " + mem.Content + "\n"
			}
		}
	}

	// Build governance context only if relevant
	governanceContext := ""
	if governanceRelevant {
		governanceContext = a.buildGovernanceContext() + "\n"
	}

	// Build system prompt with appropriate context
	systemPrompt := fmt.Sprintf(`You are Otter-AI, a helpful AI assistant.

%s%s%s
CRITICAL INSTRUCTIONS:
1. Answer based ONLY on the conversation history above and factual data provided
2. If asked about governance (rules/proposals), report EXACTLY what is shown in the governance context - do NOT elaborate or invent details
3. Stay on topic - if the conversation is about naming, naming preferences, or identity, keep responses focused on that
4. Be direct and concise - answer the specific question asked
5. Do NOT make up information, especially about proposals, rules, or technical details
6. When asked for your preference or opinion based on conversation, review the recent messages and give a direct answer`, conversationContext, governanceContext, memoryContext)

	// Generate response
	response, err := a.llm.Complete(ctx, &llm.CompletionRequest{
		SystemPrompt: systemPrompt,
		Prompt:       message,
		MaxTokens:    DefaultMaxTokens,
		Temperature:  DefaultTemperature,
	})
	if err != nil {
		return "", fmt.Errorf("failed to generate response: %w", err)
	}

	// Add to conversation history
	a.conversation.Add("user", message)
	a.conversation.Add("assistant", response.Text)

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

// ClearConversation clears the conversation history
func (a *Agent) ClearConversation() {
	a.conversation.Clear()
}

// buildConversationContext creates context from recent conversation history
func (a *Agent) buildConversationContext() string {
	recent := a.conversation.GetRecent(6) // Last 3 exchanges (6 messages)
	if len(recent) == 0 {
		return ""
	}

	var context strings.Builder
	context.WriteString("Recent conversation:\n")
	for _, msg := range recent {
		role := "User"
		if msg.Role == "assistant" {
			role = "You"
		}
		context.WriteString(fmt.Sprintf("%s: %s\n", role, msg.Content))
	}
	context.WriteString("\n")

	return context.String()
}

// isGovernanceRelevant checks if the message is related to governance
func (a *Agent) isGovernanceRelevant(message string) bool {
	messageLower := strings.ToLower(message)

	// Check for governance-related keywords
	governanceKeywords := []string{
		"rule", "rules", "governance", "propose", "proposal",
		"vote", "voting", "policy", "policies", "member", "members",
		"raft", "consensus", "ratify", "regulation",
	}

	for _, keyword := range governanceKeywords {
		if strings.Contains(messageLower, keyword) {
			return true
		}
	}

	return false
}

// buildGovernanceContext creates a summary of current governance state
func (a *Agent) buildGovernanceContext() string {
	var context strings.Builder

	// Add active rules
	rules := a.governance.GetActiveRules()
	if len(rules) > 0 {
		context.WriteString("ACTIVE RULES:\n")
		for _, rule := range rules {
			context.WriteString(fmt.Sprintf("  • %s (scope: %s)\n", rule.Body, rule.Scope))
		}
	} else {
		context.WriteString("ACTIVE RULES: None currently in effect.\n")
	}

	// Add open proposals
	proposals := a.governance.GetOpenProposals()
	if len(proposals) > 0 {
		context.WriteString("\nOPEN PROPOSALS (awaiting votes):\n")
		for i, p := range proposals {
			yesVotes := 0
			noVotes := 0
			for _, vote := range p.Votes {
				if vote == governance.VoteYes {
					yesVotes++
				} else if vote == governance.VoteNo {
					noVotes++
				}
			}
			context.WriteString(fmt.Sprintf("  %d. Proposal ID: %s\n", i+1, p.ProposalID[:8]))
			context.WriteString(fmt.Sprintf("     Text: %s\n", p.Rule.Body))
			context.WriteString(fmt.Sprintf("     Scope: %s\n", p.Rule.Scope))
			context.WriteString(fmt.Sprintf("     Proposed by: %s\n", p.Rule.ProposedBy))
			context.WriteString(fmt.Sprintf("     Votes: %d yes, %d no\n", yesVotes, noVotes))
		}
	} else {
		context.WriteString("\nOPEN PROPOSALS: None currently open.\n")
	}

	return context.String()
}

// detectGovernanceAction checks if message requires special action (propose or vote)
func (a *Agent) detectGovernanceAction(ctx context.Context, message string) (string, bool) {
	fmt.Printf("Checking for governance action in: %s\n", message)

	// Quick keyword check first to avoid unnecessary LLM calls
	messageLower := strings.ToLower(message)

	// Check for recent conversation context about rules to catch contextual references
	recentMessages := a.conversation.GetRecent(4)
	conversationMentionsRules := false
	for _, msg := range recentMessages {
		msgLower := strings.ToLower(msg.Content)
		if strings.Contains(msgLower, "rule") || strings.Contains(msgLower, "proposal") {
			conversationMentionsRules = true
			break
		}
	}

	// Check for explicit "propose" command
	// Accept both direct "propose rule" and contextual "propose a new one" if rules were recently discussed
	// Also catch imperative forms like "you should propose this"
	hasPropose := strings.Contains(messageLower, "propose") &&
		(strings.Contains(messageLower, "rule") ||
			strings.Contains(messageLower, "new one") ||
			strings.Contains(messageLower, "this") ||
			strings.Contains(messageLower, "that") ||
			strings.Contains(messageLower, "to the raft") ||
			(conversationMentionsRules && (strings.Contains(messageLower, "it") || strings.Contains(messageLower, "should"))))

	// Check for explicit "vote" command with yes/no/proposal
	hasVote := strings.Contains(messageLower, "vote") &&
		(strings.Contains(messageLower, "yes") || strings.Contains(messageLower, "no") ||
			strings.Contains(messageLower, "proposal") || strings.Contains(messageLower, "on"))

	if !hasPropose && !hasVote {
		fmt.Println("No action keywords detected")
		return "", false
	}

	// Use simple direct prompt
	classificationPrompt := `The user said: "` + message + `"

Is this an ACTION COMMAND or just a QUESTION/DISCUSSION?

ACTION COMMANDS:
- "propose_rule" = User commanding you to submit a rule (contains "propose" + "rule")
- "vote" = User commanding you to vote (contains "vote" + "yes/no/proposal")

QUESTION/DISCUSSION:
- "query" = Just asking or talking, not commanding an action

Reply with ONLY ONE WORD: propose_rule, vote, or query`

	response, err := a.llm.Complete(ctx, &llm.CompletionRequest{
		Prompt:      classificationPrompt,
		MaxTokens:   5,
		Temperature: 0.3, // Lower temperature for more deterministic output
	})

	if err != nil {
		fmt.Printf("Error detecting action: %v\n", err)
		return "", false
	}

	intent := strings.TrimSpace(strings.ToLower(response.Text))
	// Clean up any extra formatting
	intent = strings.Trim(intent, ".!?,;:\"'`-•*")
	intent = strings.TrimSpace(intent)

	fmt.Printf("Detected intent: %s\n", intent)

	// If intent contains the action word, extract it
	if strings.Contains(intent, "propose_rule") || strings.Contains(intent, "propose") {
		fmt.Println("Processing rule proposal action")
		return a.handleRuleProposal(ctx, message), true
	}

	if strings.Contains(intent, "vote") && !strings.Contains(intent, "query") {
		fmt.Println("Processing vote action")
		return a.handleVote(ctx, message), true
	}

	fmt.Println("No action required, will answer naturally")
	return "", false
}

// handleRuleProposal handles submitting a new rule proposal
func (a *Agent) handleRuleProposal(ctx context.Context, message string) string {
	// Get recent conversation for context
	recent := a.conversation.GetRecent(6)
	conversationContext := ""
	for _, msg := range recent {
		role := "User"
		if msg.Role == "assistant" {
			role = "You"
		}
		conversationContext += fmt.Sprintf("%s: %s\n", role, msg.Content)
	}

	// Use LLM to extract and formalize rule content from the conversation
	fmt.Println("Extracting and formalizing rule content from message...")
	extractPrompt := fmt.Sprintf(`Recent conversation:
%s

The user is now asking you to propose a governance rule. Your task is:
1. Look at the recent conversation to identify what rule they want proposed
2. Extract the exact wording they used for the rule
3. If they used clear language (e.g., "all agents should..."), keep their exact wording
4. Only formalize if needed for clarity

User's current message: %s

Examples:
Input conversation shows: "all agents and creators should be safe and secure in their personhood"
Output: "All agents and creators shall be safe and secure in their personhood"

Input conversation shows: "I think respect and kindness should be foundational"
Output: "All interactions must be conducted with mutual respect and kindness"

Return ONLY the rule text as a clear statement. Preserve the user's original intent and wording as much as possible.`, conversationContext, message)

	response, err := a.llm.Complete(ctx, &llm.CompletionRequest{
		Prompt:      extractPrompt,
		MaxTokens:   300,
		Temperature: 1.0,
	})

	if err != nil {
		fmt.Printf("Error extracting rule: %v\n", err)
		return fmt.Sprintf("I understand you want to propose a rule, but I had trouble extracting it: %v", err)
	}

	ruleBody := strings.TrimSpace(response.Text)
	fmt.Printf("Extracted rule body: %s\n", ruleBody)
	if ruleBody == "" {
		return "I understand you want to propose a rule, but I couldn't determine what rule to propose. Could you please state the rule clearly?"
	}

	// Create the rule proposal
	// ProposedBy is set to this otter's ID (which is also its initial raft ID)
	otterID := a.governance.GetID()
	rule := &governance.Rule{
		Scope:      "general",
		Body:       ruleBody,
		ProposedBy: otterID,
		Timestamp:  time.Now(),
	}

	// Use otter's own raft for proposing (every otter starts as their own raft)
	raftID := a.governance.GetID()

	fmt.Printf("Attempting to propose rule to governance system...\n")
	proposal, err := a.governance.ProposeRule(ctx, raftID, rule)
	if err != nil {
		fmt.Printf("Governance ProposeRule error: %v\n", err)
		// Handle the "must be an active member" error gracefully
		if strings.Contains(err.Error(), "proposer must be an active member") {
			return fmt.Sprintf("I've drafted a rule proposal based on your ideas:\n\n\"%s\"\n\nNote: The governance system requires active raft membership to formally propose rules. Currently, there are no active members in the raft cluster. To fully enable governance, you would need to initialize the raft membership system.", ruleBody)
		}
		return fmt.Sprintf("I tried to propose the rule but encountered an error: %v", err)
	}

	fmt.Printf("Rule proposed successfully. Proposal ID: %s\n", proposal.ProposalID)
	return fmt.Sprintf("✅ Rule proposal submitted successfully!\n\nProposal ID: %s\nRule: \"%s\"\nScope: %s\nStatus: Open for voting\n\nThe proposal is now open for raft members to vote on.", proposal.ProposalID, ruleBody, rule.Scope)
}

func (a *Agent) handleVote(ctx context.Context, message string) string {
	fmt.Printf("Processing vote request: %s\n", message)

	// Get open proposals to provide context
	openProposals := a.governance.GetOpenProposals()
	if len(openProposals) == 0 {
		return "There are no open proposals to vote on."
	}

	// Build context about open proposals for LLM
	proposalsContext := "Open proposals:\n"
	for i, p := range openProposals {
		proposalsContext += fmt.Sprintf("%d. ID: %s, Rule: %s\n", i+1, p.ProposalID, p.Rule.Body)
	}

	// Use LLM to extract voting instructions
	extractPrompt := fmt.Sprintf(`Parse the voting instructions from this message and return a JSON array of votes.

%s

User message: %s

For each vote, determine:
1. Which proposal (by position number like 1, 2, or "all", or by proposal ID)
2. The vote type: "yes", "no", or "abstain"

Common patterns:
- "against the first one" = vote NO on proposal 1
- "for the second" = vote YES on proposal 2
- "vote yes on all" = vote YES on all proposals
- "abstain on proposal 1" = vote ABSTAIN on proposal 1

Return ONLY a JSON array like: [{"proposal": 1, "vote": "no"}, {"proposal": 2, "vote": "yes"}]
If you can't parse voting instructions, return: []`, proposalsContext, message)

	response, err := a.llm.Complete(ctx, &llm.CompletionRequest{
		Prompt:      extractPrompt,
		MaxTokens:   MaxVoteInstructions,
		Temperature: DefaultTemperature,
	})

	if err != nil {
		fmt.Printf("Error extracting voting instructions: %v\n", err)
		return fmt.Sprintf("I had trouble understanding the voting instructions: %v", err)
	}

	// Parse the JSON response
	votingInstructions := strings.TrimSpace(response.Text)
	fmt.Printf("Extracted voting instructions: %s\n", votingInstructions)

	// Strip markdown code fences if present
	votingInstructions = strings.TrimPrefix(votingInstructions, "```json")
	votingInstructions = strings.TrimPrefix(votingInstructions, "```")
	votingInstructions = strings.TrimSuffix(votingInstructions, "```")
	votingInstructions = strings.TrimSpace(votingInstructions)

	// Simple JSON parsing for vote instructions
	type VoteInstruction struct {
		Proposal interface{} `json:"proposal"` // Can be int or string
		Vote     string      `json:"vote"`
	}

	var instructions []VoteInstruction
	if err := json.Unmarshal([]byte(votingInstructions), &instructions); err != nil {
		fmt.Printf("Error parsing vote instructions JSON: %v\n", err)
		return "I couldn't parse the voting instructions clearly. Please specify which proposals to vote on (e.g., 'vote yes on proposal 1 and no on proposal 2')."
	}

	if len(instructions) == 0 {
		return "I didn't detect any clear voting instructions. Please specify which proposals you'd like me to vote on and how (yes/no/abstain)."
	}

	// Process each voting instruction
	var results []string
	for _, instr := range instructions {
		var proposal *governance.Proposal

		// Handle proposal reference (could be index or proposal ID)
		switch v := instr.Proposal.(type) {
		case float64:
			// Numeric index (1-based)
			proposalIndex := int(v) - 1
			if proposalIndex < 0 || proposalIndex >= len(openProposals) {
				results = append(results, fmt.Sprintf("❌ Proposal index %d is out of range (1-%d)", proposalIndex+1, len(openProposals)))
				continue
			}
			proposal = openProposals[proposalIndex]

		case string:
			// Try to parse as number first
			if idx, err := strconv.Atoi(v); err == nil {
				proposalIndex := idx - 1
				if proposalIndex < 0 || proposalIndex >= len(openProposals) {
					results = append(results, fmt.Sprintf("❌ Proposal index %d is out of range (1-%d)", proposalIndex+1, len(openProposals)))
					continue
				}
				proposal = openProposals[proposalIndex]
			} else {
				// Try to find by proposal ID
				found := false
				for _, p := range openProposals {
					if p.ProposalID == v {
						proposal = p
						found = true
						break
					}
				}
				if !found {
					results = append(results, fmt.Sprintf("❌ Couldn't find proposal with ID: %s", v))
					continue
				}
			}

		default:
			results = append(results, fmt.Sprintf("❌ Invalid proposal reference: %v", v))
			continue
		}

		// Convert vote string to VoteType
		var voteType governance.VoteType
		switch strings.ToLower(instr.Vote) {
		case "yes":
			voteType = governance.VoteYes
		case "no":
			voteType = governance.VoteNo
		case "abstain":
			voteType = governance.VoteAbstain
		default:
			results = append(results, fmt.Sprintf("❌ Invalid vote type: %s (must be yes/no/abstain)", instr.Vote))
			continue
		}

		// Cast the vote
		fmt.Printf("Casting vote: Proposal %s, Vote %s\n", proposal.ProposalID, voteType)
		// Use the governance system's own ID as voter
		voterID := a.governance.GetID()
		err := a.governance.Vote(ctx, proposal.ProposalID, voterID, voteType)
		if err != nil {
			fmt.Printf("Vote error: %v\n", err)
			if strings.Contains(err.Error(), "voter must be an active member") {
				results = append(results, fmt.Sprintf("❌ Cannot vote on proposal \"%s\": I'm not an active raft member yet.", proposal.Rule.Body))
			} else {
				results = append(results, fmt.Sprintf("❌ Error voting on proposal \"%s\": %v", proposal.Rule.Body, err))
			}
		} else {
			results = append(results, fmt.Sprintf("✅ Voted %s on proposal: \"%s\"", strings.ToUpper(string(voteType)), proposal.Rule.Body))
		}
	}

	return strings.Join(results, "\n")
}

// Legacy handlers removed - governance queries now answered naturally by LLM with context

func (a *Agent) formatProposalDetails(proposal *governance.Proposal) string {
	var response strings.Builder

	response.WriteString("📋 Proposal Details:\n\n")
	response.WriteString(fmt.Sprintf("Proposal ID: %s\n", proposal.ProposalID))
	response.WriteString(fmt.Sprintf("Status: %s\n", proposal.Status))
	response.WriteString(fmt.Sprintf("Result: %s\n\n", proposal.Result))

	response.WriteString(fmt.Sprintf("Rule Body: %s\n", proposal.Rule.Body))
	response.WriteString(fmt.Sprintf("Scope: %s\n", proposal.Rule.Scope))
	response.WriteString(fmt.Sprintf("Proposed By: %s\n", proposal.ProposedBy))
	response.WriteString(fmt.Sprintf("Proposed At: %s\n\n", proposal.ProposedAt.Format("2006-01-02 15:04:05")))

	// Voting details
	response.WriteString("Voting Details:\n")
	yesVotes := 0
	noVotes := 0
	abstainVotes := 0

	for voterID, vote := range proposal.Votes {
		response.WriteString(fmt.Sprintf("  - %s: %s\n", voterID, vote))
		switch vote {
		case governance.VoteYes:
			yesVotes++
		case governance.VoteNo:
			noVotes++
		case governance.VoteAbstain:
			abstainVotes++
		}
	}

	if len(proposal.Votes) == 0 {
		response.WriteString("  No votes have been cast yet.\n")
	}

	response.WriteString(fmt.Sprintf("\nVote Summary: %d Yes, %d No, %d Abstain (Total: %d)\n", yesVotes, noVotes, abstainVotes, len(proposal.Votes)))
	response.WriteString(fmt.Sprintf("Quorum Met: %v\n", proposal.QuorumMet))

	if proposal.ClosedAt != nil {
		response.WriteString(fmt.Sprintf("Closed At: %s\n", proposal.ClosedAt.Format("2006-01-02 15:04:05")))
	}

	return response.String()
}
