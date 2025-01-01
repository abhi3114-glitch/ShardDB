package main

import (
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	concurrency := flag.Int("concurrency", 10, "Number of concurrent workers")
	duration := flag.Duration("duration", 10*time.Second, "Test duration")
	proxyAddr := flag.String("proxy", "http://localhost:8080/execute", "Proxy URL")
	flag.Parse()

	fmt.Printf("Starting Benchmark: %d workers, %v duration, target %s\n", *concurrency, *duration, *proxyAddr)

	var ops int64
	var errors int64
	start := time.Now()

	var wg sync.WaitGroup
	ctxDone := make(chan struct{})

	// Timer to stop
	go func() {
		time.Sleep(*duration)
		close(ctxDone)
	}()

	// Custom Client with Timeout
	client := &http.Client{Timeout: 5 * time.Second}

	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for {
				select {
				case <-ctxDone:
					return
				default:
					// Workload: 50% Insert, 50% Select
					key := fmt.Sprintf("user%d", rand.Intn(10000))
					val := fmt.Sprintf("val%d", rand.Intn(1000))

					var sql string
					if rand.Float32() < 0.5 {
						sql = fmt.Sprintf("INSERT INTO t VALUES ('%s', '%s')", key, val)
					} else {
						sql = fmt.Sprintf("SELECT * FROM t WHERE k='%s'", key)
					}

					resp, err := client.Get(*proxyAddr + "?sql=" + url.QueryEscape(sql))
					if err != nil || resp.StatusCode >= 400 {
						newErrCount := atomic.AddInt64(&errors, 1)
						if newErrCount <= 5 {
							errStr := "status code"
							if err != nil {
								errStr = err.Error()
							}
							fmt.Printf("Error: %v (Status: %v)\n", errStr, resp)
						}
					} else {
						atomic.AddInt64(&ops, 1)
						resp.Body.Close()
					}
				}
			}
		}(i)
	}

	wg.Wait()
	elapsed := time.Since(start)

	fmt.Println("Benchmark Finished.")
	fmt.Printf("Total Ops: %d\n", ops)
	fmt.Printf("Errors: %d\n", errors)
	fmt.Printf("Duration: %v\n", elapsed)
	fmt.Printf("RPS: %.2f\n", float64(ops)/elapsed.Seconds())
}



