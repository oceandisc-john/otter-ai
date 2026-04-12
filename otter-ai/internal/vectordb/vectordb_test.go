// Tests for the vectordb package — pure Go logic tests that don't require CGO.
// SQLite integration tests are in sqlite_test.go (build tag: cgo).
package vectordb

import (
	"testing"
)

// --- ValidateTable ---

func TestValidateTable_Authorized(t *testing.T) {
	for _, table := range []string{TableMemories, TableMusings, TablePersonality} {
		if err := ValidateTable(table); err != nil {
			t.Errorf("ValidateTable(%q) unexpected error: %v", table, err)
		}
	}
}

func TestValidateTable_Unauthorized(t *testing.T) {
	if err := ValidateTable("evil_table"); err == nil {
		t.Error("expected error for unauthorized table")
	}
}

// --- New factory error paths (no CGO needed) ---

func TestNew_UnsupportedBackends(t *testing.T) {
	for _, b := range []Backend{BackendPostgres, BackendDuckDB, BackendLanceDB, Backend("unknown")} {
		_, err := New(b, "")
		if err == nil {
			t.Errorf("expected error for backend %q", b)
		}
	}
}

// --- Backend constants ---

func TestBackendConstants(t *testing.T) {
	if BackendSQLite != "sqlite" {
		t.Errorf("BackendSQLite = %q", BackendSQLite)
	}
	if BackendPostgres != "postgres" {
		t.Errorf("BackendPostgres = %q", BackendPostgres)
	}
	if BackendDuckDB != "duckdb" {
		t.Errorf("BackendDuckDB = %q", BackendDuckDB)
	}
	if BackendLanceDB != "lancedb" {
		t.Errorf("BackendLanceDB = %q", BackendLanceDB)
	}
}

// --- Table constants ---

func TestTableConstants(t *testing.T) {
	if TableMemories != "memories" {
		t.Errorf("TableMemories = %q", TableMemories)
	}
	if TableMusings != "musings" {
		t.Errorf("TableMusings = %q", TableMusings)
	}
	if TablePersonality != "personality" {
		t.Errorf("TablePersonality = %q", TablePersonality)
	}
}
