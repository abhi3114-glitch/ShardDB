package planner

import (
	"fmt"

	"github.com/blastrain/vitess-sqlparser/sqlparser"
)

type PlanType int

const (
	PlanSingle    PlanType = 0
	PlanBroadcast PlanType = 1
)

type Plan struct {
	Type       PlanType
	RoutingKey []byte
}

func Analyze(sql string) (*Plan, error) {
	stmt, err := sqlparser.Parse(sql)
	if err != nil {
		return nil, fmt.Errorf("parse error: %v", err)
	}

	switch s := stmt.(type) {
	case *sqlparser.Select:
		return analyzeSelect(s)
	case *sqlparser.Insert:
		return analyzeInsert(s)
	default:
		// Unknown statement (BEGIN/COMMIT etc dealt with by Coordinator) -> Broadcast or Default?
		// Assume Default (Shard 1) or Error?
		// For safe MVP: Default "default" key.
		return &Plan{Type: PlanSingle, RoutingKey: []byte("default")}, nil
	}
}

func analyzeInsert(stmt *sqlparser.Insert) (*Plan, error) {
	// Simple Logic: Key is first column?
	// Or first value?
	// See `sql.parser` implementation assumption:
	// "INSERT INTO t VALUES (key, val)" -> Col 0 is key.

	// Check Values
	rows, ok := stmt.Rows.(sqlparser.Values)
	if !ok || len(rows) == 0 {
		return nil, fmt.Errorf("insert without values not supported")
	}

	row := rows[0]
	if len(row) == 0 {
		return nil, fmt.Errorf("empty row")
	}

	valExpr := row[0] // Key
	// Must be value
	keyStr := sqlparser.String(valExpr)
	// Trimming quotes if string literal?
	// sqlparser.String returns "'keyA'".
	// We might need raw bytes.
	if sVal, ok := valExpr.(*sqlparser.SQLVal); ok {
		return &Plan{Type: PlanSingle, RoutingKey: sVal.Val}, nil
	}

	return &Plan{Type: PlanSingle, RoutingKey: []byte(keyStr)}, nil
}

func analyzeSelect(stmt *sqlparser.Select) (*Plan, error) {
	// Walk WHERE clause for 'key = ...'
	if stmt.Where == nil {
		return &Plan{Type: PlanBroadcast}, nil
	}

	// Function to find key equality
	var foundKey []byte
	found := false

	sqlparser.Walk(func(node sqlparser.SQLNode) (bool, error) {
		if found {
			return false, nil
		}

		if cmp, ok := node.(*sqlparser.ComparisonExpr); ok {
			if cmp.Operator == sqlparser.EqualStr {
				// Check Left side
				if _, ok := cmp.Left.(*sqlparser.ColName); ok {
					// We assume routing key column is named 'id' or 'key' or we just take ANY equality as routing hint?
					// MVP: If column name contains "key" or "id"?
					// Or simpler: Any equality?
					// Let's assume ANY equality restricts to single shard (Optimistic).
					// Real planner needs Schema.
					// Let's check if Right is Value.
					if val, ok := cmp.Right.(*sqlparser.SQLVal); ok {
						foundKey = val.Val
						found = true
						return false, nil
					}
				}
			}
		}
		return true, nil
	}, stmt.Where)

	if found {
		return &Plan{Type: PlanSingle, RoutingKey: foundKey}, nil
	}

	return &Plan{Type: PlanBroadcast}, nil
}

