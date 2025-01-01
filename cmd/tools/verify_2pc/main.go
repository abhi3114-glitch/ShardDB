package main

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

func req(method, url string) {
	resp, err := http.Get(url)
	if err != nil {
		fmt.Printf("%s %s -> Error: %v\n", method, url, err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("%s %s -> %d: %s\n", method, url, resp.StatusCode, string(body))
}

func main() {
	// 1. Prepare
	fmt.Println("1. Sending PREPARE...")
	req("GET", "http://127.0.0.1:9001/txn/prepare?key=t:test2pc&val=myvalue&txn=txverify")

	time.Sleep(1 * time.Second)

	// 2. Get (Expect Fail/Locked)
	fmt.Println("2. Reading (Expect Locked)...")
	// Legacy /get removed. Using /execute?sql=SELECT...
	// Expect 404/Empty or "Not Found" logic depending on impl?
	// Actually, if using /execute, it returns rows.
	// If locked, what does PointGet return?
	// In MVCC, if Locked, PointGet waits or returns error?
	// Our engine is optimistic/simple.
	// If locked by *another* txn, we might see old version or nothing.
	// We want to see if it sees the NEW value.
	// It should NOT see new value yet.
	req("GET", "http://127.0.0.1:9001/execute?sql=SELECT+*+FROM+t+WHERE+k='test2pc'")

	// 3. Commit
	fmt.Println("3. Sending COMMIT...")
	req("GET", "http://127.0.0.1:9001/txn/commit?key=t:test2pc&txn=txverify&ts=2000")

	time.Sleep(1 * time.Second)

	// 4. Get (Expect Success)
	fmt.Println("4. Reading (Expect Success)...")
	req("GET", "http://127.0.0.1:9001/execute?sql=SELECT+*+FROM+t+WHERE+k='test2pc'")
}

