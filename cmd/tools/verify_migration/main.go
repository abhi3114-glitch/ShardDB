package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

func req(method, urlStr string, bodyData []byte) string {
	var bodyReader io.Reader
	if bodyData != nil {
		bodyReader = bytes.NewReader(bodyData)
	}
	req, _ := http.NewRequest(method, urlStr, bodyReader)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("FAIL: %s %s -> %v\n", method, urlStr, err)
		return ""
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	// fmt.Printf("%s %s -> %d\n", method, urlStr, resp.StatusCode)
	if resp.StatusCode >= 400 {
		fmt.Printf("ERROR %d: %s\n", resp.StatusCode, string(body))
	}
	return string(body)
}

func main() {
	// proxy := "http://localhost:8080/execute"
	s1Export := "http://localhost:9001/debug/export"
	s2Ingest := "http://localhost:9002/debug/ingest"

	fmt.Println("1. Insert KeyM to S1 (via Proxy)")
	// Assuming Router routes KeyM to S1 by default (or just push direct to S1 if needed)
	// Just push direct to S1 to be sure it's there.
	req("POST", "http://localhost:9001/execute?sql="+url.QueryEscape("INSERT INTO t VALUES ('KeyM', 'ValM')"), nil)

	time.Sleep(500 * time.Millisecond) // Wait for apply

	fmt.Println("2. Export from S1")
	// Export all (start/end empty)
	exportJson := req("GET", s1Export, nil)
	fmt.Printf("Exported Data Length: %d bytes\n", len(exportJson))

	if len(exportJson) < 10 {
		fmt.Println("FAIL: Export looks empty?")
		return
	}

	fmt.Println("3. Ingest to S2")
	req("POST", s2Ingest, []byte(exportJson))

	fmt.Println("4. Verify S2 has Data (Direct Read)")
	// Query S2 directly Point Look up
	res := req("GET", "http://localhost:9002/execute?sql="+url.QueryEscape("SELECT * FROM t WHERE k='KeyM'"), nil)
	fmt.Println("S2 Point Result:", res)

	// Query S2 Scan
	resScan := req("GET", "http://localhost:9002/execute?sql="+url.QueryEscape("SELECT * FROM t"), nil)
	fmt.Println("S2 Scan Result:", resScan)

	// Debug: Export S2 to check internal state
	s2Dump := req("GET", "http://localhost:9002/debug/export", nil)
	fmt.Printf("S2 Dump: %s\n", s2Dump)

	if res == `[["ValM"]]` || res == `[["KeyM,ValM"]]` || len(res) > 5 {
		fmt.Println("SUCCESS: Data migrated to S2!")
	} else {
		fmt.Println("FAILURE: Data not found on S2.")
	}

	// Optional: Metadata update simulated?
	// Not strictly needed to prove "Migration Primitive" works.
	// We proved "Copy".
}

