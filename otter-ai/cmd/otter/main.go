package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"otter-ai/internal/agent"
	"otter-ai/internal/api"
	"otter-ai/internal/config"
	"otter-ai/internal/governance"
	"otter-ai/internal/llm"
	"otter-ai/internal/memory"
	"otter-ai/internal/plugins"
	"otter-ai/internal/vectordb"
)

func main() {
	log.Println("Starting Otter-AI...")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize vector database
	vdb, err := vectordb.New(vectordb.Backend(cfg.VectorBackend), cfg.DBPath)
	if err != nil {
		log.Fatalf("Failed to initialize vector database: %v", err)
	}
	defer vdb.Close()

	// Initialize memory layer
	mem := memory.New(vdb)

	// Initialize governance
	govConfig := governance.RaftConfig{
		ID:            cfg.Raft.ID,
		Type:          governance.RaftType(cfg.Raft.Type),
		BindAddr:      cfg.Raft.BindAddr,
		AdvertiseAddr: cfg.Raft.AdvertiseAddr,
		DataDir:       cfg.Raft.DataDir,
	}

	gov, err := governance.New(govConfig, mem)
	if err != nil {
		log.Fatalf("Failed to initialize governance: %v", err)
	}

	// Initialize LLM provider
	llmProvider, err := llm.NewProvider(cfg.LLM)
	if err != nil {
		log.Fatalf("Failed to initialize LLM provider: %v", err)
	}

	// Initialize plugin manager
	pluginMgr := plugins.NewManager(cfg.Plugins)
	if err := pluginMgr.LoadAll(context.Background()); err != nil {
		log.Printf("Warning: failed to load some plugins: %v", err)
	}

	// Create agent
	ag := agent.New(agent.Config{
		Memory:     mem,
		Governance: gov,
		LLM:        llmProvider,
		Plugins:    pluginMgr,
	})

	// Start API server
	server := api.NewServer(cfg.API, ag)

	// Graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := server.Start(); err != nil {
			log.Fatalf("API server error: %v", err)
		}
	}()

	log.Println("Otter-AI is running")

	<-sigCh
	log.Println("Shutting down Otter-AI...")

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Error shutting down server: %v", err)
	}

	if err := pluginMgr.UnloadAll(ctx); err != nil {
		log.Printf("Error shutting down plugins: %v", err)
	}

	log.Println("Otter-AI stopped")
}
