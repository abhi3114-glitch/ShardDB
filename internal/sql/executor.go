package sql

import (
	"fmt"
	"strings"

	"github.com/myuser/sharddb/internal/storage"
)

// Row representing a result row.
type Row []string

// Execute executes a logical plan against the storage engine.
// For MVP, readTs is passed directly.
func Execute(plan PlanNode, engine storage.Engine, readTs uint64) ([]Row, error) {
	switch n := plan.(type) {
	case *InsertNode:
		return executeInsert(n, engine, readTs) // Insert also uses a commitTs, using readTs as commitTs for now
	case *PointGetNode:
		return executePointGet(n, engine, readTs)
	case *ProjectNode:
		return executeProject(n, engine, readTs)
	default:
		return nil, fmt.Errorf("unsupported plan node: %T", plan)
	}
}

func executeInsert(n *InsertNode, engine storage.Engine, commitTs uint64) ([]Row, error) {
	if len(n.Values) == 0 {
		return nil, nil
	}

	// Assume first column/value is PK.
	pk := n.Values[0] // byte slice

	// Construct Value
	var valParts []string
	for _, v := range n.Values {
		valParts = append(valParts, string(v))
	}
	valStr := strings.Join(valParts, ",")

	// Encode Key: Table + PK + TS
	// Need TableID? Use TableName as ID for now (hash or raw).
	// Just use "TableName:PK" as key prefix.
	userKey := []byte(n.Table + ":" + string(pk))
	// encodedKey := storage.EncodeKey(userKey, commitTs) // REMOVE this manual encoding

	// Use Engine Transactional API to respect commitTs
	txnID := fmt.Sprintf("auto-%d", commitTs)
	err := engine.Prepare(userKey, []byte(valStr), txnID)
	if err != nil {
		return nil, err
	}
	err = engine.Commit(userKey, txnID, commitTs)
	if err != nil {
		return nil, err
	}

	return []Row{{fmt.Sprintf("Inserted 1 row")}}, nil
}

func executePointGet(n *PointGetNode, engine storage.Engine, readTs uint64) ([]Row, error) {
	// 1. Construct Key: Table + ":" + Key
	userKey := []byte(n.Table + ":" + string(n.Key))

	val, err := engine.GetTxn(userKey, readTs)
	if err != nil {
		return nil, err
	}
	if val == nil {
		return []Row{}, nil // Empty
	}

	// Return [Key, Value] (Key is n.Key userspace)
	return []Row{{string(n.Key), string(val)}}, nil
}

func executeProject(n *ProjectNode, engine storage.Engine, readTs uint64) ([]Row, error) {
	children := n.Children()
	if len(children) != 1 {
		return nil, fmt.Errorf("project must have 1 child")
	}

	inputRows, err := executeAny(children[0], engine, readTs)
	if err != nil {
		return nil, err
	}

	return inputRows, nil
}

func executeFilter(n *FilterNode, engine storage.Engine, readTs uint64) ([]Row, error) {
	inputRows, err := executeAny(n.Input, engine, readTs)
	if err != nil {
		return nil, err
	}
	return inputRows, nil
}

func executeScan(n *ScanNode, engine storage.Engine, readTs uint64) ([]Row, error) {
	prefix := []byte(n.Table + ":")
	var rows []Row
	seenKeys := make(map[string]bool)

	start := prefix
	end := make([]byte, len(prefix))
	copy(end, prefix)
	end = append(end, 0xFF)

	err := engine.Scan(start, end, func(k, v []byte) bool {
		decodedKey, decodedTs := storage.DecodeKey(k)

		if !strings.HasPrefix(string(decodedKey), string(prefix)) {
			return true
		}

		keyStr := string(decodedKey)
		if seenKeys[keyStr] {
			return true
		}

		if decodedTs <= readTs {
			// Visible!
			// Unwrap Key: "Table:RealKey" -> "RealKey"
			realKey := keyStr
			parts := strings.SplitN(keyStr, ":", 2)
			if len(parts) == 2 {
				realKey = parts[1]
			}

			rows = append(rows, Row{realKey, string(v)})
			seenKeys[keyStr] = true
		}
		return true
	})

	if err != nil {
		return nil, err
	}

	return rows, nil
}

func executeAny(plan PlanNode, engine storage.Engine, readTs uint64) ([]Row, error) {
	switch n := plan.(type) {
	case *ScanNode:
		return executeScan(n, engine, readTs)
	case *FilterNode:
		return executeFilter(n, engine, readTs)
	case *ProjectNode:
		return executeProject(n, engine, readTs)
	case *PointGetNode:
		return executePointGet(n, engine, readTs)
	default:
		return nil, fmt.Errorf("unknown node type: %T", n)
	}
}







