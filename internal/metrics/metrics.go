package metrics

import (
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
)

// Global Registry using sync.Map for specific thread-safety on retrieval
// Keys are strings, Values are *int64
var registry sync.Map

// Inc increments a counter by 1.
func Inc(name string) {
	Add(name, 1)
}

// Add adds delta to a counter.
func Add(name string, delta int64) {
	val, ok := registry.Load(name)
	if !ok {
		newVal := new(int64)
		val, _ = registry.LoadOrStore(name, newVal)
	}
	atomic.AddInt64(val.(*int64), delta)
}

// Get returns the current value of a counter.
func Get(name string) int64 {
	val, ok := registry.Load(name)
	if !ok {
		return 0
	}
	return atomic.LoadInt64(val.(*int64))
}

// Handler is an HTTP handler that exposes all metrics as JSON.
func Handler(w http.ResponseWriter, r *http.Request) {
	snapshot := make(map[string]int64)
	registry.Range(func(key, value any) bool {
		snapshot[key.(string)] = atomic.LoadInt64(value.(*int64))
		return true
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(snapshot)
}



