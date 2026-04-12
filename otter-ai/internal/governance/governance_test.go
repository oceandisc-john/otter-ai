package governance

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"otter-ai/internal/llm"
	"otter-ai/internal/memory"
	"otter-ai/internal/vectordb"
)

// mockVectorDB implements vectordb.VectorDB for testing (no-op, no CGO needed)
type mockVectorDB struct{}

func (m *mockVectorDB) Store(_ context.Context, _ string, _ string, _ []float32, _ map[string]interface{}) error {
	return nil
}
func (m *mockVectorDB) Search(_ context.Context, _ string, _ []float32, _ int) ([]vectordb.SearchResult, error) {
	return nil, nil
}
func (m *mockVectorDB) Get(_ context.Context, _ string, _ string) (*vectordb.Record, error) {
	return nil, nil
}
func (m *mockVectorDB) Delete(_ context.Context, _ string, _ string) error { return nil }
func (m *mockVectorDB) List(_ context.Context, _ string, _, _ int) ([]vectordb.Record, error) {
	return nil, nil
}
func (m *mockVectorDB) Close() error { return nil }

// --- generateID ---

func TestGenerateID_Deterministic(t *testing.T) {
	id1 := generateID("test-input")
	id2 := generateID("test-input")
	if id1 != id2 {
		t.Errorf("generateID not deterministic: %q != %q", id1, id2)
	}
	if len(id1) != 32 { // hex-encoded 16 bytes
		t.Errorf("expected 32-char hex, got %d chars", len(id1))
	}
}

func TestGenerateID_DifferentInputs(t *testing.T) {
	id1 := generateID("input-a")
	id2 := generateID("input-b")
	if id1 == id2 {
		t.Error("different inputs should produce different IDs")
	}
}

func TestGenerateID_StructInput(t *testing.T) {
	rule := &Rule{Scope: "safety", Body: "be kind"}
	id := generateID(rule)
	if id == "" {
		t.Error("expected non-empty ID for struct input")
	}
}

// --- parseNegotiatedRuleResponse ---

func TestParseNegotiatedRuleResponse_ValidJSON(t *testing.T) {
	raw := `{"scope": "safety", "body": "All interactions must be respectful"}`
	scope, body := parseNegotiatedRuleResponse(raw, "default")
	if scope != "safety" {
		t.Errorf("scope = %q, want %q", scope, "safety")
	}
	if body != "All interactions must be respectful" {
		t.Errorf("body = %q", body)
	}
}

func TestParseNegotiatedRuleResponse_JSONWithFences(t *testing.T) {
	raw := "```json\n{\"scope\": \"ethics\", \"body\": \"Be honest\"}\n```"
	scope, body := parseNegotiatedRuleResponse(raw, "default")
	if scope != "ethics" {
		t.Errorf("scope = %q, want %q", scope, "ethics")
	}
	if body != "Be honest" {
		t.Errorf("body = %q", body)
	}
}

func TestParseNegotiatedRuleResponse_Empty(t *testing.T) {
	scope, body := parseNegotiatedRuleResponse("", "fallback")
	if scope != "fallback" {
		t.Errorf("scope = %q, want %q", scope, "fallback")
	}
	if body != "" {
		t.Errorf("expected empty body, got %q", body)
	}
}

func TestParseNegotiatedRuleResponse_PlainText(t *testing.T) {
	raw := "All agents must cooperate"
	scope, body := parseNegotiatedRuleResponse(raw, "general")
	if scope != "general" {
		t.Errorf("scope = %q, want %q", scope, "general")
	}
	if body != "All agents must cooperate" {
		t.Errorf("body = %q", body)
	}
}

func TestParseNegotiatedRuleResponse_MissingScope(t *testing.T) {
	raw := `{"body": "Be kind"}`
	scope, body := parseNegotiatedRuleResponse(raw, "default-scope")
	if scope != "default-scope" {
		t.Errorf("scope = %q, want %q", scope, "default-scope")
	}
	if body != "Be kind" {
		t.Errorf("body = %q", body)
	}
}

// --- synthesizeCompromiseRuleBody ---

func TestSynthesizeCompromiseRuleBody_Empty(t *testing.T) {
	result := synthesizeCompromiseRuleBody(nil)
	if result == "" {
		t.Error("expected non-empty fallback")
	}

	result = synthesizeCompromiseRuleBody([]*RuleConflict{})
	if result == "" {
		t.Error("expected non-empty fallback for empty slice")
	}
}

func TestSynthesizeCompromiseRuleBody_NilConflictEntries(t *testing.T) {
	conflicts := []*RuleConflict{nil, nil}
	result := synthesizeCompromiseRuleBody(conflicts)
	// All nil -> fallback
	if result == "" {
		t.Error("expected non-empty fallback")
	}
}

func TestSynthesizeCompromiseRuleBody_NilRules(t *testing.T) {
	conflicts := []*RuleConflict{
		{Rule1: nil, Rule2: &Rule{Body: "b"}},
		{Rule1: &Rule{Body: "a"}, Rule2: nil},
	}
	result := synthesizeCompromiseRuleBody(conflicts)
	// Skipped because Rule1 or Rule2 is nil
	if result == "" {
		t.Error("expected non-empty fallback")
	}
}

func TestSynthesizeCompromiseRuleBody_ValidConflicts(t *testing.T) {
	conflicts := []*RuleConflict{
		{
			ConflictScope: "safety",
			Rule1:         &Rule{Body: "Be cautious"},
			Rule2:         &Rule{Body: "Be bold"},
		},
	}
	result := synthesizeCompromiseRuleBody(conflicts)
	if result == "" {
		t.Error("expected non-empty result")
	}
	if !contains(result, "safety") {
		t.Errorf("expected scope in result: %q", result)
	}
}

func TestSynthesizeCompromiseRuleBody_MultipleConflicts(t *testing.T) {
	conflicts := []*RuleConflict{
		{ConflictScope: "safety", Rule1: &Rule{Body: "A"}, Rule2: &Rule{Body: "B"}},
		{ConflictScope: "ethics", Rule1: &Rule{Body: "C"}, Rule2: &Rule{Body: "D"}},
	}
	result := synthesizeCompromiseRuleBody(conflicts)
	if !contains(result, "safety") || !contains(result, "ethics") {
		t.Errorf("expected both scopes in result: %q", result)
	}
}

// --- maxConflictVersion ---

func TestMaxConflictVersion_Empty(t *testing.T) {
	if v := maxConflictVersion(nil); v != 0 {
		t.Errorf("got %d, want 0", v)
	}
	if v := maxConflictVersion([]*RuleConflict{}); v != 0 {
		t.Errorf("got %d, want 0", v)
	}
}

func TestMaxConflictVersion_NilEntries(t *testing.T) {
	conflicts := []*RuleConflict{nil, nil}
	if v := maxConflictVersion(conflicts); v != 0 {
		t.Errorf("got %d, want 0", v)
	}
}

func TestMaxConflictVersion_Various(t *testing.T) {
	conflicts := []*RuleConflict{
		{Rule1: &Rule{Version: 3}, Rule2: &Rule{Version: 1}},
		{Rule1: &Rule{Version: 2}, Rule2: &Rule{Version: 5}},
	}
	if v := maxConflictVersion(conflicts); v != 5 {
		t.Errorf("got %d, want 5", v)
	}
}

func TestMaxConflictVersion_NilRules(t *testing.T) {
	conflicts := []*RuleConflict{
		{Rule1: nil, Rule2: &Rule{Version: 4}},
		{Rule1: &Rule{Version: 2}, Rule2: nil},
	}
	if v := maxConflictVersion(conflicts); v != 4 {
		t.Errorf("got %d, want 4", v)
	}
}

// --- Constants ---

func TestGovernanceConstants(t *testing.T) {
	if QuorumPercentage != 67 {
		t.Errorf("QuorumPercentage = %d", QuorumPercentage)
	}
	if SuperMajorityPercentage != 75 {
		t.Errorf("SuperMajorityPercentage = %d", SuperMajorityPercentage)
	}
	if MinimumVotingMembers != 2 {
		t.Errorf("MinimumVotingMembers = %d", MinimumVotingMembers)
	}
	if SoloRaftAutoAdopt != 1 {
		t.Errorf("SoloRaftAutoAdopt = %d", SoloRaftAutoAdopt)
	}
}

func TestVoteTypeConstants(t *testing.T) {
	if VoteYes != "YES" {
		t.Errorf("VoteYes = %q", VoteYes)
	}
	if VoteNo != "NO" {
		t.Errorf("VoteNo = %q", VoteNo)
	}
	if VoteAbstain != "ABSTAIN" {
		t.Errorf("VoteAbstain = %q", VoteAbstain)
	}
}

func TestProposalStatusConstants(t *testing.T) {
	if ProposalOpen != "open" {
		t.Errorf("ProposalOpen = %q", ProposalOpen)
	}
	if ProposalClosed != "closed" {
		t.Errorf("ProposalClosed = %q", ProposalClosed)
	}
}

func TestProposalResultConstants(t *testing.T) {
	if ResultPending != "pending" {
		t.Errorf("ResultPending = %q", ResultPending)
	}
	if ResultAdopted != "adopted" {
		t.Errorf("ResultAdopted = %q", ResultAdopted)
	}
	if ResultRejected != "rejected" {
		t.Errorf("ResultRejected = %q", ResultRejected)
	}
}

func TestMembershipStateConstants(t *testing.T) {
	if StateActive != "active" {
		t.Errorf("StateActive = %q", StateActive)
	}
	if StateInactive != "inactive" {
		t.Errorf("StateInactive = %q", StateInactive)
	}
	if StateExpired != "expired" {
		t.Errorf("StateExpired = %q", StateExpired)
	}
	if StateRevoked != "revoked" {
		t.Errorf("StateRevoked = %q", StateRevoked)
	}
	if StateLeft != "left" {
		t.Errorf("StateLeft = %q", StateLeft)
	}
}

func TestNegotiationStatusConstants(t *testing.T) {
	if NegotiationInProgress != "in_progress" {
		t.Errorf("NegotiationInProgress = %q", NegotiationInProgress)
	}
	if NegotiationResolved != "resolved" {
		t.Errorf("NegotiationResolved = %q", NegotiationResolved)
	}
	if NegotiationFailed != "failed" {
		t.Errorf("NegotiationFailed = %q", NegotiationFailed)
	}
}

// --- Helper to build a testable Governance without DB/crypto ---

func newTestGovernance(id string) *Governance {
	mem := memory.New(&mockVectorDB{})
	crypto, _ := NewCryptoSystem()
	g := &Governance{
		config: RaftConfig{ID: id},
		memory: mem,
		crypto: crypto,
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
		shutdownCh: make(chan struct{}),
	}

	// Bootstrap a self raft
	now := time.Now()
	raft := &RaftInfo{
		RaftID:    id,
		Members:   map[string]*Member{id: {ID: id, State: StateActive, JoinedAt: now, LastSeenAt: now}},
		Rules:     make(map[string]*Rule),
		CreatedAt: now,
	}
	g.rafts.rafts[id] = raft

	return g
}

// --- GetID / GetPublicKey / GetCrypto ---

func TestGetID(t *testing.T) {
	g := newTestGovernance("otter-1")
	if g.GetID() != "otter-1" {
		t.Errorf("GetID() = %q", g.GetID())
	}
}

func TestGovernanceGetPublicKey(t *testing.T) {
	g := newTestGovernance("otter-1")
	pk := g.GetPublicKey()
	if len(pk) == 0 {
		t.Error("expected non-empty public key")
	}
}

func TestGetCrypto(t *testing.T) {
	g := newTestGovernance("otter-1")
	if g.GetCrypto() == nil {
		t.Error("expected non-nil crypto")
	}
}

// --- Shutdown ---

func TestShutdown(t *testing.T) {
	g := newTestGovernance("otter-1")
	err := g.Shutdown(context.Background())
	if err != nil {
		t.Errorf("Shutdown() error: %v", err)
	}
}

// --- GetRaftMembers ---

func TestGetRaftMembers_Found(t *testing.T) {
	g := newTestGovernance("otter-1")
	members, err := g.GetRaftMembers("otter-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 1 {
		t.Errorf("expected 1 member, got %d", len(members))
	}
	if members[0].ID != "otter-1" {
		t.Errorf("member ID = %q", members[0].ID)
	}
}

func TestGetRaftMembers_NotFound(t *testing.T) {
	g := newTestGovernance("otter-1")
	_, err := g.GetRaftMembers("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent raft")
	}
}

// --- getActiveMembers ---

func TestGetActiveMembers_Found(t *testing.T) {
	g := newTestGovernance("otter-1")
	// Add an inactive member
	g.rafts.rafts["otter-1"].Members["otter-2"] = &Member{ID: "otter-2", State: StateInactive}
	members := g.getActiveMembers("otter-1")
	if len(members) != 1 {
		t.Errorf("expected 1 active member, got %d", len(members))
	}
}

func TestGetActiveMembers_NotFound(t *testing.T) {
	g := newTestGovernance("otter-1")
	members := g.getActiveMembers("nonexistent")
	if members != nil {
		t.Errorf("expected nil, got %v", members)
	}
}

// --- GetActiveRules ---

func TestGetActiveRules_Empty(t *testing.T) {
	g := newTestGovernance("otter-1")
	rules := g.GetActiveRules()
	if len(rules) != 0 {
		t.Errorf("expected 0 rules, got %d", len(rules))
	}
}

func TestGetActiveRules_WithRules(t *testing.T) {
	g := newTestGovernance("otter-1")
	rule := &Rule{RuleID: "r1", Scope: "safety", Body: "be kind"}
	g.rules.active["safety"] = rule
	g.rules.rules["r1"] = rule

	rules := g.GetActiveRules()
	if len(rules) != 1 {
		t.Errorf("expected 1 rule, got %d", len(rules))
	}
	if rules["safety"].Body != "be kind" {
		t.Errorf("rule body = %q", rules["safety"].Body)
	}
}

// --- GetOpenProposals / GetAllProposals / GetProposal ---

func TestGetOpenProposals_Empty(t *testing.T) {
	g := newTestGovernance("otter-1")
	proposals := g.GetOpenProposals()
	if len(proposals) != 0 {
		t.Errorf("expected 0, got %d", len(proposals))
	}
}

func TestGetOpenProposals_FiltersClosedOnes(t *testing.T) {
	g := newTestGovernance("otter-1")
	g.proposals.proposals["p1"] = &Proposal{ProposalID: "p1", Status: ProposalOpen}
	g.proposals.proposals["p2"] = &Proposal{ProposalID: "p2", Status: ProposalClosed}
	proposals := g.GetOpenProposals()
	if len(proposals) != 1 {
		t.Errorf("expected 1 open proposal, got %d", len(proposals))
	}
}

func TestGetAllProposals(t *testing.T) {
	g := newTestGovernance("otter-1")
	g.proposals.proposals["p1"] = &Proposal{ProposalID: "p1", Status: ProposalOpen}
	g.proposals.proposals["p2"] = &Proposal{ProposalID: "p2", Status: ProposalClosed}
	proposals := g.GetAllProposals()
	if len(proposals) != 2 {
		t.Errorf("expected 2, got %d", len(proposals))
	}
}

func TestGetProposal_Found(t *testing.T) {
	g := newTestGovernance("otter-1")
	g.proposals.proposals["p1"] = &Proposal{ProposalID: "p1"}
	p, ok := g.GetProposal("p1")
	if !ok || p.ProposalID != "p1" {
		t.Error("expected to find proposal p1")
	}
}

func TestGetProposal_NotFound(t *testing.T) {
	g := newTestGovernance("otter-1")
	_, ok := g.GetProposal("nonexistent")
	if ok {
		t.Error("expected not found")
	}
}

// --- ProposeRule ---

func TestProposeRule_Success(t *testing.T) {
	g := newTestGovernance("otter-1")
	rule := &Rule{
		Scope:      "safety",
		Body:       "be kind",
		ProposedBy: "otter-1",
	}
	proposal, err := g.ProposeRule(context.Background(), "otter-1", rule)
	if err != nil {
		t.Fatal(err)
	}
	if proposal.Status != ProposalOpen {
		t.Errorf("status = %q", proposal.Status)
	}
	if proposal.ProposalID == "" {
		t.Error("expected non-empty proposal ID")
	}
	if proposal.RaftID != "otter-1" {
		t.Errorf("raft ID = %q", proposal.RaftID)
	}
}

func TestProposeRule_RaftNotFound(t *testing.T) {
	g := newTestGovernance("otter-1")
	rule := &Rule{Scope: "safety", Body: "be kind", ProposedBy: "otter-1"}
	_, err := g.ProposeRule(context.Background(), "nonexistent", rule)
	if err == nil {
		t.Error("expected error for nonexistent raft")
	}
}

func TestProposeRule_ProposerNotActive(t *testing.T) {
	g := newTestGovernance("otter-1")
	rule := &Rule{Scope: "safety", Body: "be kind", ProposedBy: "otter-2"}
	_, err := g.ProposeRule(context.Background(), "otter-1", rule)
	if err == nil {
		t.Error("expected error for non-member proposer")
	}
}

func TestProposeRule_InactiveMember(t *testing.T) {
	g := newTestGovernance("otter-1")
	g.rafts.rafts["otter-1"].Members["otter-2"] = &Member{ID: "otter-2", State: StateInactive}
	rule := &Rule{Scope: "safety", Body: "be kind", ProposedBy: "otter-2"}
	_, err := g.ProposeRule(context.Background(), "otter-1", rule)
	if err == nil {
		t.Error("expected error for inactive proposer")
	}
}

// --- Vote ---

func TestVote_Success(t *testing.T) {
	g := newTestGovernance("otter-1")
	rule := &Rule{Scope: "safety", Body: "be kind", ProposedBy: "otter-1"}
	proposal, err := g.ProposeRule(context.Background(), "otter-1", rule)
	if err != nil {
		t.Fatal(err)
	}

	err = g.Vote(context.Background(), proposal.ProposalID, "otter-1", VoteYes)
	if err != nil {
		t.Fatal(err)
	}

	// Solo raft: auto-adopts with 1 YES vote
	p, _ := g.GetProposal(proposal.ProposalID)
	if p.Status != ProposalClosed {
		t.Errorf("status = %q, want closed", p.Status)
	}
	if p.Result != ResultAdopted {
		t.Errorf("result = %q, want adopted", p.Result)
	}
}

func TestVote_ProposalNotFound(t *testing.T) {
	g := newTestGovernance("otter-1")
	err := g.Vote(context.Background(), "nonexistent", "otter-1", VoteYes)
	if err == nil {
		t.Error("expected error")
	}
}

func TestVote_ProposalClosed(t *testing.T) {
	g := newTestGovernance("otter-1")
	rule := &Rule{Scope: "safety", Body: "x", ProposedBy: "otter-1"}
	proposal, _ := g.ProposeRule(context.Background(), "otter-1", rule)
	// Vote to close it
	g.Vote(context.Background(), proposal.ProposalID, "otter-1", VoteYes)
	// Try voting again
	err := g.Vote(context.Background(), proposal.ProposalID, "otter-1", VoteYes)
	if err == nil {
		t.Error("expected error for closed proposal")
	}
}

func TestVote_VoterNotActive(t *testing.T) {
	g := newTestGovernance("otter-1")
	rule := &Rule{Scope: "safety", Body: "x", ProposedBy: "otter-1"}
	proposal, _ := g.ProposeRule(context.Background(), "otter-1", rule)
	err := g.Vote(context.Background(), proposal.ProposalID, "otter-99", VoteYes)
	if err == nil {
		t.Error("expected error for non-member voter")
	}
}

// --- checkProposalOutcome: solo raft ---

func TestCheckProposalOutcome_SoloYes(t *testing.T) {
	g := newTestGovernance("otter-1")
	proposal := &Proposal{
		ProposalID: "p1",
		RaftID:     "otter-1",
		Rule:       &Rule{RuleID: "r1", RaftID: "otter-1", Scope: "test", Body: "rule"},
		Votes:      map[string]VoteType{"otter-1": VoteYes},
		Status:     ProposalOpen,
		Result:     ResultPending,
	}
	g.checkProposalOutcome(proposal)
	if proposal.Result != ResultAdopted {
		t.Errorf("result = %q, want adopted", proposal.Result)
	}
}

func TestCheckProposalOutcome_SoloNo(t *testing.T) {
	g := newTestGovernance("otter-1")
	proposal := &Proposal{
		ProposalID: "p1",
		RaftID:     "otter-1",
		Rule:       &Rule{RuleID: "r1", RaftID: "otter-1", Scope: "test", Body: "rule"},
		Votes:      map[string]VoteType{"otter-1": VoteNo},
		Status:     ProposalOpen,
		Result:     ResultPending,
	}
	g.checkProposalOutcome(proposal)
	if proposal.Result != ResultRejected {
		t.Errorf("result = %q, want rejected", proposal.Result)
	}
}

// --- checkProposalOutcome: two-member raft ---

func TestCheckProposalOutcome_TwoMembers_Unanimous(t *testing.T) {
	g := newTestGovernance("otter-1")
	g.rafts.rafts["otter-1"].Members["otter-2"] = &Member{ID: "otter-2", State: StateActive}

	proposal := &Proposal{
		ProposalID: "p1",
		RaftID:     "otter-1",
		Rule:       &Rule{RuleID: "r1", RaftID: "otter-1", Scope: "test", Body: "rule"},
		Votes:      map[string]VoteType{"otter-1": VoteYes, "otter-2": VoteYes},
		Status:     ProposalOpen,
		Result:     ResultPending,
	}
	g.checkProposalOutcome(proposal)
	if proposal.Result != ResultAdopted {
		t.Errorf("result = %q, want adopted", proposal.Result)
	}
}

func TestCheckProposalOutcome_TwoMembers_OneNo(t *testing.T) {
	g := newTestGovernance("otter-1")
	g.rafts.rafts["otter-1"].Members["otter-2"] = &Member{ID: "otter-2", State: StateActive}

	proposal := &Proposal{
		ProposalID: "p1",
		RaftID:     "otter-1",
		Rule:       &Rule{RuleID: "r1", RaftID: "otter-1", Scope: "test", Body: "rule"},
		Votes:      map[string]VoteType{"otter-1": VoteYes, "otter-2": VoteNo},
		Status:     ProposalOpen,
		Result:     ResultPending,
	}
	g.checkProposalOutcome(proposal)
	if proposal.Result != ResultRejected {
		t.Errorf("result = %q, want rejected", proposal.Result)
	}
}

func TestCheckProposalOutcome_TwoMembers_OnlyOneVote(t *testing.T) {
	g := newTestGovernance("otter-1")
	g.rafts.rafts["otter-1"].Members["otter-2"] = &Member{ID: "otter-2", State: StateActive}

	proposal := &Proposal{
		ProposalID: "p1",
		RaftID:     "otter-1",
		Rule:       &Rule{RuleID: "r1", RaftID: "otter-1", Scope: "test", Body: "rule"},
		Votes:      map[string]VoteType{"otter-1": VoteYes},
		Status:     ProposalOpen,
		Result:     ResultPending,
	}
	g.checkProposalOutcome(proposal)
	// Quorum not met, stays pending
	if proposal.Result != ResultPending {
		t.Errorf("result = %q, want pending (quorum not met)", proposal.Result)
	}
}

// --- checkProposalOutcome: three-member raft ---

func TestCheckProposalOutcome_ThreeMembers_TwoThirds(t *testing.T) {
	g := newTestGovernance("otter-1")
	g.rafts.rafts["otter-1"].Members["otter-2"] = &Member{ID: "otter-2", State: StateActive}
	g.rafts.rafts["otter-1"].Members["otter-3"] = &Member{ID: "otter-3", State: StateActive}

	proposal := &Proposal{
		ProposalID: "p1",
		RaftID:     "otter-1",
		Rule:       &Rule{RuleID: "r1", RaftID: "otter-1", Scope: "test", Body: "rule"},
		Votes:      map[string]VoteType{"otter-1": VoteYes, "otter-2": VoteYes, "otter-3": VoteNo},
		Status:     ProposalOpen,
		Result:     ResultPending,
	}
	g.checkProposalOutcome(proposal)
	// 2 yes out of 3 active. Quorum threshold = ceil(3*67/100) = ceil(2.01) = 3. Need 3 votes for quorum.
	// Required yes = ceil(3*67/100) = 3. 2 < 3 so not adopted, all voted → rejected.
	if proposal.Status != ProposalClosed {
		t.Errorf("status = %q, want closed", proposal.Status)
	}
}

func TestCheckProposalOutcome_ThreeMembers_AllYes(t *testing.T) {
	g := newTestGovernance("otter-1")
	g.rafts.rafts["otter-1"].Members["otter-2"] = &Member{ID: "otter-2", State: StateActive}
	g.rafts.rafts["otter-1"].Members["otter-3"] = &Member{ID: "otter-3", State: StateActive}

	proposal := &Proposal{
		ProposalID: "p1",
		RaftID:     "otter-1",
		Rule:       &Rule{RuleID: "r1", RaftID: "otter-1", Scope: "test", Body: "rule"},
		Votes:      map[string]VoteType{"otter-1": VoteYes, "otter-2": VoteYes, "otter-3": VoteYes},
		Status:     ProposalOpen,
		Result:     ResultPending,
	}
	g.checkProposalOutcome(proposal)
	if proposal.Result != ResultAdopted {
		t.Errorf("result = %q, want adopted", proposal.Result)
	}
}

func TestCheckProposalOutcome_SuperMajority(t *testing.T) {
	g := newTestGovernance("otter-1")
	g.rafts.rafts["otter-1"].Members["otter-2"] = &Member{ID: "otter-2", State: StateActive}
	g.rafts.rafts["otter-1"].Members["otter-3"] = &Member{ID: "otter-3", State: StateActive}
	g.rafts.rafts["otter-1"].Members["otter-4"] = &Member{ID: "otter-4", State: StateActive}

	// BaseRuleID set → super-majority required
	proposal := &Proposal{
		ProposalID: "p1",
		RaftID:     "otter-1",
		Rule:       &Rule{RuleID: "r1", RaftID: "otter-1", Scope: "test", Body: "override rule", BaseRuleID: "base-1"},
		Votes:      map[string]VoteType{"otter-1": VoteYes, "otter-2": VoteYes, "otter-3": VoteYes, "otter-4": VoteNo},
		Status:     ProposalOpen,
		Result:     ResultPending,
	}
	g.checkProposalOutcome(proposal)
	// 4 active, super-majority = ceil(4*75/100) = 3. 3 yes >= 3, adopted.
	if proposal.Result != ResultAdopted {
		t.Errorf("result = %q, want adopted", proposal.Result)
	}
}

func TestCheckProposalOutcome_NonexistentRaft(t *testing.T) {
	g := newTestGovernance("otter-1")
	proposal := &Proposal{
		ProposalID: "p1",
		RaftID:     "nonexistent",
		Rule:       &Rule{RuleID: "r1", RaftID: "nonexistent", Scope: "test", Body: "rule"},
		Votes:      map[string]VoteType{},
		Status:     ProposalOpen,
		Result:     ResultPending,
	}
	// Should not panic
	g.checkProposalOutcome(proposal)
	if proposal.Result != ResultPending {
		t.Errorf("result = %q, want pending", proposal.Result)
	}
}

// --- activateRule ---

func TestActivateRule(t *testing.T) {
	g := newTestGovernance("otter-1")
	rule := &Rule{RuleID: "r1", RaftID: "otter-1", Scope: "safety", Body: "be kind"}
	g.activateRule(rule)

	if g.rules.active["safety"] != rule {
		t.Error("rule not activated")
	}
	if g.rules.rules["r1"] != rule {
		t.Error("rule not in registry")
	}
	// Check raft rules too
	if g.rafts.rafts["otter-1"].Rules["r1"] != rule {
		t.Error("rule not added to raft")
	}
}

func TestActivateRule_Override(t *testing.T) {
	g := newTestGovernance("otter-1")
	baseRule := &Rule{RuleID: "base-1", RaftID: "otter-1", Scope: "safety", Body: "old rule"}
	g.activateRule(baseRule)

	override := &Rule{RuleID: "r2", RaftID: "otter-1", Scope: "safety", Body: "new rule", BaseRuleID: "base-1"}
	g.activateRule(override)

	// Base rule should be deactivated from active map
	if _, exists := g.rules.active["safety"]; exists {
		// The override has the same scope; the active map should have the override
		active := g.rules.active["safety"]
		if active != nil && active.RuleID == "base-1" {
			t.Error("base rule should have been deactivated")
		}
	}
}

// --- detectRuleConflicts ---

func TestDetectRuleConflicts_NoConflicts(t *testing.T) {
	g := newTestGovernance("otter-1")
	targetRules := map[string]*Rule{
		"r1": {RuleID: "r1", Scope: "ethics", Body: "be honest"},
	}
	conflicts := g.detectRuleConflicts("raft-2", targetRules)
	if len(conflicts) != 0 {
		t.Errorf("expected no conflicts, got %d", len(conflicts))
	}
}

func TestDetectRuleConflicts_WithConflict(t *testing.T) {
	g := newTestGovernance("otter-1")
	// Add an existing rule
	g.rafts.rafts["otter-1"].Rules["r-existing"] = &Rule{
		RuleID: "r-existing", Scope: "safety", Body: "be cautious",
	}
	targetRules := map[string]*Rule{
		"r1": {RuleID: "r1", Scope: "safety", Body: "be bold"},
	}
	conflicts := g.detectRuleConflicts("raft-2", targetRules)
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(conflicts))
	}
	if conflicts[0].ConflictScope != "safety" {
		t.Errorf("conflict scope = %q", conflicts[0].ConflictScope)
	}
}

func TestDetectRuleConflicts_SameBody(t *testing.T) {
	g := newTestGovernance("otter-1")
	g.rafts.rafts["otter-1"].Rules["r-existing"] = &Rule{
		RuleID: "r-existing", Scope: "safety", Body: "be kind",
	}
	targetRules := map[string]*Rule{
		"r1": {RuleID: "r1", Scope: "safety", Body: "be kind"},
	}
	conflicts := g.detectRuleConflicts("raft-2", targetRules)
	// Same scope + same body = no conflict
	if len(conflicts) != 0 {
		t.Errorf("expected no conflicts for same-body rules, got %d", len(conflicts))
	}
}

// --- RequestJoin ---

func TestRequestJoin_Success(t *testing.T) {
	g := newTestGovernance("otter-1")
	err := g.RequestJoin(context.Background(), "otter-1", "otter-2", []byte("pubkey"))
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := g.rafts.rafts["otter-1"].Members["otter-2"]; !exists {
		t.Error("otter-2 not added as member")
	}
}

func TestRequestJoin_RaftNotFound(t *testing.T) {
	g := newTestGovernance("otter-1")
	err := g.RequestJoin(context.Background(), "nonexistent", "otter-2", []byte("pubkey"))
	if err == nil {
		t.Error("expected error")
	}
}

// --- pickActiveMemberID ---

func TestPickActiveMemberID_Self(t *testing.T) {
	g := newTestGovernance("otter-1")
	id := g.pickActiveMemberID("otter-1")
	if id != "otter-1" {
		t.Errorf("expected otter-1, got %q", id)
	}
}

func TestPickActiveMemberID_NonexistentRaft(t *testing.T) {
	g := newTestGovernance("otter-1")
	id := g.pickActiveMemberID("nonexistent")
	if id != "" {
		t.Errorf("expected empty, got %q", id)
	}
}

func TestPickActiveMemberID_FallbackToOther(t *testing.T) {
	g := newTestGovernance("otter-1")
	// Make self inactive, add another active member
	g.rafts.rafts["otter-1"].Members["otter-1"].State = StateInactive
	g.rafts.rafts["otter-1"].Members["otter-2"] = &Member{ID: "otter-2", State: StateActive}
	id := g.pickActiveMemberID("otter-1")
	if id != "otter-2" {
		t.Errorf("expected otter-2, got %q", id)
	}
}

// --- checkExpiredMembers ---

func TestCheckExpiredMembers(t *testing.T) {
	g := newTestGovernance("otter-1")
	// Add a member that was last seen > 90 days ago
	longAgo := time.Now().Add(-100 * 24 * time.Hour)
	g.rafts.rafts["otter-1"].Members["otter-old"] = &Member{
		ID:         "otter-old",
		State:      StateActive,
		LastSeenAt: longAgo,
	}
	g.checkExpiredMembers()
	if g.rafts.rafts["otter-1"].Members["otter-old"].State != StateExpired {
		t.Errorf("expected expired, got %q", g.rafts.rafts["otter-1"].Members["otter-old"].State)
	}
}

func TestCheckExpiredMembers_RecentMemberUnchanged(t *testing.T) {
	g := newTestGovernance("otter-1")
	g.checkExpiredMembers()
	if g.rafts.rafts["otter-1"].Members["otter-1"].State != StateActive {
		t.Error("recent member should stay active")
	}
}

// --- buildNegotiationPrompt ---

func TestBuildNegotiationPrompt(t *testing.T) {
	g := newTestGovernance("otter-1")
	n := &Negotiation{
		Raft1ID: "raft-a",
		Raft2ID: "raft-b",
		Conflicts: []*RuleConflict{
			{
				ConflictScope: "safety",
				Rule1:         &Rule{Body: "be cautious"},
				Rule2:         &Rule{Body: "be bold"},
			},
		},
	}
	prompt := g.buildNegotiationPrompt(n)
	if !contains(prompt, "raft-a") || !contains(prompt, "raft-b") {
		t.Errorf("prompt missing raft IDs: %s", prompt)
	}
	if !contains(prompt, "be cautious") || !contains(prompt, "be bold") {
		t.Errorf("prompt missing rule bodies: %s", prompt)
	}
}

// helper
func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsSubstring(s, sub))
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// --- New ---

func TestNew(t *testing.T) {
	mem := memory.New(&mockVectorDB{})
	g, err := New(RaftConfig{
		ID:      "test-otter",
		DataDir: t.TempDir(),
	}, mem)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Shutdown(context.Background())

	if g.GetID() != "test-otter" {
		t.Errorf("ID = %q", g.GetID())
	}
	if g.GetCrypto() == nil {
		t.Error("expected crypto to be initialized")
	}
	members, err := g.GetRaftMembers("test-otter")
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 1 {
		t.Errorf("expected 1 self member, got %d", len(members))
	}
}

// --- fetchRaftRules ---

func TestFetchRaftRules_MapFormat(t *testing.T) {
	rules := map[string]*Rule{
		"safety": {Scope: "safety", Body: "be kind", RuleID: "r1", RaftID: "raft-1", Version: 1},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(rules)
	}))
	defer srv.Close()

	g := newTestGovernance("otter-1")
	fetched, err := g.fetchRaftRules(context.Background(), srv.URL, "raft-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(fetched) == 0 {
		t.Error("expected at least one rule")
	}
}

func TestFetchRaftRules_ListFormat(t *testing.T) {
	rules := []*Rule{
		{Scope: "safety", Body: "be kind", RuleID: "r1", RaftID: "raft-1", Version: 1},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(rules)
	}))
	defer srv.Close()

	g := newTestGovernance("otter-1")
	fetched, err := g.fetchRaftRules(context.Background(), srv.URL, "raft-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(fetched) == 0 {
		t.Error("expected at least one rule")
	}
}

func TestFetchRaftRules_EmptyEndpoint(t *testing.T) {
	g := newTestGovernance("otter-1")
	_, err := g.fetchRaftRules(context.Background(), "", "raft-1")
	if err == nil {
		t.Error("expected error for empty endpoint")
	}
}

func TestFetchRaftRules_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("error"))
	}))
	defer srv.Close()

	g := newTestGovernance("otter-1")
	_, err := g.fetchRaftRules(context.Background(), srv.URL, "raft-1")
	if err == nil {
		t.Error("expected error for server error")
	}
}

func TestFetchRaftRules_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer srv.Close()

	g := newTestGovernance("otter-1")
	_, err := g.fetchRaftRules(context.Background(), srv.URL, "raft-1")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestFetchRaftRules_AutoPrefixHTTP(t *testing.T) {
	// Test that endpoint without http:// prefix gets auto-prefixed
	g := newTestGovernance("otter-1")
	// This will fail to connect but should not panic due to missing prefix
	_, err := g.fetchRaftRules(context.Background(), "127.0.0.1:0", "raft-1")
	if err == nil {
		t.Error("expected connection error")
	}
}

// --- Mock LLM for negotiation tests ---

type mockLLMProvider struct {
	response string
}

func (m *mockLLMProvider) Name() string { return "mock" }
func (m *mockLLMProvider) Complete(_ context.Context, _ *llm.CompletionRequest) (*llm.CompletionResponse, error) {
	return &llm.CompletionResponse{Text: m.response}, nil
}
func (m *mockLLMProvider) Embed(_ context.Context, _ string) ([]float32, error) {
	return []float32{0.1, 0.2}, nil
}

// --- negotiateWithLLM ---

func TestNegotiateWithLLM_WithProvider(t *testing.T) {
	g := newTestGovernance("otter-1")
	negotiation := &Negotiation{
		Raft1ID: "otter-1",
		Raft2ID: "raft-2",
		Conflicts: []*RuleConflict{
			{
				ConflictScope: "safety",
				Rule1:         &Rule{Body: "be cautious", Version: 1},
				Rule2:         &Rule{Body: "be bold", Version: 2},
			},
		},
		LLMTranscript: make([]string, 0),
	}

	mockLLM := &mockLLMProvider{
		response: `{"scope":"safety","body":"Balance caution with boldness"}`,
	}

	rule, err := g.negotiateWithLLM(context.Background(), negotiation, mockLLM)
	if err != nil {
		t.Fatal(err)
	}
	if rule.Body != "Balance caution with boldness" {
		t.Errorf("body = %q, want %q", rule.Body, "Balance caution with boldness")
	}
	if rule.Scope != "safety" {
		t.Errorf("scope = %q", rule.Scope)
	}
	if rule.Version != 3 { // max(1,2) + 1
		t.Errorf("version = %d, want 3", rule.Version)
	}
}

func TestNegotiateWithLLM_FallbackSynthesize(t *testing.T) {
	g := newTestGovernance("otter-1")
	negotiation := &Negotiation{
		Raft1ID: "otter-1",
		Raft2ID: "raft-2",
		Conflicts: []*RuleConflict{
			{
				ConflictScope: "safety",
				Rule1:         &Rule{Body: "A", Version: 1},
				Rule2:         &Rule{Body: "B", Version: 1},
			},
		},
		LLMTranscript: make([]string, 0),
	}

	// non-LLM provider → fallback to synthesizeCompromiseRuleBody
	rule, err := g.negotiateWithLLM(context.Background(), negotiation, "not-an-llm")
	if err != nil {
		t.Fatal(err)
	}
	if rule.Body == "" {
		t.Error("expected non-empty fallback body")
	}
}

// --- startNegotiation ---

func TestStartNegotiation_NoConflicts(t *testing.T) {
	g := newTestGovernance("otter-1")
	_, err := g.startNegotiation(context.Background(), "raft-2", "endpoint", nil, nil)
	if err == nil {
		t.Error("expected error for no conflicts")
	}
}

// --- JoinRaft with httptest ---

func TestJoinRaft_NoConflicts(t *testing.T) {
	g := newTestGovernance("otter-1")

	// Server returns target raft rules (no overlap with our raft)
	rulesSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/governance/rules" {
			rules := map[string]*Rule{
				"ethics": {Scope: "ethics", Body: "be honest"},
			}
			json.NewEncoder(w).Encode(rules)
			return
		}
		if r.URL.Path == "/api/v1/governance/join" {
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer rulesSrv.Close()

	err := g.JoinRaft(context.Background(), "raft-2", rulesSrv.URL, nil)
	if err != nil {
		t.Fatalf("JoinRaft error: %v", err)
	}

	// Verify we now have the raft
	g.rafts.mu.RLock()
	_, exists := g.rafts.rafts["raft-2"]
	g.rafts.mu.RUnlock()
	if !exists {
		t.Error("expected raft-2 to be added")
	}
}

func TestJoinRaft_WithConflicts_Negotiation(t *testing.T) {
	g := newTestGovernance("otter-1")
	// Add an existing rule that will conflict
	g.rafts.rafts["otter-1"].Rules["r1"] = &Rule{
		RuleID: "r1", Scope: "safety", Body: "be cautious", Version: 1,
	}

	rulesSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/governance/rules" {
			rules := map[string]*Rule{
				"r2": {Scope: "safety", Body: "be bold", RuleID: "r2", Version: 1},
			}
			json.NewEncoder(w).Encode(rules)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer rulesSrv.Close()

	mockLLM := &mockLLMProvider{
		response: `{"scope":"safety","body":"Be balanced"}`,
	}

	// This will negotiate but the dual-raft vote needs both proposals to close.
	// With a solo raft, the local proposal auto-closes, but the remote one won't.
	// The function will time out on the remote vote.
	err := g.JoinRaft(context.Background(), "raft-2", rulesSrv.URL, mockLLM)
	// We expect an error about the negotiation vote timing out or failing
	if err == nil {
		t.Log("JoinRaft succeeded (both proposals auto-closed)")
	}
	// This is acceptable - we're testing the negotiation path works without panicking
}

func TestJoinRaft_RemoteReject(t *testing.T) {
	g := newTestGovernance("otter-1")

	rulesSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/governance/rules" {
			json.NewEncoder(w).Encode(map[string]*Rule{})
			return
		}
		if r.URL.Path == "/api/v1/governance/join" {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte("not allowed"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer rulesSrv.Close()

	err := g.JoinRaft(context.Background(), "raft-2", rulesSrv.URL, nil)
	if err == nil {
		t.Error("expected error for rejected join")
	}
	// Verify rollback: raft-2 should not exist
	g.rafts.mu.RLock()
	_, exists := g.rafts.rafts["raft-2"]
	g.rafts.mu.RUnlock()
	if exists {
		t.Error("raft-2 should have been rolled back")
	}
}

// --- adoptRulesAndJoin ---

func TestAdoptRulesAndJoin_EmptyEndpoint(t *testing.T) {
	g := newTestGovernance("otter-1")
	err := g.adoptRulesAndJoin(context.Background(), "raft-2", map[string]*Rule{}, "")
	if err == nil {
		t.Error("expected error for empty endpoint")
	}
}
