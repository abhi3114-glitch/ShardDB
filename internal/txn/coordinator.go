package txn

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/myuser/sharddb/internal/router"
)

type TxnState int

const (
	StateActive    TxnState = 0
	StateCommitted TxnState = 1
	StateAborted   TxnState = 2
)

type WriteIntent struct {
	Key   string
	Value string
}

type Transaction struct {
	ID        string
	State     TxnState
	StartTime time.Time
	Writes    []WriteIntent
}

type Coordinator struct {
	mu           sync.RWMutex
	transactions map[string]*Transaction
	router       *router.Router
}

func NewCoordinator(r *router.Router) *Coordinator {
	return &Coordinator{
		transactions: make(map[string]*Transaction),
		router:       r,
	}
}

func (c *Coordinator) Begin() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	id := fmt.Sprintf("txn-%d", time.Now().UnixNano())
	c.transactions[id] = &Transaction{
		ID:        id,
		State:     StateActive,
		StartTime: time.Now(),
		Writes:    make([]WriteIntent, 0),
	}
	return id
}

func (c *Coordinator) AddWrite(txnID, key, value string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	txn, ok := c.transactions[txnID]
	if !ok || txn.State != StateActive {
		return errors.New("transaction not active")
	}
	txn.Writes = append(txn.Writes, WriteIntent{Key: key, Value: value})
	return nil
}

func (c *Coordinator) Commit(txnID string) error {
	c.mu.Lock()
	txn, ok := c.transactions[txnID]
	if !ok || txn.State != StateActive {
		c.mu.Unlock()
		return errors.New("transaction not active")
	}
	// Copy writes to release lock during network calls
	writes := make([]WriteIntent, len(txn.Writes))
	copy(writes, txn.Writes)
	c.mu.Unlock()

	if len(writes) == 0 {
		return nil // No op
	}

	// 1. Group by Shard
	shardWrites := make(map[string][]WriteIntent)
	for _, w := range writes {
		shardAddr := c.router.Locate([]byte(w.Key))
		shardWrites[shardAddr] = append(shardWrites[shardAddr], w)
	}

	// 2. Phase 1: PREPARE (or 1PC)
	if len(shardWrites) == 1 {
		// Optimization: Single Shard 1PC
		for shard, intents := range shardWrites {
			// Construct Batch?
			// MVP: Supports ONE message with all intents?
			// Or just assume one intent per txn for now?
			// The current RPCs /txn/oneshot needs to support list of keys?
			// MVP: Iterate intents? NO. 1PC must be atomic.
			// We need a /txn/batch endpoint logic.
			// For simplicity: If strict 1PC, we bundle.
			ts := uint64(time.Now().UnixNano())
			if err := c.sendOneShot(shard, intents, txnID, ts); err != nil {
				c.Abort(txnID)
				return err
			}
		}

		c.mu.Lock()
		txn.State = StateCommitted
		c.mu.Unlock()
		return nil
	}

	for shard, intents := range shardWrites {
		for _, w := range intents {
			if err := c.sendPrepare(shard, w.Key, w.Value, txnID); err != nil {
				// Abort!
				c.Abort(txnID)
				return fmt.Errorf("prepare failed on %s for key %s: %v", shard, w.Key, err)
			}
		}
	}

	// 3. Phase 2: COMMIT
	// If all Prepared, we Commit.
	ts := uint64(time.Now().UnixNano()) // Use HLC ideally.

	// Update State
	c.mu.Lock()
	txn.State = StateCommitted
	c.mu.Unlock()

	for shard, intents := range shardWrites {
		for _, w := range intents {
			// Async commit? Or Sync?
			// Ideally Async (best effort).
			go c.sendCommit(shard, w.Key, txnID, ts)
		}
	}

	return nil
}

func (c *Coordinator) Abort(txnID string) error {
	c.mu.Lock()
	txn, ok := c.transactions[txnID]
	if !ok {
		c.mu.Unlock()
		return errors.New("unknown txn")
	}
	txn.State = StateAborted
	writes := make([]WriteIntent, len(txn.Writes))
	copy(writes, txn.Writes)
	c.mu.Unlock()

	// Rollback any potential locks (best effort)
	for _, w := range writes {
		shardAddr := c.router.Locate([]byte(w.Key))
		go c.sendAbort(shardAddr, w.Key, txnID)
	}
	return nil
}

// Helpers
func (c *Coordinator) sendPrepare(shard, key, val, txnID string) error {
	// Construct URL: http://shard/txn/prepare?key=...
	// Shard Address from Router is "http://host:port" or just "host:port"?
	// Router uses "http://shard-node-X:900X".
	u := fmt.Sprintf("%s/txn/prepare?key=%s&val=%s&txn=%s", shard, url.QueryEscape(key), url.QueryEscape(val), txnID)
	resp, err := http.Get(u)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}

func (c *Coordinator) sendCommit(shard, key, txnID string, ts uint64) {
	u := fmt.Sprintf("%s/txn/commit?key=%s&txn=%s&ts=%d", shard, url.QueryEscape(key), txnID, ts)
	http.Get(u)
}

func (c *Coordinator) sendAbort(shard, key, txnID string) {
	u := fmt.Sprintf("%s/txn/abort?key=%s&txn=%s", shard, url.QueryEscape(key), txnID)
	http.Get(u)
}

func (c *Coordinator) sendOneShot(shard string, intents []WriteIntent, txnID string, ts uint64) error {
	type Op struct {
		Key string `json:"key"`
		Val string `json:"val"`
	}
	type Req struct {
		TxnID string `json:"txn"`
		Ts    uint64 `json:"ts"`
		Ops   []Op   `json:"ops"`
	}

	reqBody := Req{TxnID: txnID, Ts: ts, Ops: make([]Op, len(intents))}
	for i, w := range intents {
		reqBody.Ops[i] = Op{Key: w.Key, Val: w.Value}
	}

	data, _ := json.Marshal(reqBody)
	resp, err := http.Post(fmt.Sprintf("%s/txn/oneshot", shard), "application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("oneshot status %d", resp.StatusCode)
	}
	return nil
}
