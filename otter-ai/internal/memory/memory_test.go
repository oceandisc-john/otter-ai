package memory

import (
	"context"
	"fmt"
	"testing"
	"time"

	"otter-ai/internal/vectordb"
)

// --- Mock VectorDB ---

type mockVectorDB struct {
	records map[string]map[string]*vectordb.Record // table -> id -> record
}

func newMockVectorDB() *mockVectorDB {
	return &mockVectorDB{
		records: map[string]map[string]*vectordb.Record{
			vectordb.TableMemories:    {},
			vectordb.TableMusings:     {},
			vectordb.TablePersonality: {},
		},
	}
}

func (m *mockVectorDB) Store(ctx context.Context, table, id string, vector []float32, metadata map[string]interface{}) error {
	if err := vectordb.ValidateTable(table); err != nil {
		return err
	}
	m.records[table][id] = &vectordb.Record{
		ID:       id,
		Vector:   vector,
		Metadata: metadata,
	}
	return nil
}

func (m *mockVectorDB) Search(ctx context.Context, table string, query []float32, limit int) ([]vectordb.SearchResult, error) {
	if err := vectordb.ValidateTable(table); err != nil {
		return nil, err
	}
	var results []vectordb.SearchResult
	for _, rec := range m.records[table] {
		results = append(results, vectordb.SearchResult{
			ID:       rec.ID,
			Vector:   rec.Vector,
			Metadata: rec.Metadata,
			Score:    1.0,
		})
		if len(results) >= limit {
			break
		}
	}
	return results, nil
}

func (m *mockVectorDB) Get(ctx context.Context, table, id string) (*vectordb.Record, error) {
	if err := vectordb.ValidateTable(table); err != nil {
		return nil, err
	}
	rec, ok := m.records[table][id]
	if !ok {
		return nil, fmt.Errorf("record %s not found", id)
	}
	return rec, nil
}

func (m *mockVectorDB) Delete(ctx context.Context, table, id string) error {
	if err := vectordb.ValidateTable(table); err != nil {
		return err
	}
	delete(m.records[table], id)
	return nil
}

func (m *mockVectorDB) List(ctx context.Context, table string, limit, offset int) ([]vectordb.Record, error) {
	if err := vectordb.ValidateTable(table); err != nil {
		return nil, err
	}
	var all []vectordb.Record
	for _, rec := range m.records[table] {
		all = append(all, *rec)
	}
	if offset >= len(all) {
		return nil, nil
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}
	return all[offset:end], nil
}

func (m *mockVectorDB) Close() error { return nil }

// --- Tests ---

func TestNew(t *testing.T) {
	db := newMockVectorDB()
	mem := New(db)
	if mem == nil {
		t.Fatal("New returned nil")
	}
}

func TestGetVectorDB(t *testing.T) {
	db := newMockVectorDB()
	mem := New(db)
	if mem.GetVectorDB() != db {
		t.Error("GetVectorDB returned wrong instance")
	}
}

func TestStore_Basic(t *testing.T) {
	mem := New(newMockVectorDB())
	ctx := context.Background()

	rec := &MemoryRecord{
		Type:      MemoryTypeLongTerm,
		Content:   "test content",
		Embedding: []float32{1, 0, 0},
		Scope:     "test",
	}

	if err := mem.Store(ctx, rec); err != nil {
		t.Fatalf("Store: %v", err)
	}

	// Should auto-generate ID and timestamp
	if rec.ID == "" {
		t.Error("ID should be auto-generated")
	}
	if rec.Timestamp.IsZero() {
		t.Error("Timestamp should be auto-set")
	}
}

func TestStore_PresetIDAndTimestamp(t *testing.T) {
	mem := New(newMockVectorDB())
	ctx := context.Background()
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	rec := &MemoryRecord{
		ID:        "custom-id",
		Type:      MemoryTypeLongTerm,
		Content:   "test",
		Embedding: []float32{1},
		Timestamp: ts,
	}

	if err := mem.Store(ctx, rec); err != nil {
		t.Fatalf("Store: %v", err)
	}
	if rec.ID != "custom-id" {
		t.Errorf("ID = %q; want custom-id", rec.ID)
	}
	if !rec.Timestamp.Equal(ts) {
		t.Error("Timestamp should not be changed when preset")
	}
}

func TestStore_MetadataMerge(t *testing.T) {
	db := newMockVectorDB()
	mem := New(db)
	ctx := context.Background()

	rec := &MemoryRecord{
		ID:        "m1",
		Type:      MemoryTypeLongTerm,
		Content:   "test",
		Embedding: []float32{1},
		Timestamp: time.Now(),
		Metadata:  map[string]interface{}{"custom": "value"},
	}

	if err := mem.Store(ctx, rec); err != nil {
		t.Fatalf("Store: %v", err)
	}

	stored := db.records[vectordb.TableMemories]["m1"]
	if stored.Metadata["custom"] != "value" {
		t.Error("custom metadata not preserved")
	}
	if stored.Metadata["content"] != "test" {
		t.Error("content metadata not set")
	}
}

func TestStore_AllMemoryTypes(t *testing.T) {
	db := newMockVectorDB()
	mem := New(db)
	ctx := context.Background()

	types := []struct {
		mt    MemoryType
		table string
	}{
		{MemoryTypeLongTerm, vectordb.TableMemories},
		{MemoryTypeShortTerm, vectordb.TableMemories},
		{MemoryTypeMusing, vectordb.TableMusings},
		{MemoryTypePersonality, vectordb.TablePersonality},
	}

	for _, tt := range types {
		rec := &MemoryRecord{
			ID:        string(tt.mt) + "-id",
			Type:      tt.mt,
			Content:   "test",
			Embedding: []float32{1},
			Timestamp: time.Now(),
		}
		if err := mem.Store(ctx, rec); err != nil {
			t.Errorf("Store %s: %v", tt.mt, err)
		}
		if _, ok := db.records[tt.table][rec.ID]; !ok {
			t.Errorf("expected record in table %s for type %s", tt.table, tt.mt)
		}
	}
}

func TestSearch(t *testing.T) {
	db := newMockVectorDB()
	mem := New(db)
	ctx := context.Background()

	rec := &MemoryRecord{
		ID:         "s1",
		Type:       MemoryTypeLongTerm,
		Content:    "searchable",
		Embedding:  []float32{1, 0},
		Timestamp:  time.Now(),
		Scope:      "test",
		Importance: 0.8,
	}
	_ = mem.Store(ctx, rec)

	results, err := mem.Search(ctx, []float32{1, 0}, MemoryTypeLongTerm, 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}
	if results[0].Content != "searchable" {
		t.Errorf("Content = %q; want searchable", results[0].Content)
	}
}

func TestGet(t *testing.T) {
	db := newMockVectorDB()
	mem := New(db)
	ctx := context.Background()

	ts := time.Unix(1700000000, 0)
	rec := &MemoryRecord{
		ID:         "g1",
		Type:       MemoryTypeMusing,
		Content:    "musing content",
		Embedding:  []float32{0, 1},
		Timestamp:  ts,
		Scope:      "global",
		Importance: 0.5,
	}
	_ = mem.Store(ctx, rec)

	got, err := mem.Get(ctx, "g1", MemoryTypeMusing)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Content != "musing content" {
		t.Errorf("Content = %q", got.Content)
	}
}

func TestGet_NotFound(t *testing.T) {
	mem := New(newMockVectorDB())
	_, err := mem.Get(context.Background(), "missing", MemoryTypeLongTerm)
	if err == nil {
		t.Error("expected error for missing record")
	}
}

func TestDelete(t *testing.T) {
	db := newMockVectorDB()
	mem := New(db)
	ctx := context.Background()

	rec := &MemoryRecord{
		ID:        "d1",
		Type:      MemoryTypeLongTerm,
		Content:   "to delete",
		Embedding: []float32{1},
		Timestamp: time.Now(),
	}
	_ = mem.Store(ctx, rec)

	if err := mem.Delete(ctx, "d1", MemoryTypeLongTerm); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := mem.Get(ctx, "d1", MemoryTypeLongTerm)
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestList(t *testing.T) {
	db := newMockVectorDB()
	mem := New(db)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		_ = mem.Store(ctx, &MemoryRecord{
			ID:        fmt.Sprintf("l%d", i),
			Type:      MemoryTypeLongTerm,
			Content:   fmt.Sprintf("item %d", i),
			Embedding: []float32{float32(i)},
			Timestamp: time.Now(),
		})
	}

	records, err := mem.List(ctx, MemoryTypeLongTerm, 3, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(records) != 3 {
		t.Errorf("expected 3 records, got %d", len(records))
	}
}

func TestList_Empty(t *testing.T) {
	mem := New(newMockVectorDB())
	records, err := mem.List(context.Background(), MemoryTypeLongTerm, 10, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("expected 0 records, got %d", len(records))
	}
}

func TestGenerateMemoryID_Deterministic(t *testing.T) {
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	r1 := &MemoryRecord{Type: MemoryTypeLongTerm, Content: "hello", Timestamp: ts}
	r2 := &MemoryRecord{Type: MemoryTypeLongTerm, Content: "hello", Timestamp: ts}

	id1 := generateMemoryID(r1)
	id2 := generateMemoryID(r2)
	if id1 != id2 {
		t.Errorf("same inputs should produce same ID: %q vs %q", id1, id2)
	}
	if id1 == "" {
		t.Error("ID should not be empty")
	}
}

func TestGenerateMemoryID_Different(t *testing.T) {
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	r1 := &MemoryRecord{Type: MemoryTypeLongTerm, Content: "hello", Timestamp: ts}
	r2 := &MemoryRecord{Type: MemoryTypeLongTerm, Content: "world", Timestamp: ts}

	if generateMemoryID(r1) == generateMemoryID(r2) {
		t.Error("different content should produce different IDs")
	}
}

func TestMemoryTypeConstants(t *testing.T) {
	if MemoryTypeShortTerm != "short_term" {
		t.Errorf("MemoryTypeShortTerm = %q", MemoryTypeShortTerm)
	}
	if MemoryTypeLongTerm != "long_term" {
		t.Errorf("MemoryTypeLongTerm = %q", MemoryTypeLongTerm)
	}
	if MemoryTypeMusing != "musing" {
		t.Errorf("MemoryTypeMusing = %q", MemoryTypeMusing)
	}
	if MemoryTypePersonality != "personality" {
		t.Errorf("MemoryTypePersonality = %q", MemoryTypePersonality)
	}
}
