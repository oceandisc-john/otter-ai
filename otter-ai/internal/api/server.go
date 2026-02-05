package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"otter-ai/internal/agent"
	"otter-ai/internal/config"
	"otter-ai/internal/governance"
	"otter-ai/internal/memory"
)

// Server is the REST API server
type Server struct {
	config config.APIConfig
	agent  *agent.Agent
	server *http.Server
}

// NewServer creates a new API server
func NewServer(cfg config.APIConfig, agent *agent.Agent) *Server {
	return &Server{
		config: cfg,
		agent:  agent,
	}
}

// Start starts the API server
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("/health", s.handleHealth)

	// Agent endpoints
	mux.HandleFunc("POST /api/v1/chat", s.handleChat)
	mux.HandleFunc("GET /api/v1/memories", s.handleListMemories)
	mux.HandleFunc("POST /api/v1/memories", s.handleCreateMemory)
	mux.HandleFunc("DELETE /api/v1/memories/{id}", s.handleDeleteMemory)

	// Governance endpoints
	mux.HandleFunc("GET /api/v1/governance/rules", s.handleListRules)
	mux.HandleFunc("POST /api/v1/governance/rules", s.handleProposeRule)
	mux.HandleFunc("POST /api/v1/governance/vote", s.handleVote)
	mux.HandleFunc("GET /api/v1/governance/members", s.handleListMembers)

	// CORS middleware
	handler := corsMiddleware(mux)

	s.server = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", s.config.Host, s.config.Port),
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("API server listening on %s", s.server.Addr)
	return s.server.ListenAndServe()
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

// handleHealth handles health check requests
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
		"time":   time.Now().Format(time.RFC3339),
	})
}

// handleChat handles chat requests
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Message string `json:"message"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Message == "" {
		respondError(w, http.StatusBadRequest, "message is required")
		return
	}

	response, err := s.agent.ProcessMessage(r.Context(), req.Message)
	if err != nil {
		log.Printf("Error processing message: %v", err)
		respondError(w, http.StatusInternalServerError, "failed to process message")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{
		"response": response,
	})
}

// handleListMemories handles listing memories
func (s *Server) handleListMemories(w http.ResponseWriter, r *http.Request) {
	memType := r.URL.Query().Get("type")
	if memType == "" {
		memType = string(memory.MemoryTypeLongTerm)
	}

	memories, err := s.agent.GetMemory().List(r.Context(), memory.MemoryType(memType), 50, 0)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list memories")
		return
	}

	respondJSON(w, http.StatusOK, memories)
}

// handleCreateMemory handles creating a memory
func (s *Server) handleCreateMemory(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Content    string  `json:"content"`
		Type       string  `json:"type"`
		Importance float32 `json:"importance"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Generate embedding (placeholder - should use LLM)
	embedding := make([]float32, 384)

	record := &memory.MemoryRecord{
		Type:       memory.MemoryType(req.Type),
		Content:    req.Content,
		Embedding:  embedding,
		Importance: req.Importance,
	}

	if err := s.agent.GetMemory().Store(r.Context(), record); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to store memory")
		return
	}

	respondJSON(w, http.StatusCreated, map[string]string{
		"id": record.ID,
	})
}

// handleDeleteMemory handles deleting a memory
func (s *Server) handleDeleteMemory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	memType := r.URL.Query().Get("type")
	if memType == "" {
		memType = string(memory.MemoryTypeLongTerm)
	}

	if err := s.agent.GetMemory().Delete(r.Context(), id, memory.MemoryType(memType)); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to delete memory")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleListRules handles listing active governance rules
func (s *Server) handleListRules(w http.ResponseWriter, r *http.Request) {
	rules := s.agent.GetGovernance().GetActiveRules()
	respondJSON(w, http.StatusOK, rules)
}

// handleProposeRule handles proposing a new rule
func (s *Server) handleProposeRule(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Scope      string `json:"scope"`
		Body       string `json:"body"`
		ProposedBy string `json:"proposed_by"`
		BaseRuleID string `json:"base_rule_id,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	rule := &governance.Rule{
		Scope:      req.Scope,
		Body:       req.Body,
		ProposedBy: req.ProposedBy,
		BaseRuleID: req.BaseRuleID,
		Timestamp:  time.Now(),
	}

	proposal, err := s.agent.GetGovernance().ProposeRule(r.Context(), rule)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, proposal)
}

// handleVote handles voting on a proposal
func (s *Server) handleVote(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProposalID string `json:"proposal_id"`
		VoterID    string `json:"voter_id"`
		Vote       string `json:"vote"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	err := s.agent.GetGovernance().Vote(r.Context(), req.ProposalID, req.VoterID, governance.VoteType(req.Vote))
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{
		"status": "vote recorded",
	})
}

// handleListMembers handles listing raft members (stub)
func (s *Server) handleListMembers(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement member listing
	respondJSON(w, http.StatusOK, []interface{}{})
}

// corsMiddleware adds CORS headers
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// respondJSON writes a JSON response
func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// respondError writes an error response
func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{
		"error": message,
	})
}
