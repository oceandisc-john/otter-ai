package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"otter-ai/internal/agent"
	"otter-ai/internal/config"
	"otter-ai/internal/governance"
	"otter-ai/internal/llm"
	"otter-ai/internal/memory"
	"otter-ai/internal/vectordb"
)

// --- respondJSON ---

func TestRespondJSON(t *testing.T) {
	w := httptest.NewRecorder()
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})

	if w.Code != http.StatusOK {
		t.Errorf("status = %d", w.Code)
	}
	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q", w.Header().Get("Content-Type"))
	}

	var body map[string]string
	json.NewDecoder(w.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Errorf("body = %v", body)
	}
}

// --- respondError ---

func TestRespondError(t *testing.T) {
	w := httptest.NewRecorder()
	respondError(w, http.StatusBadRequest, "bad input")

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d", w.Code)
	}

	var body map[string]string
	json.NewDecoder(w.Body).Decode(&body)
	if body["error"] != "bad input" {
		t.Errorf("error = %q", body["error"])
	}
}

// --- corsMiddleware ---

func TestCorsMiddleware(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := corsMiddleware(inner)

	// Normal request
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("missing CORS origin header")
	}
	if w.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Error("missing CORS methods header")
	}
	if w.Code != http.StatusOK {
		t.Errorf("status = %d", w.Code)
	}
}

func TestCorsMiddleware_Preflight(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("inner handler should not be called for OPTIONS")
	})

	handler := corsMiddleware(inner)

	req := httptest.NewRequest("OPTIONS", "/api/v1/chat", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d; want 200", w.Code)
	}
}

// --- handleHealth (doesn't depend on agent) ---
// We test it via a direct function call since it doesn't depend on agent state.

func TestHandleHealth(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	s.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d", w.Code)
	}

	var body map[string]string
	json.NewDecoder(w.Body).Decode(&body)
	if body["status"] != "healthy" {
		t.Errorf("status = %q", body["status"])
	}
	if body["time"] == "" {
		t.Error("time should be set")
	}
}

// --- Mock VectorDB ---

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

// --- Mock LLM Provider ---

type mockLLMProvider struct {
	completeResp string
	completeErr  error
	embedResp    []float32
}

func (m *mockLLMProvider) Name() string { return "mock" }
func (m *mockLLMProvider) Complete(_ context.Context, _ *llm.CompletionRequest) (*llm.CompletionResponse, error) {
	if m.completeErr != nil {
		return nil, m.completeErr
	}
	return &llm.CompletionResponse{Text: m.completeResp}, nil
}
func (m *mockLLMProvider) Embed(_ context.Context, _ string) ([]float32, error) {
	if m.embedResp != nil {
		return m.embedResp, nil
	}
	return []float32{0.1, 0.2, 0.3}, nil
}

// newTestServer creates a Server with a real Agent backed by mocks (no governance).
func newTestServer(passphrase string) *Server {
	mem := memory.New(&mockVectorDB{})
	mockLLM := &mockLLMProvider{
		completeResp: "mock response",
		embedResp:    []float32{0.1, 0.2, 0.3},
	}

	ag := agent.New(agent.Config{
		Memory: mem,
		LLM:    mockLLM,
	})

	apiCfg := config.APIConfig{
		Port:            0,
		Host:            "localhost",
		Passphrase:      passphrase,
		RateLimit:       100,
		RateLimitWindow: time.Minute,
	}

	return NewServer(apiCfg, ag)
}

// --- handleAuth ---

func TestHandleAuth_NoPassphrase(t *testing.T) {
	s := newTestServer("")
	body := `{"passphrase": ""}`
	req := httptest.NewRequest("POST", "/api/v1/auth", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleAuth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d", w.Code)
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["authenticated"] != true {
		t.Error("expected authenticated=true")
	}
}

func TestHandleAuth_ValidPassphrase(t *testing.T) {
	s := newTestServer("secret123")
	body := `{"passphrase": "secret123"}`
	req := httptest.NewRequest("POST", "/api/v1/auth", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleAuth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d", w.Code)
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["token"] == "" {
		t.Error("expected non-empty token")
	}
}

func TestHandleAuth_InvalidPassphrase(t *testing.T) {
	s := newTestServer("secret123")
	body := `{"passphrase": "wrong"}`
	req := httptest.NewRequest("POST", "/api/v1/auth", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleAuth(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestHandleAuth_InvalidBody(t *testing.T) {
	s := newTestServer("secret123")
	req := httptest.NewRequest("POST", "/api/v1/auth", strings.NewReader("not json"))
	w := httptest.NewRecorder()
	s.handleAuth(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// --- requireAuth middleware ---

func TestRequireAuth_NoPassphrase(t *testing.T) {
	s := newTestServer("")
	called := false
	handler := s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if !called {
		t.Error("handler should be called when no passphrase configured")
	}
}

func TestRequireAuth_ValidToken(t *testing.T) {
	s := newTestServer("secret123")
	token, _ := s.jwtManager.GenerateToken("test-user")

	called := false
	handler := s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler(w, req)

	if !called {
		t.Error("handler should be called with valid token")
	}
}

func TestRequireAuth_InvalidToken(t *testing.T) {
	s := newTestServer("secret123")
	handler := s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestRequireAuth_MissingHeader(t *testing.T) {
	s := newTestServer("secret123")
	handler := s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestRequireAuth_BadFormat(t *testing.T) {
	s := newTestServer("secret123")
	handler := s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Basic abc123")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

// --- handleChat ---

func TestHandleChat_EmptyMessage(t *testing.T) {
	s := newTestServer("")
	body := `{"message": ""}`
	req := httptest.NewRequest("POST", "/api/v1/chat", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleChat(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleChat_TooLong(t *testing.T) {
	s := newTestServer("")
	longMsg := strings.Repeat("x", 10001)
	body, _ := json.Marshal(map[string]string{"message": longMsg})
	req := httptest.NewRequest("POST", "/api/v1/chat", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleChat(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleChat_InvalidBody(t *testing.T) {
	s := newTestServer("")
	req := httptest.NewRequest("POST", "/api/v1/chat", strings.NewReader("not json"))
	w := httptest.NewRecorder()
	s.handleChat(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleChat_Success(t *testing.T) {
	s := newTestServer("")
	body := `{"message": "hello"}`
	req := httptest.NewRequest("POST", "/api/v1/chat", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleChat(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["response"] == "" {
		t.Error("expected non-empty response")
	}
}

// --- handleClearChat ---

func TestHandleClearChat(t *testing.T) {
	s := newTestServer("")
	req := httptest.NewRequest("POST", "/api/v1/chat/clear", nil)
	w := httptest.NewRecorder()
	s.handleClearChat(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// --- handleListMemories ---

func TestHandleListMemories(t *testing.T) {
	s := newTestServer("")
	req := httptest.NewRequest("GET", "/api/v1/memories", nil)
	w := httptest.NewRecorder()
	s.handleListMemories(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestHandleListMemories_WithType(t *testing.T) {
	s := newTestServer("")
	req := httptest.NewRequest("GET", "/api/v1/memories?type=musing", nil)
	w := httptest.NewRecorder()
	s.handleListMemories(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// --- handleListRules ---

func TestHandleListRules(t *testing.T) {
	s := newTestServerWithGov(t)
	req := httptest.NewRequest("GET", "/api/v1/governance/rules", nil)
	w := httptest.NewRecorder()
	s.handleListRules(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// --- handleProposeRule ---

func TestHandleProposeRule_Success(t *testing.T) {
	s := newTestServerWithGov(t)
	otterID := s.agent.GetGovernance().GetID()
	body, _ := json.Marshal(map[string]string{
		"scope":       "safety",
		"body":        "be kind",
		"proposed_by": otterID,
	})
	req := httptest.NewRequest("POST", "/api/v1/governance/rules", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleProposeRule(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201, body: %s", w.Code, w.Body.String())
	}
}

func TestHandleProposeRule_MissingFields(t *testing.T) {
	s := newTestServerWithGov(t)
	body := `{"scope": "safety"}`
	req := httptest.NewRequest("POST", "/api/v1/governance/rules", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleProposeRule(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleProposeRule_BodyTooLong(t *testing.T) {
	s := newTestServerWithGov(t)
	longBody := strings.Repeat("x", 1001)
	body, _ := json.Marshal(map[string]string{
		"scope":       "safety",
		"body":        longBody,
		"proposed_by": "otter-1",
	})
	req := httptest.NewRequest("POST", "/api/v1/governance/rules", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleProposeRule(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleProposeRule_ScopeTooLong(t *testing.T) {
	s := newTestServerWithGov(t)
	longScope := strings.Repeat("x", 101)
	body, _ := json.Marshal(map[string]string{
		"scope":       longScope,
		"body":        "rule",
		"proposed_by": "otter-1",
	})
	req := httptest.NewRequest("POST", "/api/v1/governance/rules", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleProposeRule(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// --- handleVote ---

func TestHandleVote_MissingFields(t *testing.T) {
	s := newTestServerWithGov(t)
	body := `{"proposal_id": "p1"}`
	req := httptest.NewRequest("POST", "/api/v1/governance/vote", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleVote(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleVote_InvalidVoteType(t *testing.T) {
	s := newTestServerWithGov(t)
	body := `{"proposal_id": "p1", "voter_id": "v1", "vote": "MAYBE"}`
	req := httptest.NewRequest("POST", "/api/v1/governance/vote", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleVote(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// --- handleJoinRaft ---

func TestHandleJoinRaft_MissingFields(t *testing.T) {
	s := newTestServerWithGov(t)
	body := `{"raft_id": "r1"}`
	req := httptest.NewRequest("POST", "/api/v1/governance/join", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleJoinRaft(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleJoinRaft_InvalidHex(t *testing.T) {
	s := newTestServerWithGov(t)
	body := `{"raft_id": "r1", "requester_id": "o2", "public_key": "not-hex"}`
	req := httptest.NewRequest("POST", "/api/v1/governance/join", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleJoinRaft(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleJoinRaft_Success(t *testing.T) {
	s := newTestServerWithGov(t)
	raftID := s.agent.GetGovernance().GetID()
	body, _ := json.Marshal(map[string]string{
		"raft_id":      raftID,
		"requester_id": "new-otter",
		"public_key":   "abcdef1234567890",
	})
	req := httptest.NewRequest("POST", "/api/v1/governance/join", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleJoinRaft(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200, body: %s", w.Code, w.Body.String())
	}
}

// --- handleListMembers ---

func TestHandleListMembers(t *testing.T) {
	s := newTestServerWithGov(t)
	req := httptest.NewRequest("GET", "/api/v1/governance/members", nil)
	w := httptest.NewRecorder()
	s.handleListMembers(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestHandleListMembers_WithRaftID(t *testing.T) {
	s := newTestServerWithGov(t)
	raftID := s.agent.GetGovernance().GetID()
	req := httptest.NewRequest("GET", "/api/v1/governance/members?raft_id="+raftID, nil)
	w := httptest.NewRecorder()
	s.handleListMembers(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d", w.Code)
	}
}

func TestHandleListMembers_NonexistentRaft(t *testing.T) {
	s := newTestServerWithGov(t)
	req := httptest.NewRequest("GET", "/api/v1/governance/members?raft_id=nonexistent", nil)
	w := httptest.NewRecorder()
	s.handleListMembers(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

// newTestServerWithGov creates a test server with real governance (uses t.TempDir).
func newTestServerWithGov(t *testing.T) *Server {
	t.Helper()
	vdb := &mockVectorDB{}
	mem := memory.New(vdb)
	mockLLM := &mockLLMProvider{
		completeResp: "mock response",
		embedResp:    []float32{0.1, 0.2, 0.3},
	}

	gov, err := governance.New(governance.RaftConfig{
		ID:      "test-otter",
		DataDir: t.TempDir(),
	}, mem)
	if err != nil {
		t.Fatal(err)
	}

	ag := agent.New(agent.Config{
		Memory:     mem,
		LLM:        mockLLM,
		Governance: gov,
	})

	apiCfg := config.APIConfig{
		Port:            0,
		Host:            "localhost",
		Passphrase:      "",
		RateLimit:       100,
		RateLimitWindow: time.Minute,
	}

	return NewServer(apiCfg, ag)
}
