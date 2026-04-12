package governance

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"otter-ai/internal/llm"
	"otter-ai/internal/memory"
)

// Constants for governance thresholds and timeouts
const (
	MemberExpirationDays    = 90
	LivenessCheckInterval   = 1 * time.Hour
	QuorumPercentage        = 67 // 2/3 majority
	SuperMajorityPercentage = 75 // 3/4 majority for overrides
	MinimumVotingMembers    = 2
	UnanimousVotingMembers  = 2
	SoloRaftAutoAdopt       = 1
	GovernanceHTTPTimeout   = 15 * time.Second
	NegotiationVoteTimeout  = 30 * time.Second
	NegotiationPollInterval = 500 * time.Millisecond
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
	NegotiationID  string
	Raft1ID        string
	Raft2ID        string
	TargetEndpoint string
	Conflicts      []*RuleConflict
	ProposedRule   *Rule     // The negotiated compromise rule
	Raft1Proposal  *Proposal // Proposal in raft 1
	Raft2Proposal  *Proposal // Proposal in raft 2
	Status         NegotiationStatus
	StartedAt      time.Time
	CompletedAt    *time.Time
	LLMTranscript  []string // Record of LLM negotiation
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
	ticker := time.NewTicker(LivenessCheckInterval)
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

	expirationThreshold := time.Now().Add(-MemberExpirationDays * 24 * time.Hour)

	for _, raft := range g.rafts.rafts {
		raft.mu.Lock()
		for _, member := range raft.Members {
			if member.State == StateActive && member.LastSeenAt.Before(expirationThreshold) {
				member.State = StateExpired
				expiresAt := member.LastSeenAt.Add(MemberExpirationDays * 24 * time.Hour)
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

	if rule.Timestamp.IsZero() {
		rule.Timestamp = time.Now()
	}

	// Set raft ID on rule
	rule.RaftID = raftID

	if rule.RuleID == "" {
		rule.RuleID = generateID(rule)
	}

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
		quorumThreshold := (totalActive*QuorumPercentage + 99) / 100 // Ceiling calculation
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
			requiredVotes := (totalActive*SuperMajorityPercentage + 99) / 100 // Ceiling calculation
			adopted = yesVotes >= requiredVotes
		} else {
			// 2/3 majority: YES >= 2/3 of total active members
			requiredVotes := (totalActive*QuorumPercentage + 99) / 100 // Ceiling calculation
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
	negotiation, err := g.startNegotiation(ctx, targetRaftID, targetOtterEndpoint, conflicts, llmProvider)
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

	// Request membership from target raft via API.
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return fmt.Errorf("target endpoint is required for join request")
	}
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = "http://" + endpoint
	}

	joinReq := map[string]string{
		"raft_id":      targetRaftID,
		"requester_id": g.config.ID,
		"public_key":   hex.EncodeToString(g.crypto.GetPublicKey()),
	}
	body, err := json.Marshal(joinReq)
	if err != nil {
		return fmt.Errorf("failed to marshal join request: %w", err)
	}

	url := strings.TrimRight(endpoint, "/") + "/api/v1/governance/join"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create join request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: GovernanceHTTPTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send join request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read join response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Roll back local membership when remote induction fails.
		g.rafts.mu.Lock()
		delete(g.rafts.rafts, targetRaftID)
		g.rafts.mu.Unlock()
		return fmt.Errorf("join request rejected (%d): %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	// Reflect self as active local member in this raft after successful induction.
	raft.mu.Lock()
	raft.Members[g.config.ID] = &Member{
		ID:         g.config.ID,
		State:      StateActive,
		JoinedAt:   time.Now(),
		LastSeenAt: time.Now(),
		PublicKey:  g.crypto.GetPublicKey(),
		InductedBy: targetRaftID,
	}
	raft.mu.Unlock()

	if err := g.saveRaft(ctx, raft); err != nil {
		fmt.Printf("Warning: Failed to persist inducted raft membership %s: %v\n", targetRaftID, err)
	}

	return nil
}

// startNegotiation initiates LLM-based negotiation between conflicting rafts
func (g *Governance) startNegotiation(ctx context.Context, targetRaftID string, targetEndpoint string, conflicts []*RuleConflict, llmProvider interface{}) (*Negotiation, error) {
	if len(conflicts) == 0 {
		return nil, fmt.Errorf("no conflicts to negotiate")
	}

	negotiationID := generateID(fmt.Sprintf("negotiation-%s-%d", targetRaftID, time.Now().Unix()))

	negotiation := &Negotiation{
		NegotiationID:  negotiationID,
		Raft1ID:        conflicts[0].Raft1ID, // Primary conflict source
		Raft2ID:        targetRaftID,
		TargetEndpoint: strings.TrimSpace(targetEndpoint),
		Conflicts:      conflicts,
		Status:         NegotiationInProgress,
		StartedAt:      time.Now(),
		LLMTranscript:  make([]string, 0),
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

	scope := negotiation.Conflicts[0].ConflictScope
	body := ""

	if provider, ok := llmProvider.(interface {
		Complete(context.Context, *llm.CompletionRequest) (*llm.CompletionResponse, error)
	}); ok {
		resp, err := provider.Complete(ctx, &llm.CompletionRequest{
			Prompt:      fmt.Sprintf("%s\n\nReturn ONLY JSON in this shape: {\"scope\":\"...\",\"body\":\"...\"}", prompt),
			MaxTokens:   400,
			Temperature: 0.2,
		})
		if err == nil && resp != nil {
			negotiation.LLMTranscript = append(negotiation.LLMTranscript, resp.Text)
			if parsedScope, parsedBody := parseNegotiatedRuleResponse(resp.Text, scope); parsedBody != "" {
				scope = parsedScope
				body = parsedBody
			}
		}
	}

	if body == "" {
		body = synthesizeCompromiseRuleBody(negotiation.Conflicts)
	}

	proposedBy := g.pickActiveMemberID(negotiation.Raft1ID)
	if proposedBy == "" {
		proposedBy = g.config.ID
	}

	compromiseRule := &Rule{
		RuleID:     generateID(fmt.Sprintf("compromise-%s-%s", negotiation.NegotiationID, negotiation.Raft1ID)),
		RaftID:     negotiation.Raft1ID,
		Scope:      scope,
		Version:    maxConflictVersion(negotiation.Conflicts) + 1,
		Timestamp:  time.Now(),
		Body:       body,
		ProposedBy: proposedBy,
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
func (g *Governance) executeDualRaftVote(ctx context.Context, negotiation *Negotiation, _ interface{}) error {
	proposer1 := g.pickActiveMemberID(negotiation.Raft1ID)
	proposer2 := g.pickActiveMemberID(negotiation.Raft2ID)
	if proposer1 == "" || proposer2 == "" {
		return fmt.Errorf("cannot execute dual-raft vote: both rafts must have at least one active member")
	}

	// Create independent rule instances so each raft owns its own rule ID.
	rule1 := *negotiation.ProposedRule
	rule1.RaftID = negotiation.Raft1ID
	rule1.ProposedBy = proposer1
	rule1.RuleID = generateID(fmt.Sprintf("%s|%s|%s", negotiation.Raft1ID, rule1.Scope, rule1.Body))

	rule2 := *negotiation.ProposedRule
	rule2.RaftID = negotiation.Raft2ID
	rule2.ProposedBy = proposer2
	rule2.RuleID = generateID(fmt.Sprintf("%s|%s|%s", negotiation.Raft2ID, rule2.Scope, rule2.Body))

	proposal1, err := g.ProposeRule(ctx, negotiation.Raft1ID, &rule1)
	if err != nil {
		return fmt.Errorf("failed to propose to raft 1: %w", err)
	}

	proposal2, err := g.ProposeRule(ctx, negotiation.Raft2ID, &rule2)
	if err != nil {
		return fmt.Errorf("failed to propose to raft 2: %w", err)
	}

	negotiation.Raft1Proposal = proposal1
	negotiation.Raft2Proposal = proposal2

	// Cast initial YES votes from the local active members we selected as proposers.
	_ = g.Vote(ctx, proposal1.ProposalID, proposer1, VoteYes)
	_ = g.Vote(ctx, proposal2.ProposalID, proposer2, VoteYes)

	ticker := time.NewTicker(NegotiationPollInterval)
	defer ticker.Stop()
	deadline := time.NewTimer(NegotiationVoteTimeout)
	defer deadline.Stop()

	for {
		latest1, ok1 := g.GetProposal(proposal1.ProposalID)
		latest2, ok2 := g.GetProposal(proposal2.ProposalID)
		if !ok1 || !ok2 {
			return fmt.Errorf("negotiation proposals missing while awaiting outcome")
		}

		if latest1.Status == ProposalClosed && latest2.Status == ProposalClosed {
			if latest1.Result == ResultAdopted && latest2.Result == ResultAdopted {
				return nil
			}
			return fmt.Errorf("negotiation vote failed: raft1=%s raft2=%s", latest1.Result, latest2.Result)
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("negotiation vote canceled: %w", ctx.Err())
		case <-deadline.C:
			return fmt.Errorf("negotiation vote timed out waiting for both rafts to close proposals")
		case <-ticker.C:
		}
	}
}

// fetchRaftRules fetches all rules from a remote raft
// In a real implementation, this would make an HTTP/gRPC call
func (g *Governance) fetchRaftRules(ctx context.Context, endpoint string, raftID string) (map[string]*Rule, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return nil, fmt.Errorf("target endpoint is required")
	}
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = "http://" + endpoint
	}

	url := strings.TrimRight(endpoint, "/") + "/api/v1/governance/rules"
	client := &http.Client{Timeout: GovernanceHTTPTimeout}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed creating request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed fetching raft rules from %s: %w", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed reading rules response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rules endpoint returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	rules := make(map[string]*Rule)

	var byScope map[string]*Rule
	if err := json.Unmarshal(body, &byScope); err == nil && len(byScope) > 0 {
		now := time.Now()
		for scope, rule := range byScope {
			if rule == nil {
				continue
			}
			if rule.Scope == "" {
				rule.Scope = scope
			}
			if rule.RaftID == "" {
				rule.RaftID = raftID
			}
			if rule.Timestamp.IsZero() {
				rule.Timestamp = now
			}
			if rule.Version == 0 {
				rule.Version = 1
			}
			if rule.RuleID == "" {
				rule.RuleID = generateID(fmt.Sprintf("%s|%s|%s", raftID, rule.Scope, rule.Body))
			}
			rules[rule.RuleID] = rule
		}
		return rules, nil
	}

	var asList []*Rule
	if err := json.Unmarshal(body, &asList); err == nil && len(asList) > 0 {
		now := time.Now()
		for _, rule := range asList {
			if rule == nil {
				continue
			}
			if rule.RaftID == "" {
				rule.RaftID = raftID
			}
			if rule.Timestamp.IsZero() {
				rule.Timestamp = now
			}
			if rule.Version == 0 {
				rule.Version = 1
			}
			if rule.RuleID == "" {
				rule.RuleID = generateID(fmt.Sprintf("%s|%s|%s", raftID, rule.Scope, rule.Body))
			}
			rules[rule.RuleID] = rule
		}
		return rules, nil
	}

	return nil, fmt.Errorf("unable to parse rules response from %s", url)
}

func parseNegotiatedRuleResponse(raw string, defaultScope string) (string, string) {
	clean := strings.TrimSpace(raw)
	clean = strings.TrimPrefix(clean, "```json")
	clean = strings.TrimPrefix(clean, "```")
	clean = strings.TrimSuffix(clean, "```")
	clean = strings.TrimSpace(clean)

	var parsed struct {
		Scope string `json:"scope"`
		Body  string `json:"body"`
	}
	if err := json.Unmarshal([]byte(clean), &parsed); err == nil {
		scope := strings.TrimSpace(parsed.Scope)
		if scope == "" {
			scope = defaultScope
		}
		return scope, strings.TrimSpace(parsed.Body)
	}

	if clean == "" {
		return defaultScope, ""
	}
	return defaultScope, clean
}

func synthesizeCompromiseRuleBody(conflicts []*RuleConflict) string {
	if len(conflicts) == 0 {
		return "Compromise rule: both rafts must explicitly approve a shared policy before it becomes active."
	}

	var lines []string
	for _, c := range conflicts {
		if c == nil || c.Rule1 == nil || c.Rule2 == nil {
			continue
		}
		line := fmt.Sprintf("Scope %s compromise: evaluate policy using stricter requirement between '%s' and '%s'.", c.ConflictScope, strings.TrimSpace(c.Rule1.Body), strings.TrimSpace(c.Rule2.Body))
		lines = append(lines, line)
	}

	if len(lines) == 0 {
		return "Compromise rule: both rafts must explicitly approve a shared policy before it becomes active."
	}
	return strings.Join(lines, "\n")
}

func maxConflictVersion(conflicts []*RuleConflict) int {
	maxVersion := 0
	for _, c := range conflicts {
		if c == nil {
			continue
		}
		if c.Rule1 != nil && c.Rule1.Version > maxVersion {
			maxVersion = c.Rule1.Version
		}
		if c.Rule2 != nil && c.Rule2.Version > maxVersion {
			maxVersion = c.Rule2.Version
		}
	}
	return maxVersion
}

func (g *Governance) pickActiveMemberID(raftID string) string {
	g.rafts.mu.RLock()
	raft, exists := g.rafts.rafts[raftID]
	g.rafts.mu.RUnlock()
	if !exists {
		return ""
	}

	raft.mu.RLock()
	defer raft.mu.RUnlock()

	if member, ok := raft.Members[g.config.ID]; ok && member.State == StateActive {
		return g.config.ID
	}
	for memberID, member := range raft.Members {
		if member.State == StateActive {
			return memberID
		}
	}
	return ""
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
