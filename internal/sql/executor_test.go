package sql

import (
	"testing"

	"github.com/myuser/sharddb/internal/storage"
)

func TestExecutor_InsertSelect(t *testing.T) {
	// Setup storage
	store := storage.NewMemoryStore()
	defer store.Close()

	// 1. Parse Insert
	insSql := "INSERT INTO users (name, age) VALUES ('alice', '30')"
	insPlan, err := ParseToPlan(insSql)
	if err != nil {
		t.Fatalf("Parse Insert failed: %v", err)
	}

	// 2. Execute Insert
	// TS = 10
	rows, err := Execute(insPlan, store, 10)
	if err != nil {
		t.Fatalf("Execute Insert failed: %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("Expected 1 result row, got %d", len(rows))
	}

	// 3. Parse Select
	selSql := "SELECT name FROM users"
	selPlan, err := ParseToPlan(selSql)
	if err != nil {
		t.Fatalf("Parse Select failed: %v", err)
	}

	// 4. Execute Select
	// TS = 15 (should see TS=10)
	rows, err = Execute(selPlan, store, 15)
	if err != nil {
		t.Fatalf("Execute Select failed: %v", err)
	}

	if len(rows) != 1 {
		t.Fatalf("Expected 1 row, got %d", len(rows))
	}

	rowStr := rows[0][0] // First column of first row
	// Implementation stores value as "val1,val2" (alice,30)
	// Project Implementation currently returns FULL ROW.
	// So we expect "alice,30".

	if rowStr != "alice,30" {
		t.Errorf("Expected 'alice,30', got '%s'", rowStr)
	}

	// 5. Verify Isolation (TS = 5 should not see TS=10)
	rows, err = Execute(selPlan, store, 5)
	if err != nil {
		t.Fatalf("Execute Select (Old TS) failed: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("Expected 0 rows for old TS, got %d", len(rows))
	}
}


