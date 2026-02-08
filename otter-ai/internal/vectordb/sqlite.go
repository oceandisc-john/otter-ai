package vectordb

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"

	_ "github.com/mattn/go-sqlite3"
)

// SQLiteVectorDB implements VectorDB using SQLite with vector extensions
type SQLiteVectorDB struct {
	db *sql.DB
}

// NewSQLiteVectorDB creates a new SQLite-based vector database
func NewSQLiteVectorDB(dbPath string) (*SQLiteVectorDB, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	vdb := &SQLiteVectorDB{db: db}

	// Initialize tables
	if err := vdb.initTables(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize tables: %w", err)
	}

	return vdb, nil
}

// initTables creates the necessary tables
func (v *SQLiteVectorDB) initTables() error {
	tables := []string{TableMemories, TableMusings, TablePersonality}

	for _, table := range tables {
		query := fmt.Sprintf(`
			CREATE TABLE IF NOT EXISTS %s (
				id TEXT PRIMARY KEY,
				vector TEXT NOT NULL,
				metadata TEXT,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
			)
		`, table)

		if _, err := v.db.Exec(query); err != nil {
			return fmt.Errorf("failed to create table %s: %w", table, err)
		}

		// Create index on created_at
		indexQuery := fmt.Sprintf(`
			CREATE INDEX IF NOT EXISTS idx_%s_created_at ON %s(created_at)
		`, table, table)

		if _, err := v.db.Exec(indexQuery); err != nil {
			return fmt.Errorf("failed to create index on %s: %w", table, err)
		}
	}

	// Create governance tables
	if err := v.initGovernanceTables(); err != nil {
		return err
	}

	return nil
}

// initGovernanceTables creates tables for governance persistence
func (v *SQLiteVectorDB) initGovernanceTables() error {
	// Raft memberships table
	_, err := v.db.Exec(`
		CREATE TABLE IF NOT EXISTS governance_rafts (
			raft_id TEXT PRIMARY KEY,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create governance_rafts table: %w", err)
	}

	// Raft members table
	_, err = v.db.Exec(`
		CREATE TABLE IF NOT EXISTS governance_members (
			raft_id TEXT NOT NULL,
			member_id TEXT NOT NULL,
			state TEXT NOT NULL,
			joined_at INTEGER NOT NULL,
			last_seen_at INTEGER NOT NULL,
			public_key BLOB,
			signature BLOB,
			inducted_by TEXT NOT NULL,
			expires_at INTEGER,
			PRIMARY KEY (raft_id, member_id),
			FOREIGN KEY (raft_id) REFERENCES governance_rafts(raft_id)
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create governance_members table: %w", err)
	}

	// Rules table
	_, err = v.db.Exec(`
		CREATE TABLE IF NOT EXISTS governance_rules (
			rule_id TEXT PRIMARY KEY,
			raft_id TEXT NOT NULL,
			scope TEXT NOT NULL,
			version INTEGER NOT NULL,
			timestamp INTEGER NOT NULL,
			body TEXT NOT NULL,
			base_rule_id TEXT,
			signature BLOB,
			proposed_by TEXT NOT NULL,
			adopted_at INTEGER,
			FOREIGN KEY (raft_id) REFERENCES governance_rafts(raft_id)
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create governance_rules table: %w", err)
	}

	// Create indices for faster lookups
	indices := []string{
		"CREATE INDEX IF NOT EXISTS idx_members_raft ON governance_members(raft_id)",
		"CREATE INDEX IF NOT EXISTS idx_rules_raft ON governance_rules(raft_id)",
		"CREATE INDEX IF NOT EXISTS idx_rules_scope ON governance_rules(scope)",
	}

	for _, indexQuery := range indices {
		if _, err := v.db.Exec(indexQuery); err != nil {
			return fmt.Errorf("failed to create governance index: %w", err)
		}
	}

	return nil
}

// Store stores a vector with metadata
func (v *SQLiteVectorDB) Store(ctx context.Context, table string, id string, vector []float32, metadata map[string]interface{}) error {
	if err := ValidateTable(table); err != nil {
		return err
	}

	vectorJSON, err := json.Marshal(vector)
	if err != nil {
		return fmt.Errorf("failed to marshal vector: %w", err)
	}

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	query := fmt.Sprintf(`
		INSERT OR REPLACE INTO %s (id, vector, metadata, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
	`, table)

	_, err = v.db.ExecContext(ctx, query, id, string(vectorJSON), string(metadataJSON))
	if err != nil {
		return fmt.Errorf("failed to store vector: %w", err)
	}

	return nil
}

// Search searches for similar vectors using cosine similarity
func (v *SQLiteVectorDB) Search(ctx context.Context, table string, queryVector []float32, limit int) ([]SearchResult, error) {
	if err := ValidateTable(table); err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`
		SELECT id, vector, metadata FROM %s
	`, table)

	rows, err := v.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query vectors: %w", err)
	}
	defer rows.Close()

	var results []SearchResult

	for rows.Next() {
		var id, vectorStr, metadataStr string
		if err := rows.Scan(&id, &vectorStr, &metadataStr); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		var vector []float32
		if err := json.Unmarshal([]byte(vectorStr), &vector); err != nil {
			continue // Skip invalid vectors
		}

		var metadata map[string]interface{}
		if err := json.Unmarshal([]byte(metadataStr), &metadata); err != nil {
			metadata = make(map[string]interface{})
		}

		// Calculate cosine similarity
		score := cosineSimilarity(queryVector, vector)

		results = append(results, SearchResult{
			ID:       id,
			Score:    score,
			Metadata: metadata,
			Vector:   vector,
		})
	}

	// Sort by score descending
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Score > results[i].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	// Return top results
	if limit > 0 && limit < len(results) {
		results = results[:limit]
	}

	return results, nil
}

// Get retrieves a record by ID
func (v *SQLiteVectorDB) Get(ctx context.Context, table string, id string) (*Record, error) {
	if err := ValidateTable(table); err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`
		SELECT id, vector, metadata FROM %s WHERE id = ?
	`, table)

	var vectorStr, metadataStr string
	err := v.db.QueryRowContext(ctx, query, id).Scan(&id, &vectorStr, &metadataStr)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("record not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get record: %w", err)
	}

	var vector []float32
	if err := json.Unmarshal([]byte(vectorStr), &vector); err != nil {
		return nil, fmt.Errorf("failed to unmarshal vector: %w", err)
	}

	var metadata map[string]interface{}
	if err := json.Unmarshal([]byte(metadataStr), &metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	return &Record{
		ID:       id,
		Vector:   vector,
		Metadata: metadata,
	}, nil
}

// Delete removes a record by ID
func (v *SQLiteVectorDB) Delete(ctx context.Context, table string, id string) error {
	if err := ValidateTable(table); err != nil {
		return err
	}

	query := fmt.Sprintf(`DELETE FROM %s WHERE id = ?`, table)
	_, err := v.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete record: %w", err)
	}

	return nil
}

// List retrieves records with pagination
func (v *SQLiteVectorDB) List(ctx context.Context, table string, limit, offset int) ([]Record, error) {
	if err := ValidateTable(table); err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`
		SELECT id, vector, metadata FROM %s
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, table)

	rows, err := v.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list records: %w", err)
	}
	defer rows.Close()

	var records []Record

	for rows.Next() {
		var id, vectorStr, metadataStr string
		if err := rows.Scan(&id, &vectorStr, &metadataStr); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		var vector []float32
		if err := json.Unmarshal([]byte(vectorStr), &vector); err != nil {
			continue
		}

		var metadata map[string]interface{}
		if err := json.Unmarshal([]byte(metadataStr), &metadata); err != nil {
			metadata = make(map[string]interface{})
		}

		records = append(records, Record{
			ID:       id,
			Vector:   vector,
			Metadata: metadata,
		})
	}

	return records, nil
}

// Close closes the database connection
func (v *SQLiteVectorDB) Close() error {
	return v.db.Close()
}

// GetDB returns the underlying database connection for direct queries
// This is used by other internal packages like governance for persistence
func (v *SQLiteVectorDB) GetDB() *sql.DB {
	return v.db
}

// cosineSimilarity calculates cosine similarity between two vectors
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}

	var dotProduct, magA, magB float64

	for i := 0; i < len(a); i++ {
		dotProduct += float64(a[i]) * float64(b[i])
		magA += float64(a[i]) * float64(a[i])
		magB += float64(b[i]) * float64(b[i])
	}

	magA = math.Sqrt(magA)
	magB = math.Sqrt(magB)

	if magA == 0 || magB == 0 {
		return 0
	}

	return dotProduct / (magA * magB)
}
