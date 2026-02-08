package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

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

	// Check if this is a governance action request
	if action, handled := a.handleGovernanceAction(ctx, message); handled {
		// Store the interaction as memory
		interactionMemory := &memory.MemoryRecord{
			Type:       memory.MemoryTypeLongTerm,
			Content:    fmt.Sprintf("User: %s\nOtter: %s", message, action),
			Embedding:  embedding,
			Importance: 0.7, // Higher importance for governance actions
			Metadata: map[string]interface{}{
				"user_message": message,
				"response":     action,
				"type":         "governance_action",
			},
		}
		a.memory.Store(ctx, interactionMemory)
		return action, nil
	}

	// Search for relevant memories
	memories, err := a.memory.Search(ctx, embedding, memory.MemoryTypeLongTerm, 5)
	if err != nil {
		return "", fmt.Errorf("failed to search memories: %w", err)
	}

	// Log memory retrieval for debugging
	fmt.Printf("Retrieved %d memories for query: %s\n", len(memories), message)

	// Build context from memories
	memoryContext := ""
	if len(memories) > 0 {
		memoryContext = "Previous interactions you remember:\n"
		for _, mem := range memories {
			memoryContext += "- " + mem.Content + "\n"
		}
	} else {
		memoryContext = "No previous relevant interactions found in memory.\n"
	}

	// Get active governance rules
	rules := a.governance.GetActiveRules()
	rulesContext := ""
	if len(rules) > 0 {
		rulesContext = "Active governance rules you must follow:\n"
		for _, rule := range rules {
			rulesContext += fmt.Sprintf("- Rule [%s]: %s\n", rule.Scope, rule.Body)
		}
	}

	// Build prompt
	systemPrompt := fmt.Sprintf(`You are Otter-AI, a governed AI agent with persistent memory capabilities and governance actions.

IMPORTANT: You have a vector memory system that stores all conversations. When relevant memories are retrieved, they appear below. You DO have memories and should acknowledge them when asked.

GOVERNANCE CAPABILITIES:
- When users ask you to "propose a rule" or "submit a rule to the raft", I will automatically handle that action
- You can suggest rules when asked, and I will propose them to the governance system when requested
- The governance system uses a raft-based consensus model for decision making

%s

%s

When users ask about your memory or past conversations, reference the information above. When discussing governance, you can propose rules and they will be submitted to the raft for voting.`, rulesContext, memoryContext)

	// Generate response
	response, err := a.llm.Complete(ctx, &llm.CompletionRequest{
		SystemPrompt: systemPrompt,
		Prompt:       message,
		MaxTokens:    500,
		Temperature:  1.0, // Use default temperature for compatibility with all models
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

// handleGovernanceAction detects and handles governance-related actions
func (a *Agent) handleGovernanceAction(ctx context.Context, message string) (string, bool) {
	// Use LLM to classify the intent
	fmt.Printf("Analyzing governance intent for: %s\n", message)

	classificationPrompt := `Classify the user's intent into ONE of these categories:

1. "active_rules" - User wants to see currently active/adopted rules that are in effect
2. "proposals" - User wants to see rule proposals (open for voting, pending, or voting status)
3. "propose_rule" - User wants to CREATE/submit a new rule proposal, or is discussing ideas they want to formalize
4. "vote" - User wants you (the agent) to vote on a proposal or indicates voting preference
5. "not_governance" - User is not asking about governance

Examples:
- "what rules are in effect" -> active_rules
- "what rules are active" -> active_rules
- "show me the rules" -> active_rules
- "what proposals are open" -> proposals
- "has anyone voted on this" -> proposals
- "are there any rule proposals" -> proposals
- "tell me what proposals are open" -> proposals
- "what rules are open" -> proposals (rules "open" for voting means proposals)
- "you should propose [rule text]" -> propose_rule
- "propose this rule: [text]" -> propose_rule
- "I think [idea] might be a good starting point" -> propose_rule (expressing ideas to formalize)
- "would you propose this?" -> propose_rule
- "how can we create a proposal" -> propose_rule
- "let's make a rule about" -> propose_rule
- "how can we shape these thoughts into a proposal" -> propose_rule
- "I'd like to suggest" -> propose_rule
- "can you help me draft a rule" -> propose_rule
- "vote yes on that" -> vote
- "I think you should vote for the first one" -> vote
- "you should against the first one, and for the second" -> vote
- "vote no on proposal X" -> vote
- "how are you doing" -> not_governance

User message: ` + message + `

Respond with ONLY one word: active_rules, proposals, propose_rule, vote, or not_governance`

	response, err := a.llm.Complete(ctx, &llm.CompletionRequest{
		Prompt:      classificationPrompt,
		MaxTokens:   10,
		Temperature: 1.0, // Use default for compatibility with all models
	})

	if err != nil {
		fmt.Printf("Error classifying governance intent: %v\n", err)
		return "", false
	}

	intent := strings.TrimSpace(strings.ToLower(response.Text))
	fmt.Printf("Classified intent as: %s\n", intent)

	switch intent {
	case "active_rules":
		fmt.Printf("Routing to active rules query\n")
		return a.handleRuleQuery(ctx), true

	case "proposals":
		fmt.Printf("Routing to proposal query\n")
		return a.handleProposalQuery(ctx, message), true

	case "propose_rule":
		fmt.Printf("Routing to rule proposal submission\n")
		return a.handleRuleProposal(ctx, message), true

	case "vote":
		fmt.Printf("Routing to vote handler\n")
		return a.handleVote(ctx, message), true

	case "not_governance":
		fmt.Printf("Not a governance request\n")
		return "", false

	default:
		fmt.Printf("Unknown intent: %s, treating as not governance\n", intent)
		return "", false
	}
}

// handleRuleProposal handles submitting a new rule proposal
func (a *Agent) handleRuleProposal(ctx context.Context, message string) string {
	// Use LLM to extract and formalize rule content from the conversation
	fmt.Println("Extracting and formalizing rule content from message...")
	extractPrompt := fmt.Sprintf(`The user is expressing ideas for a governance rule. Your task is to:
1. Extract all the ideas and principles they've mentioned
2. Synthesize them into a clear, concise governance rule
3. Format it as a formal rule statement

User's message: %s

If the message contains multiple ideas or principles, combine them into a single coherent rule. If the user is asking how to formalize their thoughts, take their expressed ideas and draft a proper rule statement.

Examples:
Input: "I think respect and kindness should be foundational"
Output: "All interactions must be conducted with mutual respect and kindness"

Input: "Everyone should have their own space and feel secure in it"
Output: "Each member has sovereignty over their own domain and this autonomy must be respected"

Return ONLY the rule text as a clear, actionable statement. Do not include explanations or meta-commentary.`, message)

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
		MaxTokens:   200,
		Temperature: 1.0, // Use default for compatibility with all models
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
		err := a.governance.Vote(ctx, proposal.ProposalID, "otter-1", voteType)
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

func (a *Agent) handleRuleQuery(ctx context.Context) string {
	fmt.Printf("Retrieving active governance rules...\n")
	rules := a.governance.GetActiveRules()

	if len(rules) == 0 {
		// Use LLM to reflect on the absence of governance rules
		prompt := `You are an AI assistant in a governance system. The user has asked about active governance rules, but there are currently no rules in place.

Reflect thoughtfully on what this means:
- Explain that the governance system is currently operating without formal rules
- Discuss the implications of having no established guidelines
- Suggest that this might be a good opportunity to propose and establish foundational rules
- Encourage the user to think about what guidelines would be most valuable for the system

Keep your response conversational, helpful, and encouraging. This is a chance to help bootstrap the governance process.`

		response, err := a.llm.Complete(ctx, &llm.CompletionRequest{
			Prompt:      prompt,
			Temperature: 1.0,
			MaxTokens:   300,
		})
		if err != nil {
			fmt.Printf("Failed to generate reflection on empty rules: %v\n", err)
			return "There are currently no active governance rules in the system. This might be a good opportunity to propose some foundational guidelines!"
		}
		return response.Text
	}

	var response strings.Builder
	response.WriteString(fmt.Sprintf("📋 Active Governance Rules (%d total):\n\n", len(rules)))

	// Sort rules by ID for consistent ordering
	ruleIDs := make([]string, 0, len(rules))
	for id := range rules {
		ruleIDs = append(ruleIDs, id)
	}

	for i, id := range ruleIDs {
		rule := rules[id]
		response.WriteString(fmt.Sprintf("%d. Rule ID: %s\n", i+1, rule.RuleID))
		response.WriteString(fmt.Sprintf("   Scope: %s\n", rule.Scope))
		response.WriteString(fmt.Sprintf("   Body: %s\n", rule.Body))
		response.WriteString(fmt.Sprintf("   Proposed By: %s\n", rule.ProposedBy))

		if rule.AdoptedAt != nil {
			response.WriteString(fmt.Sprintf("   Adopted At: %s\n", rule.AdoptedAt.Format("2006-01-02 15:04:05")))
		}

		if rule.BaseRuleID != "" {
			response.WriteString(fmt.Sprintf("   Overrides: %s\n", rule.BaseRuleID))
		}

		response.WriteString("\n")
	}

	fmt.Printf("Formatted %d active rules for response\n", len(rules))
	return response.String()
}

func (a *Agent) handleProposalQuery(ctx context.Context, message string) string {
	fmt.Printf("Retrieving proposal information...\n")

	// Try to extract a proposal ID from the message
	// Look for hex strings that could be proposal IDs
	proposalIDPattern := regexp.MustCompile(`\b([a-f0-9]{32})\b`)
	matches := proposalIDPattern.FindStringSubmatch(strings.ToLower(message))

	if len(matches) > 1 {
		// Specific proposal ID mentioned
		proposalID := matches[1]
		fmt.Printf("Looking up specific proposal: %s\n", proposalID)
		proposal, exists := a.governance.GetProposal(proposalID)

		if !exists {
			return fmt.Sprintf("I couldn't find a proposal with ID: %s", proposalID)
		}

		return a.formatProposalDetails(proposal)
	}

	// No specific ID mentioned, check if asking about latest/recent proposal
	messageLower := strings.ToLower(message)
	if strings.Contains(messageLower, "this") || strings.Contains(messageLower, "that") || strings.Contains(messageLower, "latest") || strings.Contains(messageLower, "recent") {
		// Try to get the most recent proposal from memory or all proposals
		allProposals := a.governance.GetAllProposals()
		if len(allProposals) > 0 {
			// Find the most recent proposal
			var mostRecent *governance.Proposal
			for _, p := range allProposals {
				if mostRecent == nil || p.ProposedAt.After(mostRecent.ProposedAt) {
					mostRecent = p
				}
			}
			fmt.Printf("Showing most recent proposal: %s\n", mostRecent.ProposalID)
			return a.formatProposalDetails(mostRecent)
		}
	}

	// General proposal listing
	openProposals := a.governance.GetOpenProposals()

	if len(openProposals) == 0 {
		// Use LLM to reflect on the absence of proposals
		prompt := `You are an AI assistant in a governance system. The user has asked about current proposals, but there are no open proposals at the moment.

Reflect thoughtfully on what this means:
- Explain that there are currently no proposals being voted on
- This could mean either the system is new, or all previous proposals have been decided upon
- Suggest that if they have ideas for how the system should operate, they could propose new rules
- Keep it conversational and encouraging

Keep your response brief and helpful.`

		response, err := a.llm.Complete(ctx, &llm.CompletionRequest{
			Prompt:      prompt,
			Temperature: 1.0,
			MaxTokens:   200,
		})
		if err != nil {
			fmt.Printf("Failed to generate reflection on empty proposals: %v\n", err)
			return "There are currently no open proposals. If you have ideas for governance rules, feel free to propose them!"
		}
		return response.Text
	}

	var response strings.Builder
	response.WriteString(fmt.Sprintf("🗳️  Open Proposals (%d total):\n\n", len(openProposals)))

	for i, proposal := range openProposals {
		response.WriteString(fmt.Sprintf("%d. Proposal ID: %s\n", i+1, proposal.ProposalID))
		response.WriteString(fmt.Sprintf("   Rule: %s\n", proposal.Rule.Body))
		response.WriteString(fmt.Sprintf("   Proposed By: %s\n", proposal.ProposedBy))
		response.WriteString(fmt.Sprintf("   Proposed At: %s\n", proposal.ProposedAt.Format("2006-01-02 15:04:05")))

		// Show voting status
		yesVotes := 0
		noVotes := 0
		abstainVotes := 0
		for _, vote := range proposal.Votes {
			switch vote {
			case governance.VoteYes:
				yesVotes++
			case governance.VoteNo:
				noVotes++
			case governance.VoteAbstain:
				abstainVotes++
			}
		}

		response.WriteString(fmt.Sprintf("   Votes: %d Yes, %d No, %d Abstain (Total: %d)\n", yesVotes, noVotes, abstainVotes, len(proposal.Votes)))
		response.WriteString(fmt.Sprintf("   Quorum Met: %v\n", proposal.QuorumMet))
		response.WriteString("\n")
	}

	fmt.Printf("Formatted %d open proposals for response\n", len(openProposals))
	return response.String()
}

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
