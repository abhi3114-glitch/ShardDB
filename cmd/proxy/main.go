package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/myuser/sharddb/internal/metrics"
	"github.com/myuser/sharddb/internal/planner"
	"github.com/myuser/sharddb/internal/router"
	"github.com/myuser/sharddb/internal/sql"
	"github.com/myuser/sharddb/internal/txn"
)

func main() {
	metaAddr := flag.String("meta", "http://localhost:9000", "Meta Node Address")
	port := flag.Int("port", 8080, "Proxy Port")
	flag.Parse()

	fmt.Printf("Starting ShardDB Proxy on %d connected to Meta %s\n", *port, *metaAddr)

	rtr := router.NewRouter(*metaAddr)
	coord := txn.NewCoordinator(rtr)

	http.HandleFunc("/metrics", metrics.Handler)

	http.HandleFunc("/execute", func(w http.ResponseWriter, r *http.Request) {
		metrics.Inc("requests_total")
		sqlStr := r.URL.Query().Get("sql")
		if sqlStr == "" {
			buf := new(strings.Builder)
			io.Copy(buf, r.Body)
			sqlStr = buf.String()
		}

		clientTxnID := r.URL.Query().Get("txn") // Passed by client if inside a transaction

		fmt.Printf("Proxy Received: %s (Txn: %s)\n", sqlStr, clientTxnID)

		// 1. Handle Transaction Commands
		upperParams := strings.ToUpper(strings.TrimSpace(sqlStr))
		if upperParams == "BEGIN" {
			newID := coord.Begin()
			w.Write([]byte(fmt.Sprintf("TxnID:%s", newID)))
			return
		}
		if upperParams == "COMMIT" {
			if clientTxnID == "" {
				http.Error(w, "COMMIT without txn param", 400)
				return
			}
			err := coord.Commit(clientTxnID)
			if err != nil {
				http.Error(w, fmt.Sprintf("COMMIT Failed: %v", err), 500)
				return
			}
			w.Write([]byte("COMMIT OK"))
			return
		}
		if upperParams == "ROLLBACK" {
			if clientTxnID == "" {
				http.Error(w, "ROLLBACK without txn param", 400)
				return
			}
			coord.Abort(clientTxnID)
			w.Write([]byte("ROLLBACK OK"))
			return
		}

		// 2. Parse SQL
		plan, err := sql.ParseToPlan(sqlStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("Proxy Parse Error: %v", err), 400)
			return
		}

		// 3. Handle INSERT
		if ins, ok := plan.(*sql.InsertNode); ok {
			// Extract Key/Value
			// Assuming single value insert
			if len(ins.Values) > 0 && len(ins.Values) >= 2 {
				// HACK: Value is... complicated.
				// Our Parser stores `Values [][]byte`.
				// Table structure? User specifies?
				// MVP: INSERT INTO t VALUES (key, val)
				// Parser likely stores ALL values.
				// We assume Col 0 is Key, Col 1 is Value.
				// Format Key as "Table:Key" to match executor expectation
				rawKey := string(ins.Values[0])
				key := ins.Table + ":" + rawKey
				val := string(ins.Values[1])

				if clientTxnID != "" {
					// BUFFER WRITE
					err := coord.AddWrite(clientTxnID, key, val)
					if err != nil {
						http.Error(w, err.Error(), 500)
						return
					}
					w.Write([]byte("Buffered Write OK"))
					return
				}
				// Else: Auto-Commit (Legacy Forward)
				// We fall through to routing logic below.
			}
		}

		// 4. Planner & Routing
		pln, err := planner.Analyze(sqlStr)
		if err != nil {
			// Fallback?
			fmt.Printf("Planner Error: %v. Defaulting to Broadcast.\n", err)
			pln = &planner.Plan{Type: planner.PlanBroadcast}
		}

		// Explicit hint override
		if hint := r.URL.Query().Get("routing_key"); hint != "" {
			pln = &planner.Plan{Type: planner.PlanSingle, RoutingKey: []byte(hint)}
		}

		if pln.Type == planner.PlanBroadcast {
			// Scatter-Gather
			fmt.Println("Plan: Broadcast")

			// Get all shards
			// Using ShardAddrs from router.
			// Need access. ShardAddrs is exported.
			// Router doesn't expose keys list directly, only map.

			type result struct {
				shardID uint64
				rows    [][]string // JSON decodes []byte as base64 string
				err     error
			}

			// We need to access ShardAddrs safely.
			// router.ShardAddrs is protected by mutex but we can't lock it from here cleanly if we iterate map?
			// Router doesn't expose "GetAllShards".
			// We iterate 1..N assuming known IDs?
			// Or we assume 1,2,3 for MVP?
			// Better: Add GetAllShards to Router.
			// For now: Iterate 1,2,3 since we know the dev cluster.
			// Or iterate keys of map if we can access it (unsafe if concurrent writes, but router is mostly static).
			// Let's add GetAllShardIDs to Router properly later.
			// For now, hardcode 1, 2, 3 as per NewRouter default.
			shardIDs := []uint64{1, 2, 3}

			resultsCh := make(chan result, len(shardIDs))
			var wg sync.WaitGroup

			for _, sid := range shardIDs {
				wg.Add(1)
				go func(id uint64) {
					defer wg.Done()
					respStr, err := rtr.ForwardRequest(id, sqlStr)
					if err != nil {
						resultsCh <- result{shardID: id, err: err}
						return
					}

					// Parse Result
					var rows [][]string
					// Ignore empty/OK response for non-SELECT (broadcast delete?)
					if strings.TrimSpace(respStr) == "OK" {
						resultsCh <- result{shardID: id}
						return
					}

					if err := json.Unmarshal([]byte(respStr), &rows); err != nil {
						// Not JSON?
						resultsCh <- result{shardID: id}
						return
					}
					resultsCh <- result{shardID: id, rows: rows}
				}(sid)
			}

			wg.Wait()
			close(resultsCh)

			var allRows [][]string
			// var errs []error

			for res := range resultsCh {
				if res.err != nil {
					// Partial failure? Log.
					fmt.Printf("Shard %d failed: %v\n", res.shardID, res.err)
					continue
				}
				allRows = append(allRows, res.rows...)
			}

			json.NewEncoder(w).Encode(allRows)
			return
		}

		// Single Shard
		key := pln.RoutingKey
		if len(key) == 0 {
			key = []byte("default")
		}

		shardID := rtr.GetShardID(key)
		fmt.Printf("Plan: Single Shard %d (Key: %s)\n", shardID, key)

		resp, err := rtr.ForwardRequest(shardID, sqlStr)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Write([]byte(resp))
	})

	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), nil))
}


