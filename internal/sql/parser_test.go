package sql

import (
	"strings"
	"testing"
)

func TestParseSelect(t *testing.T) {
	sql := "SELECT name, age FROM users WHERE age > 21"
	plan, err := ParseToPlan(sql)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Expected structure: Project -> Filter -> Scan
	// Check root
	proj, ok := plan.(*ProjectNode)
	if !ok {
		t.Fatalf("Expected ProjectNode, got %T", plan)
	}

	// Check Filter
	if len(proj.Input.Children()) == 0 && proj.Input.Type() == NodeScan {
		// Maybe no filter? Wait, WHERE clause exists.
	}

	// Simple string check for now as my String() methods are descriptive
	str := plan.String()
	if !strings.Contains(str, "Project") {
		t.Errorf("Expected string representation to contain Project, got %s", str)
	}
	// My String() implementation doesn't look at children recursively.
	// But let's check types.

	filter, ok := proj.Input.(*FilterNode)
	if !ok {
		t.Fatalf("Expected FilterNode child of Project, got %T", proj.Input)
	}

	scan, ok := filter.Input.(*ScanNode)
	if !ok {
		t.Fatalf("Expected ScanNode child of Filter, got %T", filter.Input)
	}

	if scan.Table != "users" {
		t.Errorf("Expected table 'users', got '%s'", scan.Table)
	}
}

func TestParseInsert(t *testing.T) {
	sql := "INSERT INTO users (name, age) VALUES ('alice', '30')"
	plan, err := ParseToPlan(sql)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	ins, ok := plan.(*InsertNode)
	if !ok {
		t.Fatalf("Expected InsertNode, got %T", plan)
	}

	if ins.Table != "users" {
		t.Errorf("Expected table 'users', got '%s'", ins.Table)
	}
	if len(ins.Values) != 2 {
		t.Errorf("Expected 2 values, got %d", len(ins.Values))
	}
}

