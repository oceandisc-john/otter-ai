//go:build cgo

package vectordb

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// --- helpers ---

func tempDB(t *testing.T) *SQLiteVectorDB {
	t.Helper()
	dir := t.TempDir()
	db, err := NewSQLiteVectorDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewSQLiteVectorDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func vec(vals ...float32) []float32 { return vals }

// --- New with SQLite backend ---

func TestNew_SQLiteBackend(t *testing.T) {
	dir := t.TempDir()
	db, err := New(BackendSQLite, filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer db.Close()
}

// --- Store ---

func TestStore_Basic(t *testing.T) {
	db := tempDB(t)
	ctx := context.Background()

	err := db.Store(ctx, TableMemories, "id1", vec(1, 0, 0), map[string]interface{}{"key": "value"})
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
}

func TestStore_Upsert(t *testing.T) {
	db := tempDB(t)
	ctx := context.Background()

	_ = db.Store(ctx, TableMemories, "id1", vec(1, 0), map[string]interface{}{"v": 1})
	err := db.Store(ctx, TableMemories, "id1", vec(0, 1), map[string]interface{}{"v": 2})
	if err != nil {
		t.Fatalf("Store upsert: %v", err)
	}

	rec, err := db.Get(ctx, TableMemories, "id1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if v, ok := rec.Metadata["v"].(float64); !ok || v != 2 {
		t.Errorf("expected metadata v=2, got %v", rec.Metadata["v"])
	}
}

func TestStore_InvalidTable(t *testing.T) {
	db := tempDB(t)
	err := db.Store(context.Background(), "bad_table", "id", vec(1), nil)
	if err == nil {
		t.Error("expected error for invalid table")
	}
}

func TestStore_AllTables(t *testing.T) {
	db := tempDB(t)
	ctx := context.Background()
	for _, table := range []string{TableMemories, TableMusings, TablePersonality} {
		err := db.Store(ctx, table, "id-"+table, vec(1, 2), map[string]interface{}{"t": table})
		if err != nil {
			t.Errorf("Store to %s: %v", table, err)
		}
	}
}

// --- Get ---

func TestGet_Found(t *testing.T) {
	db := tempDB(t)
	ctx := context.Background()
	_ = db.Store(ctx, TableMemories, "id1", vec(1, 2, 3), map[string]interface{}{"a": "b"})

	rec, err := db.Get(ctx, TableMemories, "id1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if rec.ID != "id1" {
		t.Errorf("ID = %q; want id1", rec.ID)
	}
	if len(rec.Vector) != 3 {
		t.Errorf("Vector len = %d; want 3", len(rec.Vector))
	}
}

func TestGet_NotFound(t *testing.T) {
	db := tempDB(t)
	_, err := db.Get(context.Background(), TableMemories, "missing")
	if err == nil {
		t.Error("expected error for missing record")
	}
}

func TestGet_InvalidTable(t *testing.T) {
	db := tempDB(t)
	_, err := db.Get(context.Background(), "bad", "id")
	if err == nil {
		t.Error("expected error for invalid table")
	}
}

// --- Delete ---

func TestDelete_Basic(t *testing.T) {
	db := tempDB(t)
	ctx := context.Background()
	_ = db.Store(ctx, TableMemories, "id1", vec(1), map[string]interface{}{})

	err := db.Delete(ctx, TableMemories, "id1")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = db.Get(ctx, TableMemories, "id1")
	if err == nil {
		t.Error("expected not found after delete")
	}
}

func TestDelete_InvalidTable(t *testing.T) {
	db := tempDB(t)
	err := db.Delete(context.Background(), "bad", "id")
	if err == nil {
		t.Error("expected error for invalid table")
	}
}

func TestDelete_NonExistent(t *testing.T) {
	db := tempDB(t)
	err := db.Delete(context.Background(), TableMemories, "nope")
	if err != nil {
		t.Errorf("Delete non-existent: %v", err)
	}
}

// --- List ---

func TestList_Empty(t *testing.T) {
	db := tempDB(t)
	records, err := db.List(context.Background(), TableMemories, 10, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("expected 0 records, got %d", len(records))
	}
}

func TestList_WithRecords(t *testing.T) {
	db := tempDB(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		_ = db.Store(ctx, TableMemories, "id"+string(rune('a'+i)), vec(float32(i)), map[string]interface{}{})
	}

	records, err := db.List(ctx, TableMemories, 3, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(records) != 3 {
		t.Errorf("expected 3 records, got %d", len(records))
	}
}

func TestList_Offset(t *testing.T) {
	db := tempDB(t)
	ctx := context.Background()
	_ = db.Store(ctx, TableMemories, "a", vec(1), map[string]interface{}{})
	_ = db.Store(ctx, TableMemories, "b", vec(2), map[string]interface{}{})

	records, err := db.List(ctx, TableMemories, 10, 1)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(records) != 1 {
		t.Errorf("expected 1 record with offset=1, got %d", len(records))
	}
}

func TestList_InvalidTable(t *testing.T) {
	db := tempDB(t)
	_, err := db.List(context.Background(), "bad", 10, 0)
	if err == nil {
		t.Error("expected error for invalid table")
	}
}

// --- Search ---

func TestSearch_CosineSimilarity(t *testing.T) {
	db := tempDB(t)
	ctx := context.Background()

	_ = db.Store(ctx, TableMemories, "close", vec(1, 0, 0), map[string]interface{}{"label": "close"})
	_ = db.Store(ctx, TableMemories, "far", vec(0, 0, 1), map[string]interface{}{"label": "far"})
	_ = db.Store(ctx, TableMemories, "medium", vec(0.7, 0.7, 0), map[string]interface{}{"label": "medium"})

	results, err := db.Search(ctx, TableMemories, vec(1, 0, 0), 3)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if results[0].ID != "close" {
		t.Errorf("first result should be 'close', got %q", results[0].ID)
	}
}

func TestSearch_Limit(t *testing.T) {
	db := tempDB(t)
	ctx := context.Background()
	for i := 0; i < 10; i++ {
		_ = db.Store(ctx, TableMemories, string(rune('a'+i)), vec(float32(i)), map[string]interface{}{})
	}

	results, err := db.Search(ctx, TableMemories, vec(5), 3)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}
}

func TestSearch_InvalidTable(t *testing.T) {
	db := tempDB(t)
	_, err := db.Search(context.Background(), "bad", vec(1), 5)
	if err == nil {
		t.Error("expected error for invalid table")
	}
}

func TestSearch_Empty(t *testing.T) {
	db := tempDB(t)
	results, err := db.Search(context.Background(), TableMemories, vec(1), 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

// --- cosineSimilarity ---

func TestCosineSimilarity_Identical(t *testing.T) {
	score := cosineSimilarity(vec(1, 0, 0), vec(1, 0, 0))
	if math.Abs(score-1.0) > 1e-6 {
		t.Errorf("expected 1.0, got %f", score)
	}
}

func TestCosineSimilarity_Orthogonal(t *testing.T) {
	score := cosineSimilarity(vec(1, 0, 0), vec(0, 1, 0))
	if math.Abs(score) > 1e-6 {
		t.Errorf("expected 0.0, got %f", score)
	}
}

func TestCosineSimilarity_Opposite(t *testing.T) {
	score := cosineSimilarity(vec(1, 0), vec(-1, 0))
	if math.Abs(score-(-1.0)) > 1e-6 {
		t.Errorf("expected -1.0, got %f", score)
	}
}

func TestCosineSimilarity_DifferentLengths(t *testing.T) {
	score := cosineSimilarity(vec(1, 0), vec(1, 0, 0))
	if score != 0 {
		t.Errorf("expected 0 for different lengths, got %f", score)
	}
}

func TestCosineSimilarity_ZeroVector(t *testing.T) {
	score := cosineSimilarity(vec(0, 0, 0), vec(1, 0, 0))
	if score != 0 {
		t.Errorf("expected 0 for zero vector, got %f", score)
	}
}

// --- Close / GetDB ---

func TestClose(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLiteVectorDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestGetDB(t *testing.T) {
	db := tempDB(t)
	if db.GetDB() == nil {
		t.Error("GetDB returned nil")
	}
}

// --- NewSQLiteVectorDB with invalid path ---

func TestNewSQLiteVectorDB_InvalidPath(t *testing.T) {
	_, err := NewSQLiteVectorDB(filepath.Join(os.DevNull, "nonexistent", "test.db"))
	if err == nil {
		t.Error("expected error for invalid path")
	}
}

// --- Governance tables ---

func TestGovernanceTables_Exist(t *testing.T) {
	db := tempDB(t)
	sqlDB := db.GetDB()

	tables := []string{"governance_rafts", "governance_members", "governance_rules"}
	for _, table := range tables {
		var name string
		err := sqlDB.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
		if err != nil {
			t.Errorf("governance table %q not found: %v", table, err)
		}
	}
}
