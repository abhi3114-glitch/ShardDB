package sql

import (
	"fmt"

	"github.com/blastrain/vitess-sqlparser/sqlparser"
)

// ParseToPlan parses a SQL string and returns a logical plan.
func ParseToPlan(sql string) (PlanNode, error) {
	stmt, err := sqlparser.Parse(sql)
	if err != nil {
		return nil, err
	}

	switch s := stmt.(type) {
	case *sqlparser.Select:
		return buildSelectPlan(s)
	case *sqlparser.Insert:
		return buildInsertPlan(s)
	default:
		return nil, fmt.Errorf("unsupported statement type: %T", stmt)
	}
}

func buildSelectPlan(stmt *sqlparser.Select) (PlanNode, error) {
	// 1. From Clause (Scan)
	if len(stmt.From) == 0 {
		return nil, fmt.Errorf("SELECT without FROM is not supported")
	}

	// Support simple table (aliased or not)
	// Only first table for now
	tableExpr := stmt.From[0]
	aliasedTable, ok := tableExpr.(*sqlparser.AliasedTableExpr)
	if !ok {
		return nil, fmt.Errorf("complex FROM clauses not supported")
	}
	tableNameStr := sqlparser.String(aliasedTable.Expr)

	node := PlanNode(&ScanNode{Table: tableNameStr})

	// 2. Where Clause (Filter)
	if stmt.Where != nil {
		exprStr := sqlparser.String(stmt.Where.Expr)

		// Optimization: Check for Point Lookup (Key = Value)
		if cmp, ok := stmt.Where.Expr.(*sqlparser.ComparisonExpr); ok {
			if cmp.Operator == sqlparser.EqualStr {
				if _, ok := cmp.Left.(*sqlparser.ColName); ok { // Assuming any col = key for MVP
					if val, ok := cmp.Right.(*sqlparser.SQLVal); ok {
						// Found Point Lookup!
						return &PointGetNode{
							Table: tableNameStr,
							Key:   val.Val,
						}, nil
					}
				}
			}
		}

		node = &FilterNode{
			Input: node,
			Expr:  exprStr,
		}
	}

	// 3. Select Exprs (Project)
	// Just capture column names for now
	var cols []string
	for _, expr := range stmt.SelectExprs {
		switch e := expr.(type) {
		case *sqlparser.AliasedExpr:
			cols = append(cols, sqlparser.String(e.Expr))
		case *sqlparser.StarExpr:
			cols = append(cols, "*")
		}
	}

	node = &ProjectNode{
		Input:   node,
		Columns: cols,
	}

	return node, nil
}

func buildInsertPlan(stmt *sqlparser.Insert) (PlanNode, error) {
	tableNameStr := sqlparser.String(stmt.Table)

	// Columns
	var cols []string
	for _, col := range stmt.Columns {
		cols = append(cols, col.String())
	}

	// Values
	// stmt.Rows can be *Select, *Values, etc.
	rowsVals, ok := stmt.Rows.(sqlparser.Values)
	if !ok {
		return nil, fmt.Errorf("INSERT from SELECT not supported")
	}

	var values [][]byte
	// Supporting single row insert for now logic
	// But structure allows multiples?
	// Let's flattening for MVP or just first row?
	// Note: InsertNode needs List of Rows?
	// My PlanNode has `Values [][]byte`. This implies 1 Row.
	// Let's assume 1 row for this MVP wrapper logic or just store raw strings.

	if len(rowsVals) > 0 {
		row := rowsVals[0]
		for _, val := range row {
			// Convert value expr to string/bytes
			switch v := val.(type) {
			case *sqlparser.SQLVal:
				values = append(values, v.Val)
			default:
				// Fallback for others
				valStr := sqlparser.String(val)
				values = append(values, []byte(valStr))
			}
		}
	}

	return &InsertNode{
		Table:   tableNameStr,
		Columns: cols,
		Values:  values,
	}, nil
}








