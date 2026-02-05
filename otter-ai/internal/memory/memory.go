package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"otter-ai/internal/vectordb"
)

// Memory manages the agent's memory layer with bounded, auditable storage
type Memory struct {
	vectorDB vectordb.VectorDB
}

// MemoryType defines the type of memory
type MemoryType string

const (
	MemoryTypeShortTerm   MemoryType = "short_term"
	MemoryTypeLongTerm    MemoryType = "long_term"
	MemoryTypeMusing      MemoryType = "musing"
	MemoryTypePersonality MemoryType = "personality"
)

// MemoryRecord represents a memory entry
type MemoryRecord struct {
	ID         string
	Type       MemoryType
	Content    string
	Embedding  []float32
	Timestamp  time.Time
	Scope      string
	Importance float32
	Metadata   map[string]interface{}
}

// New creates a new memory layer
func New(vectorDB vectordb.VectorDB) *Memory {
	return &Memory{
		vectorDB: vectorDB,
	}
}

// Store stores a memory with its embedding
func (m *Memory) Store(ctx context.Context, record *MemoryRecord) error {
	if record.ID == "" {
		record.ID = generateMemoryID(record)
	}

	if record.Timestamp.IsZero() {
		record.Timestamp = time.Now()
	}

	table := m.getTableForType(record.Type)

	metadata := map[string]interface{}{
		"content":    record.Content,
		"timestamp":  record.Timestamp.Unix(),
		"scope":      record.Scope,
		"importance": record.Importance,
		"type":       string(record.Type),
	}

	// Merge additional metadata
	for k, v := range record.Metadata {
		metadata[k] = v
	}

	err := m.vectorDB.Store(ctx, table, record.ID, record.Embedding, metadata)
	if err != nil {
		return fmt.Errorf("failed to store memory: %w", err)
	}

	return nil
}

// Search searches for similar memories
func (m *Memory) Search(ctx context.Context, queryEmbedding []float32, memoryType MemoryType, limit int) ([]MemoryRecord, error) {
	table := m.getTableForType(memoryType)

	results, err := m.vectorDB.Search(ctx, table, queryEmbedding, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to search memories: %w", err)
	}

	var memories []MemoryRecord

	for _, result := range results {
		memory := MemoryRecord{
			ID:        result.ID,
			Embedding: result.Vector,
			Metadata:  result.Metadata,
		}

		// Extract metadata
		if content, ok := result.Metadata["content"].(string); ok {
			memory.Content = content
		}
		if ts, ok := result.Metadata["timestamp"].(float64); ok {
			memory.Timestamp = time.Unix(int64(ts), 0)
		}
		if scope, ok := result.Metadata["scope"].(string); ok {
			memory.Scope = scope
		}
		if importance, ok := result.Metadata["importance"].(float64); ok {
			memory.Importance = float32(importance)
		}
		if memType, ok := result.Metadata["type"].(string); ok {
			memory.Type = MemoryType(memType)
		}

		memories = append(memories, memory)
	}

	return memories, nil
}

// Get retrieves a memory by ID
func (m *Memory) Get(ctx context.Context, id string, memoryType MemoryType) (*MemoryRecord, error) {
	table := m.getTableForType(memoryType)

	record, err := m.vectorDB.Get(ctx, table, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get memory: %w", err)
	}

	memory := &MemoryRecord{
		ID:        record.ID,
		Embedding: record.Vector,
		Metadata:  record.Metadata,
	}

	// Extract metadata
	if content, ok := record.Metadata["content"].(string); ok {
		memory.Content = content
	}
	if ts, ok := record.Metadata["timestamp"].(float64); ok {
		memory.Timestamp = time.Unix(int64(ts), 0)
	}
	if scope, ok := record.Metadata["scope"].(string); ok {
		memory.Scope = scope
	}
	if importance, ok := record.Metadata["importance"].(float64); ok {
		memory.Importance = float32(importance)
	}
	if memType, ok := record.Metadata["type"].(string); ok {
		memory.Type = MemoryType(memType)
	}

	return memory, nil
}

// Delete removes a memory
func (m *Memory) Delete(ctx context.Context, id string, memoryType MemoryType) error {
	table := m.getTableForType(memoryType)

	err := m.vectorDB.Delete(ctx, table, id)
	if err != nil {
		return fmt.Errorf("failed to delete memory: %w", err)
	}

	return nil
}

// List retrieves memories with pagination
func (m *Memory) List(ctx context.Context, memoryType MemoryType, limit, offset int) ([]MemoryRecord, error) {
	table := m.getTableForType(memoryType)

	records, err := m.vectorDB.List(ctx, table, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list memories: %w", err)
	}

	var memories []MemoryRecord

	for _, record := range records {
		memory := MemoryRecord{
			ID:        record.ID,
			Embedding: record.Vector,
			Metadata:  record.Metadata,
		}

		// Extract metadata
		if content, ok := record.Metadata["content"].(string); ok {
			memory.Content = content
		}
		if ts, ok := record.Metadata["timestamp"].(float64); ok {
			memory.Timestamp = time.Unix(int64(ts), 0)
		}
		if scope, ok := record.Metadata["scope"].(string); ok {
			memory.Scope = scope
		}
		if importance, ok := record.Metadata["importance"].(float64); ok {
			memory.Importance = float32(importance)
		}
		if memType, ok := record.Metadata["type"].(string); ok {
			memory.Type = MemoryType(memType)
		}

		memories = append(memories, memory)
	}

	return memories, nil
}

// getTableForType maps memory type to vector database table
func (m *Memory) getTableForType(memoryType MemoryType) string {
	switch memoryType {
	case MemoryTypeMusing:
		return vectordb.TableMusings
	case MemoryTypePersonality:
		return vectordb.TablePersonality
	default:
		return vectordb.TableMemories
	}
}

// generateMemoryID generates a deterministic ID for a memory
func generateMemoryID(record *MemoryRecord) string {
	data := fmt.Sprintf("%s:%s:%d", record.Type, record.Content, record.Timestamp.Unix())
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:16])
}
