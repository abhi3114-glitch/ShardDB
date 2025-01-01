package main

import (
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
	fmt.Printf("%s %s -> %d: %s\n", method, urlStr, resp.StatusCode, string(body))
	return string(body)
}

func main() {
	proxy := "http://localhost:8080/execute"

	// 1. Begin Txn
	fmt.Println("1. BEGIN Transaction")
	body := req("GET", proxy+"?sql=BEGIN")
	if !strings.HasPrefix(body, "TxnID:") {
		fmt.Println("Failed to start txn")
		return
	}
	txnID := strings.TrimPrefix(body, "TxnID:")
	fmt.Printf("Started Txn: %s\n", txnID)

	// 2. Insert A (Buffered)
	fmt.Println("2. INSERT A")
	// Encoded SQL: INSERT INTO t VALUES (keyA, valA)
	sqlA := "INSERT INTO t VALUES (keyA, valA)"
	req("GET", proxy+"?txn="+txnID+"&sql="+url.QueryEscape(sqlA))

	// 3. Insert B (Buffered)
	fmt.Println("3. INSERT B")
	sqlB := "INSERT INTO t VALUES (keyB, valB)"
	req("GET", proxy+"?txn="+txnID+"&sql="+url.QueryEscape(sqlB))

	// 4. Verify Not Visible Yet (Read A)
	fmt.Println("4. SELECT A (Should fail/empty or old value if Isolation works? Our Read doesn't check lock yet?)")
	// Wait, our Shard Read checks locks.
	// If Buffered in Proxy, it is NOT sent to Shard yet.
	// So Shard has NOTHING. Read should return Empty/404.
	// NOTE: Router maps "keyA" to "keyA".
	// "INSERT INTO t..." -> routing logic extracts "keyA".
	// SELECT logic need to target same shard.
	// ScanNode uses broadcast/default.
	// Let's use hint to be sure: &routing_key=keyA
	req("GET", proxy+"?sql="+url.QueryEscape("SELECT * FROM t")+"&routing_key=keyA")

	// 5. Commit
	fmt.Println("5. COMMIT")
	req("GET", proxy+"?sql=COMMIT&txn="+txnID)

	time.Sleep(1 * time.Second)

	// 6. Verify Visible (Read A)
	fmt.Println("6. SELECT A (Should succeed)")
	// Returns JSON? or Text?
	// Shard /execute returns JSON. Proxy forwards.
	req("GET", proxy+"?sql="+url.QueryEscape("SELECT * FROM t")+"&routing_key=keyA")
}




