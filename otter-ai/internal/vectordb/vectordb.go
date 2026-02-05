package vectordb

import (
	"context"
	"fmt"
)

// VectorDB is the interface for vector database operations
type VectorDB interface {
	// Store vector with metadata
	Store(ctx context.Context, table string, id string, vector []float32, metadata map[string]interface{}) error

	// Search for similar vectors
	Search(ctx context.Context, table string, vector []float32, limit int) ([]SearchResult, error)

	// Get by ID
	Get(ctx context.Context, table string, id string) (*Record, error)

	// Delete by ID
	Delete(ctx context.Context, table string, id string) error

	// List all records in a table
	List(ctx context.Context, table string, limit, offset int) ([]Record, error)

	// Close the database connection
	Close() error
}

// SearchResult represents a search result
type SearchResult struct {
	ID       string
	Score    float64
	Metadata map[string]interface{}
	Vector   []float32
}

// Record represents a stored record
type Record struct {
	ID       string
	Vector   []float32
	Metadata map[string]interface{}
}

// Backend type
type Backend string

const (
	BackendSQLite   Backend = "sqlite"
	BackendPostgres Backend = "postgres"
	BackendDuckDB   Backend = "duckdb"
	BackendLanceDB  Backend = "lancedb"
)

// Authorized tables
const (
	TableMemories    = "memories"
	TableMusings     = "musings"
	TablePersonality = "personality"
)

// New creates a new vector database instance
func New(backend Backend, dbPath string) (VectorDB, error) {
	switch backend {
	case BackendSQLite:
		return NewSQLiteVectorDB(dbPath)
	case BackendPostgres:
		return nil, fmt.Errorf("postgres backend not yet implemented")
	case BackendDuckDB:
		return nil, fmt.Errorf("duckdb backend not yet implemented")
	case BackendLanceDB:
		return nil, fmt.Errorf("lancedb backend not yet implemented")
	default:
		return nil, fmt.Errorf("unknown backend: %s", backend)
	}
}

// ValidateTable ensures only authorized tables are used
func ValidateTable(table string) error {
	authorized := map[string]bool{
		TableMemories:    true,
		TableMusings:     true,
		TablePersonality: true,
	}

	if !authorized[table] {
		return fmt.Errorf("unauthorized table: %s", table)
	}
	return nil
}
