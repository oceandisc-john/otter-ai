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
	config     RaftConfig
	memory     *memory.Memory
	members    *MemberRegistry
	rules      *RuleRegistry
	proposals  *ProposalRegistry
	crypto     *CryptoSystem
	mu         sync.RWMutex
	shutdownCh chan struct{}
}

// RaftConfig holds governance configuration
type RaftConfig struct {
	ID            string
	Type          RaftType
	BindAddr      string
	AdvertiseAddr string
	DataDir       string
}

// RaftType defines the type of raft
type RaftType string

const (
	SuperRaft RaftType = "super-raft" // Raft 0: induction authority, global overrides
	Raft      RaftType = "raft"       // Raft 1: membership gatekeeper, quorum enforcement
	SubRaft   RaftType = "sub-raft"   // Raft 2+: regular members
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
	RaftID     string
	State      MembershipState
	JoinedAt   time.Time
	LastSeenAt time.Time
	PublicKey  []byte
	Signature  []byte
	InductedBy string
	ExpiresAt  *time.Time
}

// Rule represents an atomic governance unit
type Rule struct {
	RuleID     string
	Scope      string
	Version    int
	Timestamp  time.Time
	Body       string
	BaseRuleID string // For overrides
	Signature  []byte
	ProposedBy string
	AdoptedAt  *time.Time
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
	Rule       *Rule
	ProposedBy string
	ProposedAt time.Time
	Votes      map[string]VoteType
	Status     ProposalStatus
	QuorumMet  bool
	Result     ProposalResult
	ClosedAt   *time.Time
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

// MemberRegistry manages raft members
type MemberRegistry struct {
	members map[string]*Member
	mu      sync.RWMutex
}

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
		members: &MemberRegistry{
			members: make(map[string]*Member),
		},
		rules: &RuleRegistry{
			rules:  make(map[string]*Rule),
			active: make(map[string]*Rule),
		},
		proposals: &ProposalRegistry{
			proposals: make(map[string]*Proposal),
		},
		crypto:     cryptoSystem,
		shutdownCh: make(chan struct{}),
	}

	// Initialize this otter as a member
	if err := g.initializeSelf(); err != nil {
		return nil, fmt.Errorf("failed to initialize self: %w", err)
	}

	// Start background tasks
	go g.livenessMonitor()

	return g, nil
}

// initializeSelf registers this otter as a member
func (g *Governance) initializeSelf() error {
	now := time.Now()
	member := &Member{
		ID:         g.config.ID,
		RaftID:     g.config.ID,
		State:      StateActive,
		JoinedAt:   now,
		LastSeenAt: now,
		PublicKey:  g.crypto.GetPublicKey(),
		InductedBy: "self", // Bootstrap
	}

	g.members.mu.Lock()
	g.members.members[member.ID] = member
	g.members.mu.Unlock()

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
	g.members.mu.Lock()
	defer g.members.mu.Unlock()

	expirationThreshold := time.Now().Add(-90 * 24 * time.Hour)

	for _, member := range g.members.members {
		if member.State == StateActive && member.LastSeenAt.Before(expirationThreshold) {
			member.State = StateExpired
			expiresAt := member.LastSeenAt.Add(90 * 24 * time.Hour)
			member.ExpiresAt = &expiresAt
		}
	}
}

// ProposeRule submits a new rule proposal
func (g *Governance) ProposeRule(ctx context.Context, rule *Rule) (*Proposal, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Validate proposer is active
	proposer, exists := g.members.members[rule.ProposedBy]
	if !exists || proposer.State != StateActive {
		return nil, fmt.Errorf("proposer must be an active member")
	}

	// Generate proposal ID
	proposalID := generateID(rule)

	proposal := &Proposal{
		ProposalID: proposalID,
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

	// Validate voter is active
	voter, exists := g.members.members[voterID]
	if !exists || voter.State != StateActive {
		return fmt.Errorf("voter must be an active member")
	}

	proposal.Votes[voterID] = vote

	// Check if voting is complete
	g.checkProposalOutcome(proposal)

	return nil
}

// checkProposalOutcome determines if a proposal has reached a decision
func (g *Governance) checkProposalOutcome(proposal *Proposal) {
	activeMembers := g.getActiveMembers()
	totalActive := len(activeMembers)

	// Check quorum (≥ 50% active members)
	votescast := len(proposal.Votes)
	quorumThreshold := (totalActive + 1) / 2
	proposal.QuorumMet = votescast >= quorumThreshold

	if !proposal.QuorumMet {
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

	totalVotes := yesVotes + noVotes
	if totalVotes == 0 {
		return
	}

	// Determine if super-majority is needed (for overrides)
	needsSuperMajority := proposal.Rule.BaseRuleID != ""

	var adopted bool
	if needsSuperMajority {
		// Super-majority: YES > 75% of (YES + NO)
		adopted = float64(yesVotes) > 0.75*float64(totalVotes)
	} else {
		// Simple majority: YES > (YES + NO) / 2
		adopted = yesVotes > totalVotes/2
	}

	if adopted {
		proposal.Result = ResultAdopted
		proposal.Status = ProposalClosed
		now := time.Now()
		proposal.ClosedAt = &now
		proposal.Rule.AdoptedAt = &now

		// Activate the rule
		g.activateRule(proposal.Rule)
	} else if votescast >= totalActive {
		// All members voted, but not adopted
		proposal.Result = ResultRejected
		proposal.Status = ProposalClosed
		now := time.Now()
		proposal.ClosedAt = &now
	}
}

// activateRule adds a rule to the active rule set
func (g *Governance) activateRule(rule *Rule) {
	g.rules.mu.Lock()
	defer g.rules.mu.Unlock()

	g.rules.rules[rule.RuleID] = rule
	g.rules.active[rule.Scope] = rule

	// If this is an override, deactivate the base rule
	if rule.BaseRuleID != "" {
		baseRule := g.rules.rules[rule.BaseRuleID]
		if baseRule != nil && g.rules.active[baseRule.Scope] == baseRule {
			delete(g.rules.active, baseRule.Scope)
		}
	}
}

// getActiveMembers returns all active members
func (g *Governance) getActiveMembers() []*Member {
	g.members.mu.RLock()
	defer g.members.mu.RUnlock()

	var active []*Member
	for _, member := range g.members.members {
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

// RequestJoin handles a join request
func (g *Governance) RequestJoin(ctx context.Context, requesterID string, publicKey []byte) error {
	// Only Super-Raft and Raft can induct new members
	if g.config.Type != SuperRaft && g.config.Type != Raft {
		return fmt.Errorf("only super-raft and raft can induct new members")
	}

	member := &Member{
		ID:         requesterID,
		RaftID:     g.config.ID,
		State:      StateActive,
		JoinedAt:   time.Now(),
		LastSeenAt: time.Now(),
		PublicKey:  publicKey,
		InductedBy: g.config.ID,
	}

	g.members.mu.Lock()
	g.members.members[requesterID] = member
	g.members.mu.Unlock()

	return nil
}

// GetPublicKey returns this otter's public key
func (g *Governance) GetPublicKey() []byte {
	return g.crypto.GetPublicKey()
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
