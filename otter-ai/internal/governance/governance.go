package governance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"otter-ai/internal/memory"
)

// Governance system implementing Raft-based governance model
type Governance struct {
	config       RaftConfig
	memory       *memory.Memory
	rafts        *RaftRegistry        // All rafts this otter is part of
	rules        *RuleRegistry        // Global rule registry
	proposals    *ProposalRegistry    // Proposal registry
	negotiations *NegotiationRegistry // Inter-raft negotiations
	crypto       *CryptoSystem
	mu           sync.RWMutex
	shutdownCh   chan struct{}
}

// RaftConfig holds governance configuration
type RaftConfig struct {
	ID            string
	Type          RaftType // Deprecated: kept for backwards compatibility
	BindAddr      string
	AdvertiseAddr string
	DataDir       string
}

// RaftType is deprecated but kept for backwards compatibility
type RaftType string

const (
	SuperRaft RaftType = "super-raft" // Deprecated
	Raft      RaftType = "raft"       // Deprecated
	SubRaft   RaftType = "sub-raft"   // Deprecated
)

// MembershipState defines the state of a member
type MembershipState string

const (
	StateActive   MembershipState = "active"
	StateInactive MembershipState = "inactive"
	StateExpired  MembershipState = "expired"
	StateRevoked  MembershipState = "revoked"
	StateLeft     MembershipState = "left"
)

// Member represents a raft member
type Member struct {
	ID         string
	State      MembershipState
	JoinedAt   time.Time
	LastSeenAt time.Time
	PublicKey  []byte
	Signature  []byte
	InductedBy string
	ExpiresAt  *time.Time
}

// RaftInfo describes a raft group
type RaftInfo struct {
	RaftID    string
	Members   map[string]*Member // memberID -> Member
	Rules     map[string]*Rule   // ruleID -> Rule
	CreatedAt time.Time
	mu        sync.RWMutex
}

// RaftRegistry manages multiple raft memberships
type RaftRegistry struct {
	rafts map[string]*RaftInfo // raftID -> RaftInfo
	mu    sync.RWMutex
}

// Rule represents an atomic governance unit
type Rule struct {
	RuleID     string
	RaftID     string // Which raft this rule belongs to
	Scope      string
	Version    int
	Timestamp  time.Time
	Body       string
	BaseRuleID string // For overrides
	Signature  []byte
	ProposedBy string
	AdoptedAt  *time.Time
}

// RuleConflict represents a conflict between two raft rules
type RuleConflict struct {
	ConflictID    string
	Raft1ID       string
	Raft2ID       string
	Rule1         *Rule
	Rule2         *Rule
	ConflictScope string // What scope these rules conflict on
	DetectedAt    time.Time
}

// VoteType defines vote options
type VoteType string

const (
	VoteYes     VoteType = "YES"
	VoteNo      VoteType = "NO"
	VoteAbstain VoteType = "ABSTAIN"
)

// Proposal represents a rule proposal
type Proposal struct {
	ProposalID string
	RaftID     string // Which raft this proposal is for
	Rule       *Rule
	ProposedBy string
	ProposedAt time.Time
	Votes      map[string]VoteType
	Status     ProposalStatus
	QuorumMet  bool
	Result     ProposalResult
	ClosedAt   *time.Time
}

// Negotiation represents an inter-raft rule negotiation
type Negotiation struct {
	NegotiationID string
	Raft1ID       string
	Raft2ID       string
	Conflicts     []*RuleConflict
	ProposedRule  *Rule     // The negotiated compromise rule
	Raft1Proposal *Proposal // Proposal in raft 1
	Raft2Proposal *Proposal // Proposal in raft 2
	Status        NegotiationStatus
	StartedAt     time.Time
	CompletedAt   *time.Time
	LLMTranscript []string // Record of LLM negotiation
}

// NegotiationStatus defines negotiation state
type NegotiationStatus string

const (
	NegotiationInProgress NegotiationStatus = "in_progress"
	NegotiationResolved   NegotiationStatus = "resolved"
	NegotiationFailed     NegotiationStatus = "failed"
)

// NegotiationRegistry manages inter-raft negotiations
type NegotiationRegistry struct {
	negotiations map[string]*Negotiation
	mu           sync.RWMutex
}

// ProposalStatus defines proposal state
type ProposalStatus string

const (
	ProposalOpen   ProposalStatus = "open"
	ProposalClosed ProposalStatus = "closed"
)

// ProposalResult defines proposal outcome
type ProposalResult string

const (
	ResultPending  ProposalResult = "pending"
	ResultAdopted  ProposalResult = "adopted"
	ResultRejected ProposalResult = "rejected"
)

// RuleRegistry manages governance rules
type RuleRegistry struct {
	rules  map[string]*Rule
	active map[string]*Rule // Active rules by scope
	mu     sync.RWMutex
}

// ProposalRegistry manages proposals
type ProposalRegistry struct {
	proposals map[string]*Proposal
	mu        sync.RWMutex
}

// New creates a new governance system
func New(config RaftConfig, mem *memory.Memory) (*Governance, error) {
	// Initialize cryptographic system (load existing or generate new)
	cryptoSystem, err := LoadOrGenerateKeys(config.DataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize crypto system: %w", err)
	}

	g := &Governance{
		config: config,
		memory: mem,
		rafts: &RaftRegistry{
			rafts: make(map[string]*RaftInfo),
		},
		rules: &RuleRegistry{
			rules:  make(map[string]*Rule),
			active: make(map[string]*Rule),
		},
		proposals: &ProposalRegistry{
			proposals: make(map[string]*Proposal),
		},
		negotiations: &NegotiationRegistry{
			negotiations: make(map[string]*Negotiation),
		},
		crypto:     cryptoSystem,
		shutdownCh: make(chan struct{}),
	}

	// Initialize this otter as a solo raft
	if err := g.initializeSelf(); err != nil {
		return nil, fmt.Errorf("failed to initialize self: %w", err)
	}

	// Load persisted governance state (rafts, members, rules)
	// This will restore any additional rafts this otter was part of
	if err := g.loadGovernanceState(context.Background()); err != nil {
		// Don't fail if persistence is not available yet, just log
		fmt.Printf("Note: Could not load persisted governance state (may be first run): %v\n", err)
	}

	// Start background tasks
	go g.livenessMonitor()

	return g, nil
}

// initializeSelf creates this otter's initial solo raft
func (g *Governance) initializeSelf() error {
	now := time.Now()
	member := &Member{
		ID:         g.config.ID,
		State:      StateActive,
		JoinedAt:   now,
		LastSeenAt: now,
		PublicKey:  g.crypto.GetPublicKey(),
		InductedBy: "self", // Bootstrap
	}

	// Create initial raft with just this otter
	raft := &RaftInfo{
		RaftID:    g.config.ID, // Raft ID is same as otter ID initially
		Members:   map[string]*Member{member.ID: member},
		Rules:     make(map[string]*Rule),
		CreatedAt: now,
	}

	g.rafts.mu.Lock()
	g.rafts.rafts[raft.RaftID] = raft
	g.rafts.mu.Unlock()

	// Persist the initial raft
	ctx := context.Background()
	if err := g.saveRaft(ctx, raft); err != nil {
		// Don't fail initialization if persistence fails
		fmt.Printf("Warning: Failed to persist initial raft: %v\n", err)
	}

	return nil
}

// livenessMonitor checks for expired members
func (g *Governance) livenessMonitor() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			g.checkExpiredMembers()
		case <-g.shutdownCh:
			return
		}
	}
}

// checkExpiredMembers marks members expired after 90 days of inactivity
func (g *Governance) checkExpiredMembers() {
	g.rafts.mu.Lock()
	defer g.rafts.mu.Unlock()

	expirationThreshold := time.Now().Add(-90 * 24 * time.Hour)

	for _, raft := range g.rafts.rafts {
		raft.mu.Lock()
		for _, member := range raft.Members {
			if member.State == StateActive && member.LastSeenAt.Before(expirationThreshold) {
				member.State = StateExpired
				expiresAt := member.LastSeenAt.Add(90 * 24 * time.Hour)
				member.ExpiresAt = &expiresAt
			}
		}
		raft.mu.Unlock()
	}
}

// ProposeRule submits a new rule proposal for a specific raft
func (g *Governance) ProposeRule(ctx context.Context, raftID string, rule *Rule) (*Proposal, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Validate raft exists
	g.rafts.mu.RLock()
	raft, exists := g.rafts.rafts[raftID]
	g.rafts.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("raft not found: %s", raftID)
	}

	// Validate proposer is active member of this raft
	raft.mu.RLock()
	proposer, exists := raft.Members[rule.ProposedBy]
	raft.mu.RUnlock()

	if !exists || proposer.State != StateActive {
		return nil, fmt.Errorf("proposer must be an active member of raft %s", raftID)
	}

	// Set raft ID on rule
	rule.RaftID = raftID

	// Generate proposal ID
	proposalID := generateID(rule)

	proposal := &Proposal{
		ProposalID: proposalID,
		RaftID:     raftID,
		Rule:       rule,
		ProposedBy: rule.ProposedBy,
		ProposedAt: time.Now(),
		Votes:      make(map[string]VoteType),
		Status:     ProposalOpen,
		Result:     ResultPending,
	}

	g.proposals.mu.Lock()
	g.proposals.proposals[proposalID] = proposal
	g.proposals.mu.Unlock()

	return proposal, nil
}

// Vote casts a vote on a proposal
func (g *Governance) Vote(ctx context.Context, proposalID, voterID string, vote VoteType) error {
	g.proposals.mu.Lock()
	defer g.proposals.mu.Unlock()

	proposal, exists := g.proposals.proposals[proposalID]
	if !exists {
		return fmt.Errorf("proposal not found")
	}

	if proposal.Status != ProposalOpen {
		return fmt.Errorf("proposal is closed")
	}

	// Validate voter is active member of the proposal's raft
	g.rafts.mu.RLock()
	raft, exists := g.rafts.rafts[proposal.RaftID]
	g.rafts.mu.RUnlock()

	if !exists {
		return fmt.Errorf("raft not found")
	}

	raft.mu.RLock()
	voter, exists := raft.Members[voterID]
	raft.mu.RUnlock()

	if !exists || voter.State != StateActive {
		return fmt.Errorf("voter must be an active member of this raft")
	}

	proposal.Votes[voterID] = vote

	// Check if voting is complete
	g.checkProposalOutcome(proposal)

	return nil
}

// checkProposalOutcome determines if a proposal has reached a decision
func (g *Governance) checkProposalOutcome(proposal *Proposal) {
	activeMembers := g.getActiveMembers(proposal.RaftID)
	if activeMembers == nil {
		// Raft doesn't exist - should not happen if proposal was created correctly
		return
	}
	totalActive := len(activeMembers)
	if totalActive == 0 {
		// No active members - proposal cannot proceed
		return
	}

	// Count votes
	yesVotes := 0
	noVotes := 0

	for _, vote := range proposal.Votes {
		switch vote {
		case VoteYes:
			yesVotes++
		case VoteNo:
			noVotes++
		}
	}

	votescast := len(proposal.Votes)
	totalVotes := yesVotes + noVotes

	// Determine adoption based on raft size
	var adopted bool
	var shouldClose bool

	switch totalActive {
	case 1:
		// Solo otter: auto-adopt if they vote YES, reject if NO
		proposal.QuorumMet = votescast >= 1
		if votescast >= 1 {
			adopted = yesVotes >= 1
			shouldClose = true
		}

	case 2:
		// Two otters: require unanimous consent (both must vote YES)
		proposal.QuorumMet = votescast >= 2 // Both must vote
		if proposal.QuorumMet {
			adopted = yesVotes == 2 && noVotes == 0
			shouldClose = true
		}

	default:
		// 3+ otters: 2/3 majority of total active members
		// Quorum: at least 2/3 must participate
		quorumThreshold := (totalActive*2 + 2) / 3 // Ceiling of 2/3
		proposal.QuorumMet = votescast >= quorumThreshold

		if !proposal.QuorumMet {
			return
		}

		if totalVotes == 0 {
			return
		}

		// Determine if super-majority is needed (for overrides)
		needsSuperMajority := proposal.Rule.BaseRuleID != ""

		if needsSuperMajority {
			// Super-majority: YES > 75% of total active members
			requiredVotes := (totalActive*3 + 3) / 4 // Ceiling of 75%
			adopted = yesVotes >= requiredVotes
		} else {
			// 2/3 majority: YES >= 2/3 of total active members
			requiredVotes := (totalActive*2 + 2) / 3 // Ceiling of 2/3
			adopted = yesVotes >= requiredVotes
		}

		// Close if decision reached or all members voted
		shouldClose = adopted || votescast >= totalActive
	}

	if shouldClose {
		if adopted {
			proposal.Result = ResultAdopted
			proposal.Status = ProposalClosed
			now := time.Now()
			proposal.ClosedAt = &now
			proposal.Rule.AdoptedAt = &now

			// Activate the rule
			g.activateRule(proposal.Rule)
		} else {
			// All members voted, but not adopted
			proposal.Result = ResultRejected
			proposal.Status = ProposalClosed
			now := time.Now()
			proposal.ClosedAt = &now
		}
	}
}

// activateRule adds a rule to the active rule set and the raft's rules
func (g *Governance) activateRule(rule *Rule) {
	g.rules.mu.Lock()
	g.rules.rules[rule.RuleID] = rule
	g.rules.active[rule.Scope] = rule
	g.rules.mu.Unlock()

	// Add to raft's rules
	g.rafts.mu.RLock()
	raft, exists := g.rafts.rafts[rule.RaftID]
	g.rafts.mu.RUnlock()

	if exists {
		raft.mu.Lock()
		raft.Rules[rule.RuleID] = rule
		raft.mu.Unlock()

		// Persist the rule and raft to database
		ctx := context.Background()
		if err := g.saveRule(ctx, rule); err != nil {
			fmt.Printf("Warning: Failed to persist rule %s: %v\n", rule.RuleID, err)
		}
	}

	// If this is an override, deactivate the base rule
	if rule.BaseRuleID != "" {
		g.rules.mu.Lock()
		baseRule := g.rules.rules[rule.BaseRuleID]
		if baseRule != nil && g.rules.active[baseRule.Scope] == baseRule {
			delete(g.rules.active, baseRule.Scope)
		}
		g.rules.mu.Unlock()
	}
}

// getActiveMembers returns all active members of a raft
func (g *Governance) getActiveMembers(raftID string) []*Member {
	g.rafts.mu.RLock()
	raft, exists := g.rafts.rafts[raftID]
	g.rafts.mu.RUnlock()

	if !exists {
		return nil
	}

	raft.mu.RLock()
	defer raft.mu.RUnlock()

	var active []*Member
	for _, member := range raft.Members {
		if member.State == StateActive {
			active = append(active, member)
		}
	}
	return active
}

// GetActiveRules returns all active rules
func (g *Governance) GetActiveRules() map[string]*Rule {
	g.rules.mu.RLock()
	defer g.rules.mu.RUnlock()

	rules := make(map[string]*Rule)
	for scope, rule := range g.rules.active {
		rules[scope] = rule
	}
	return rules
}

// GetProposal returns a proposal by ID
func (g *Governance) GetProposal(proposalID string) (*Proposal, bool) {
	g.proposals.mu.RLock()
	defer g.proposals.mu.RUnlock()

	proposal, exists := g.proposals.proposals[proposalID]
	return proposal, exists
}

// GetOpenProposals returns all open proposals
func (g *Governance) GetOpenProposals() []*Proposal {
	g.proposals.mu.RLock()
	defer g.proposals.mu.RUnlock()

	var openProposals []*Proposal
	for _, proposal := range g.proposals.proposals {
		if proposal.Status == ProposalOpen {
			openProposals = append(openProposals, proposal)
		}
	}
	return openProposals
}

// GetAllProposals returns all proposals (open and closed)
func (g *Governance) GetAllProposals() []*Proposal {
	g.proposals.mu.RLock()
	defer g.proposals.mu.RUnlock()

	var proposals []*Proposal
	for _, proposal := range g.proposals.proposals {
		proposals = append(proposals, proposal)
	}
	return proposals
}

// RequestJoin handles a join request from another otter to join a specific raft
// This otter must already be a member of the target raft to accept the request
func (g *Governance) RequestJoin(ctx context.Context, targetRaftID string, requesterID string, publicKey []byte) error {
	// Validate this otter is a member of the target raft
	g.rafts.mu.RLock()
	raft, exists := g.rafts.rafts[targetRaftID]
	g.rafts.mu.RUnlock()

	if !exists {
		return fmt.Errorf("not a member of raft %s", targetRaftID)
	}

	// Create new member
	now := time.Now()
	member := &Member{
		ID:         requesterID,
		State:      StateActive,
		JoinedAt:   now,
		LastSeenAt: now,
		PublicKey:  publicKey,
		InductedBy: g.config.ID,
	}

	// Add member to raft
	raft.mu.Lock()
	raft.Members[requesterID] = member
	raft.mu.Unlock()

	return nil
}

// JoinRaft attempts to join this otter to another otter's raft
// This handles the full flow: adopt rules, detect conflicts, negotiate if needed
func (g *Governance) JoinRaft(ctx context.Context, targetRaftID string, targetOtterEndpoint string, llmProvider interface{}) error {
	// Step 1: Get target raft's rules
	targetRules, err := g.fetchRaftRules(ctx, targetOtterEndpoint, targetRaftID)
	if err != nil {
		return fmt.Errorf("failed to fetch target raft rules: %w", err)
	}

	// Step 2: Detect conflicts with existing rafts
	conflicts := g.detectRuleConflicts(targetRaftID, targetRules)

	// Step 3: If no conflicts, adopt rules and join
	if len(conflicts) == 0 {
		return g.adoptRulesAndJoin(ctx, targetRaftID, targetRules, targetOtterEndpoint)
	}

	// Step 4: If conflicts exist, initiate LLM negotiation
	negotiation, err := g.startNegotiation(ctx, targetRaftID, conflicts, llmProvider)
	if err != nil {
		return fmt.Errorf("negotiation initiation failed: %w", err)
	}

	// Step 5: If negotiation succeeds, propose amendments to both rafts
	if negotiation.Status == NegotiationResolved {
		return g.executeDualRaftVote(ctx, negotiation, llmProvider)
	}

	return fmt.Errorf("negotiation failed: rafts could not agree on common rules")
}

// detectRuleConflicts checks if target raft rules conflict with any current raft rules
func (g *Governance) detectRuleConflicts(targetRaftID string, targetRules map[string]*Rule) []*RuleConflict {
	var conflicts []*RuleConflict

	g.rafts.mu.RLock()
	defer g.rafts.mu.RUnlock()

	// Check each existing raft's rules
	for existingRaftID, existingRaft := range g.rafts.rafts {
		if existingRaftID == targetRaftID {
			continue // Skip self
		}

		existingRaft.mu.RLock()
		for _, targetRule := range targetRules {
			for _, existingRule := range existingRaft.Rules {
				// Rules conflict if they have the same scope but different bodies
				if targetRule.Scope == existingRule.Scope && targetRule.Body != existingRule.Body {
					conflict := &RuleConflict{
						ConflictID:    generateID(fmt.Sprintf("%s-%s", targetRule.RuleID, existingRule.RuleID)),
						Raft1ID:       existingRaftID,
						Raft2ID:       targetRaftID,
						Rule1:         existingRule,
						Rule2:         targetRule,
						ConflictScope: targetRule.Scope,
						DetectedAt:    time.Now(),
					}
					conflicts = append(conflicts, conflict)
				}
			}
		}
		existingRaft.mu.RUnlock()
	}

	return conflicts
}

// adoptRulesAndJoin adopts all target raft rules and joins the raft
func (g *Governance) adoptRulesAndJoin(ctx context.Context, targetRaftID string, targetRules map[string]*Rule, endpoint string) error {
	// Create raft info for the new membership
	g.rafts.mu.Lock()

	raft := &RaftInfo{
		RaftID:    targetRaftID,
		Members:   make(map[string]*Member), // Will be populated when accepted
		Rules:     targetRules,
		CreatedAt: time.Now(),
	}

	g.rafts.rafts[targetRaftID] = raft
	g.rafts.mu.Unlock()

	// Persist the new raft membership
	if err := g.saveRaft(ctx, raft); err != nil {
		fmt.Printf("Warning: Failed to persist raft %s: %v\n", targetRaftID, err)
	}

	// Request membership from target raft
	// In a real implementation, this would be an HTTP/gRPC call
	// For now, we'll leave it as a placeholder
	return nil
}

// startNegotiation initiates LLM-based negotiation between conflicting rafts
func (g *Governance) startNegotiation(ctx context.Context, targetRaftID string, conflicts []*RuleConflict, llmProvider interface{}) (*Negotiation, error) {
	if len(conflicts) == 0 {
		return nil, fmt.Errorf("no conflicts to negotiate")
	}

	negotiationID := generateID(fmt.Sprintf("negotiation-%s-%d", targetRaftID, time.Now().Unix()))

	negotiation := &Negotiation{
		NegotiationID: negotiationID,
		Raft1ID:       conflicts[0].Raft1ID, // Primary conflict source
		Raft2ID:       targetRaftID,
		Conflicts:     conflicts,
		Status:        NegotiationInProgress,
		StartedAt:     time.Now(),
		LLMTranscript: make([]string, 0),
	}

	g.negotiations.mu.Lock()
	g.negotiations.negotiations[negotiationID] = negotiation
	g.negotiations.mu.Unlock()

	// Perform LLM negotiation
	proposedRule, err := g.negotiateWithLLM(ctx, negotiation, llmProvider)
	if err != nil {
		negotiation.Status = NegotiationFailed
		return negotiation, err
	}

	negotiation.ProposedRule = proposedRule
	negotiation.Status = NegotiationResolved
	now := time.Now()
	negotiation.CompletedAt = &now

	return negotiation, nil
}

// negotiateWithLLM uses LLM to negotiate a compromise between conflicting rules
func (g *Governance) negotiateWithLLM(ctx context.Context, negotiation *Negotiation, llmProvider interface{}) (*Rule, error) {
	// Build negotiation prompt
	prompt := g.buildNegotiationPrompt(negotiation)

	// Record the prompt in the transcript
	negotiation.LLMTranscript = append(negotiation.LLMTranscript, prompt)

	// This would use the actual LLM provider
	// For now, returning a placeholder
	// In real implementation: use llmProvider.Complete(ctx, &CompletionRequest{Prompt: prompt, ...})

	compromiseRule := &Rule{
		RuleID:     generateID(fmt.Sprintf("compromise-%s", negotiation.NegotiationID)),
		Scope:      negotiation.Conflicts[0].ConflictScope,
		Version:    1,
		Timestamp:  time.Now(),
		Body:       "# Negotiated compromise rule\n# (LLM-generated)", // Placeholder
		ProposedBy: "llm-negotiation",
	}

	return compromiseRule, nil
}

// buildNegotiationPrompt creates a prompt for LLM negotiation
func (g *Governance) buildNegotiationPrompt(negotiation *Negotiation) string {
	prompt := fmt.Sprintf(`You are mediating a governance rule conflict between two otter rafts.

Raft 1 ID: %s
Raft 2 ID: %s

Conflicts:
`, negotiation.Raft1ID, negotiation.Raft2ID)

	for i, conflict := range negotiation.Conflicts {
		prompt += fmt.Sprintf(`
Conflict %d - Scope: %s
Raft 1 Rule: %s
Raft 2 Rule: %s
`, i+1, conflict.ConflictScope, conflict.Rule1.Body, conflict.Rule2.Body)
	}

	prompt += `
Please propose a compromise rule that respects both rafts' interests and can be adopted by both.
The proposal should be clear, actionable, and acceptable to all members of both rafts.
`

	return prompt
}

// executeDualRaftVote proposes the negotiated rule to both rafts and waits for votes
func (g *Governance) executeDualRaftVote(ctx context.Context, negotiation *Negotiation, llmProvider interface{}) error {
	// Create proposals in both rafts
	proposal1, err := g.ProposeRule(ctx, negotiation.Raft1ID, negotiation.ProposedRule)
	if err != nil {
		return fmt.Errorf("failed to propose to raft 1: %w", err)
	}

	proposal2Rule := *negotiation.ProposedRule
	proposal2Rule.RaftID = negotiation.Raft2ID
	proposal2, err := g.ProposeRule(ctx, negotiation.Raft2ID, &proposal2Rule)
	if err != nil {
		return fmt.Errorf("failed to propose to raft 2: %w", err)
	}

	negotiation.Raft1Proposal = proposal1
	negotiation.Raft2Proposal = proposal2

	// TODO: In a real implementation, this should:
	// 1. Wait for both proposals to complete voting
	// 2. Check if both rafts adopted the rule (proposal1.Result == ResultAdopted && proposal2.Result == ResultAdopted)
	// 3. If both adopted: finalize raft peering
	// 4. If either rejected: return error and clean up
	// For now, returning nil as placeholder - this means the join will appear successful
	// even though votes haven't been cast yet

	return nil
}

// fetchRaftRules fetches all rules from a remote raft
// In a real implementation, this would make an HTTP/gRPC call
func (g *Governance) fetchRaftRules(ctx context.Context, endpoint string, raftID string) (map[string]*Rule, error) {
	// Placeholder - in real implementation:
	// 1. Make HTTP GET to endpoint/api/v1/rafts/{raftID}/rules
	// 2. Parse response into map[string]*Rule
	// 3. Return rules
	return make(map[string]*Rule), nil
}

// GetPublicKey returns this otter's public key
func (g *Governance) GetPublicKey() []byte {
	return g.crypto.GetPublicKey()
}

// GetID returns this otter's ID
func (g *Governance) GetID() string {
	return g.config.ID
}

// GetRaftMembers returns all members of a specific raft
func (g *Governance) GetRaftMembers(raftID string) ([]*Member, error) {
	g.rafts.mu.RLock()
	raft, exists := g.rafts.rafts[raftID]
	g.rafts.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("raft not found: %s", raftID)
	}

	raft.mu.RLock()
	defer raft.mu.RUnlock()

	members := make([]*Member, 0, len(raft.Members))
	for _, member := range raft.Members {
		members = append(members, member)
	}
	return members, nil
}

// GetCrypto returns the crypto system (for advanced operations)
func (g *Governance) GetCrypto() *CryptoSystem {
	return g.crypto
}

// Shutdown gracefully shuts down the governance system
func (g *Governance) Shutdown(ctx context.Context) error {
	close(g.shutdownCh)
	return nil
}

// generateID generates a deterministic ID for an object
func generateID(obj interface{}) string {
	hash := sha256.Sum256([]byte(fmt.Sprintf("%v", obj)))
	return hex.EncodeToString(hash[:16])
}
