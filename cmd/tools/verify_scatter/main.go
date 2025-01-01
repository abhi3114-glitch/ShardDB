package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func req(method, urlStr string) string {
	resp, err := http.Get(urlStr)
	if err != nil {
		fmt.Printf("FAIL: %s %s -> %v\n", method, urlStr, err)
		return ""
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("%s %s -> %d\n", method, urlStr, resp.StatusCode)
	return string(body)
}

func main() {
	proxy := "http://localhost:8080/execute"

	fmt.Println("1. Cleaning Cluster (Optional: We assume clean or append)")

	// 2. Insert into Shard 1 (Key "keyA")
	// Based on Router: "keyA" -> Shard 1? Or Shard 1 default?
	// Router uses alphabetical ranges.
	// Default range covers all. Shard 1.
	// We need keys that go to different shards?
	// Ah, our Router currently maps ALL keys to Shard 1 by default unless we updated it.
	// `NewRouter`: ranges=[{Start:nil, End:nil, ShardID:1}]
	// So Scatter-Gather will technically verify "Broadcast", but only Shard 1 has data?
	// Wait, Broadcast sends to All Shards (1, 2, 3).
	// If Shard 1 has Data, Shard 2 empty, Shard 3 empty.
	// Merged result = Data + [] + [].
	// This verifies Broadcast works.
	// To verify logic truly merges different data, we need to manually inject data into Shard 2?
	// OR update Router?
	// Ideally we update Router.
	// BUT, if we just insert "keyA" (Shard 1 according to Router).
	// And manually insert "keyZ" into Shard 2 using direct Shard Port?
	// Yes!

	fmt.Println("2. Inserting via Proxy (Shard 1)")
	req("GET", proxy+"?sql="+url.QueryEscape("INSERT INTO t VALUES (keyA, valA)"))

	fmt.Println("3. Inserting Direct to Shard 2 (Bypass Router)")
	// Shard 2 is port 9002.
	// Direct shard node expects "SQL:" proposal or via /execute?
	// /execute on shard node handles SQL.
	req("POST", "http://localhost:9002/execute?sql="+url.QueryEscape("INSERT INTO t VALUES (keyB, valB)"))

	time.Sleep(1 * time.Second)

	// 4. Query Specific (keyA) -> Should hit Shard 1
	fmt.Println("4. SELECT keyA (Point Query)")
	bodyA := req("GET", proxy+"?sql="+url.QueryEscape("SELECT * FROM t WHERE k = 'keyA'"))
	fmt.Println("Result A:", bodyA)

	// 5. Query Broadcast (SELECT *) -> Should hit Shard 1 AND 2
	fmt.Println("5. SELECT * (Scatter-Gather)")
	bodyAll := req("GET", proxy+"?sql="+url.QueryEscape("SELECT * FROM t"))
	fmt.Println("Result All:", bodyAll)

	// Check if bodyAll contains both valA and valB
	var rows [][]string
	json.Unmarshal([]byte(bodyAll), &rows)
	foundA := false
	foundB := false
	for _, r := range rows {
		// Row is [key, val]
		// Val usually contains "key,val" due to simple storage format
		if len(r) > 1 {
			valCol := r[1]
			// Check strict or substring
			if valCol == "valA" || strings.Contains(valCol, ",valA") || strings.Contains(valCol, "valA") {
				foundA = true
			}
			if valCol == "valB" || strings.Contains(valCol, ",valB") || strings.Contains(valCol, "valB") {
				foundB = true
			}
		}
	}

	if foundA && foundB {
		fmt.Println("SUCCESS: Found both valA and valB from different shards!")
	} else {
		fmt.Printf("FAILURE: Missing data. A=%v, B=%v. Rows: %v\n", foundA, foundB, rows)
	}
}




