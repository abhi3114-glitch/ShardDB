package router

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/myuser/sharddb/internal/meta"
)

// Router caches metadata and routes requests.
type Router struct {
	mu      sync.RWMutex
	MetaURL string
	// Cache
	ranges []meta.Range // Sorted
	// Shard Clients?
	// For MVP: Simple HTTP client and hardcoded or discovered mapping.
	// Let's assume sharding 1,2,3 map to known ports for dev environment if not in meta?
	// Real system: Meta provides Shard->Addrs.
	ShardAddrs map[uint64]string
}

func NewRouter(metaURL string) *Router {
	return &Router{
		MetaURL: metaURL,
		// Seed with initial dev setup
		ranges: []meta.Range{
			{StartKey: nil, EndKey: nil, ShardID: 1},
		},
		ShardAddrs: map[uint64]string{
			1: "http://localhost:9001", // Default dev ports
			2: "http://localhost:9002",
			3: "http://localhost:9003",
		},
	}
}

// Refresh fetches latest mapping from Meta-Node.
// MVP: No-op or simple fetch.
func (r *Router) Refresh() error {
	// TODO: Fetch from r.MetaURL + "/routes"
	return nil
}

// GetShardID finds the shard for a key.
// Duplicates logic from MetaStore for client-side routing.
func (r *Router) GetShardID(key []byte) uint64 {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Linear scan for MVP (small range count)
	for _, rng := range r.ranges {
		// Check start
		if rng.StartKey != nil && string(key) < string(rng.StartKey) {
			continue
		}
		// Check end
		if rng.EndKey != nil && string(key) >= string(rng.EndKey) {
			continue
		}
		return rng.ShardID
	}
	return 1 // Default
}

// Locate returns the address of the shard responsible for the key.
func (r *Router) Locate(key []byte) string {
	id := r.GetShardID(key)
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.ShardAddrs[id]
}

// ForwardRequest forwards a raw SQL request to the shard.
// Used for simple "ExecuteSQL" commands.
func (r *Router) ForwardRequest(shardID uint64, sql string) (string, error) {
	r.mu.RLock()
	addr, ok := r.ShardAddrs[shardID]
	r.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("unknown shard %d", shardID)
	}

	// Determine endpoint.
	// Current Shard Node doesn't expose generic SQL execute via HTTP yet.
	// Assuming we add "/execute" to shard node or use Propose/Get directly.
	// For Phase 3 Verification: Use /propose for Write, /get for Read.
	// Logic:
	// If INSERT -> /propose
	// If SELECT -> /get
	// This "SQL Proxy" is primitive.

	// Determine shard URL.
	url := fmt.Sprintf("%s/execute", addr) // Shard Node now has unified /execute endpoint!

	// Create Request
	req, err := http.NewRequest("POST", url, strings.NewReader(sql))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "text/plain")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("shard error (%d): %s", resp.StatusCode, string(body))
	}

	return string(body), nil
}



