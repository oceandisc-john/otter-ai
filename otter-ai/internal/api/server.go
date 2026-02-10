package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"otter-ai/internal/agent"
	"otter-ai/internal/config"
	"otter-ai/internal/governance"
	"otter-ai/internal/memory"
)

// Constants for API server configuration
const (
	ServerReadTimeout  = 30 * time.Second
	ServerWriteTimeout = 150 * time.Second // Allow enough time for LLM API calls (120s) + buffer
	ServerIdleTimeout  = 60 * time.Second
)

// Server is the REST API server
type Server struct {
	config      config.APIConfig
	agent       *agent.Agent
	server      *http.Server
	jwtManager  *JWTManager
	rateLimiter *RateLimiter
}

// NewServer creates a new API server
func NewServer(cfg config.APIConfig, agent *agent.Agent) *Server {
	// Initialize JWT manager
	jwtManager, err := NewJWTManager(cfg.JWTSecret)
	if err != nil {
		log.Printf("Warning: Failed to initialize JWT manager: %v", err)
	}

	// Initialize rate limiter
	rateLimiter := NewRateLimiter(cfg.RateLimit, cfg.RateLimitWindow)

	return &Server{
		config:      cfg,
		agent:       agent,
		jwtManager:  jwtManager,
		rateLimiter: rateLimiter,
	}
}

// Start starts the API server
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// Health check (no auth required)
	mux.HandleFunc("/health", s.handleHealth)

	// Authentication endpoint
	mux.HandleFunc("POST /api/v1/auth", s.handleAuth)

	// Protected endpoints - require authentication
	mux.HandleFunc("POST /api/v1/chat", s.requireAuth(s.handleChat))
	mux.HandleFunc("POST /api/v1/chat/clear", s.requireAuth(s.handleClearChat))
	mux.HandleFunc("GET /api/v1/memories", s.requireAuth(s.handleListMemories))
	mux.HandleFunc("GET /api/v1/governance/rules", s.requireAuth(s.handleListRules))
	mux.HandleFunc("POST /api/v1/governance/rules", s.requireAuth(s.handleProposeRule))
	mux.HandleFunc("POST /api/v1/governance/vote", s.requireAuth(s.handleVote))
	mux.HandleFunc("GET /api/v1/governance/members", s.requireAuth(s.handleListMembers))

	// Apply middleware chain: rate limiting -> CORS
	handler := corsMiddleware(s.rateLimiter.Middleware(mux))

	s.server = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", s.config.Host, s.config.Port),
		Handler:      handler,
		ReadTimeout:  ServerReadTimeout,
		WriteTimeout: ServerWriteTimeout,
		IdleTimeout:  ServerIdleTimeout,
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

	if len(req.Message) > 10000 {
		respondError(w, http.StatusBadRequest, "message too long (max 10000 characters)")
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

// handleClearChat clears the conversation history
func (s *Server) handleClearChat(w http.ResponseWriter, r *http.Request) {
	s.agent.ClearConversation()
	respondJSON(w, http.StatusOK, map[string]string{
		"message": "Conversation history cleared",
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

// Memories and musings can only be created/modified by the otter agent internally.
// No public API endpoints are provided for creating or deleting memories.

// handleListRules handles listing active governance rules
func (s *Server) handleListRules(w http.ResponseWriter, r *http.Request) {
	rules := s.agent.GetGovernance().GetActiveRules()
	respondJSON(w, http.StatusOK, rules)
}

// handleProposeRule handles proposing a new rule
func (s *Server) handleProposeRule(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RaftID     string `json:"raft_id"` // Optional: defaults to otter's own raft
		Scope      string `json:"scope"`
		Body       string `json:"body"`
		ProposedBy string `json:"proposed_by"`
		BaseRuleID string `json:"base_rule_id,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Scope == "" || req.Body == "" || req.ProposedBy == "" {
		respondError(w, http.StatusBadRequest, "scope, body, and proposed_by are required")
		return
	}

	if len(req.Body) > 1000 {
		respondError(w, http.StatusBadRequest, "rule body too long (max 1000 characters)")
		return
	}

	if len(req.Scope) > 100 {
		respondError(w, http.StatusBadRequest, "scope too long (max 100 characters)")
		return
	}

	// Default to otter's own raft if not specified
	raftID := req.RaftID
	if raftID == "" {
		raftID = s.agent.GetGovernance().GetID() // Use otter's own raft ID
	}

	rule := &governance.Rule{
		Scope:      req.Scope,
		Body:       req.Body,
		ProposedBy: req.ProposedBy,
		BaseRuleID: req.BaseRuleID,
		Timestamp:  time.Now(),
	}

	proposal, err := s.agent.GetGovernance().ProposeRule(r.Context(), raftID, rule)
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

	if req.ProposalID == "" || req.VoterID == "" || req.Vote == "" {
		respondError(w, http.StatusBadRequest, "proposal_id, voter_id, and vote are required")
		return
	}

	vote := governance.VoteType(req.Vote)
	if vote != governance.VoteYes && vote != governance.VoteNo && vote != governance.VoteAbstain {
		respondError(w, http.StatusBadRequest, "vote must be YES, NO, or ABSTAIN")
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

// handleListMembers handles listing raft members
func (s *Server) handleListMembers(w http.ResponseWriter, r *http.Request) {
	raftID := r.URL.Query().Get("raft_id")
	if raftID == "" {
		// Default to otter's own raft
		raftID = s.agent.GetGovernance().GetID()
	}

	// Get members from the specified raft
	members, err := s.agent.GetGovernance().GetRaftMembers(raftID)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	// Format members for response
	response := make([]interface{}, 0, len(members))
	for _, member := range members {
		response = append(response, map[string]interface{}{
			"id":          member.ID,
			"state":       string(member.State),
			"joined_at":   member.JoinedAt,
			"last_seen":   member.LastSeenAt,
			"inducted_by": member.InductedBy,
		})
	}

	respondJSON(w, http.StatusOK, response)
}

// handleAuth handles authentication requests
func (s *Server) handleAuth(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Passphrase string `json:"passphrase"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// If no passphrase is configured, allow access without JWT
	if s.config.Passphrase == "" {
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"authenticated": true,
			"token":         "",
		})
		return
	}

	// Validate passphrase
	if req.Passphrase != s.config.Passphrase {
		respondError(w, http.StatusUnauthorized, "invalid passphrase")
		return
	}

	// Generate JWT token
	token, err := s.jwtManager.GenerateToken("otter-user")
	if err != nil {
		log.Printf("Error generating JWT token: %v", err)
		respondError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"authenticated": true,
		"token":         token,
		"expires_in":    int(JWTExpirationTime.Seconds()),
	})
}

// requireAuth is a middleware that checks for valid authentication
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// If no passphrase is configured, allow all requests
		if s.config.Passphrase == "" {
			next(w, r)
			return
		}

		// Check Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			respondError(w, http.StatusUnauthorized, "missing authorization header")
			return
		}

		// Expect "Bearer <token>"
		const prefix = "Bearer "
		if len(authHeader) < len(prefix) || !strings.HasPrefix(authHeader, prefix) {
			respondError(w, http.StatusUnauthorized, "invalid authorization format")
			return
		}

		token := strings.TrimSpace(authHeader[len(prefix):])

		// Validate JWT token
		claims, err := s.jwtManager.ValidateToken(token)
		if err != nil {
			respondError(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}

		// Token is valid, add claims to request context if needed
		_ = claims // Can be used for user identification in the future

		next(w, r)
	}
}

// corsMiddleware adds CORS headers
// WARNING: Allows all origins (*) - restrict in production
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// TODO: Restrict to specific origins in production
		// For now, allow all for development convenience
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
