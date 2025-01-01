package storage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/google/btree"
)

// MemoryStore implements Engine.
type MemoryStore struct {
	mu   sync.RWMutex
	tree *btree.BTree

	// LockTable for Active Transactions (Intents)
	// Key -> TxnID (Exclusive Lock)
	// Also store value? Yes.
	locks map[string]Lock
}

type Lock struct {
	TxnID string
	Value []byte
}

type item struct {
	key   []byte
	value []byte
}

func (i *item) Less(than btree.Item) bool {
	return bytes.Compare(i.key, than.(*item).key) < 0
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		tree:  btree.New(32),
		locks: make(map[string]Lock),
	}
}

// Prepare acquires a lock and stores the intent value.
func (s *MemoryStore) Prepare(key []byte, value []byte, txnID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	k := string(key)

	// Check if locked by other
	if lock, ok := s.locks[k]; ok {
		if lock.TxnID != txnID {
			return fmt.Errorf("key %s locked by %s", k, lock.TxnID)
		}
		// Idempotent (upgrade?) overwriting own lock is fine
	}

	// Acquire Lock
	s.locks[k] = Lock{
		TxnID: txnID,
		Value: value,
	}
	return nil
}

// Commit moves the intent to the MVCC tree at commitTs.
func (s *MemoryStore) Commit(key []byte, txnID string, commitTs uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	k := string(key)
	lock, ok := s.locks[k]
	if !ok {
		// Possibly already committed? or never locked?
		// For 2PC correctness, we expect lock to exist.
		// If retrying commit, might be gone.
		// Assume success if gone? Or error?
		// Stick to strict for now.
		return fmt.Errorf("no lock on key %s", k)
	}

	if lock.TxnID != txnID {
		return fmt.Errorf("key %s locked by %s, cannot commit %s", k, lock.TxnID, txnID)
	}

	// Write to MVCC
	encodedKey := EncodeKey(key, commitTs)
	s.tree.ReplaceOrInsert(&item{key: encodedKey, value: lock.Value})

	// Release Lock
	delete(s.locks, k)
	return nil
}

// Abort removes the intent.
func (s *MemoryStore) Abort(key []byte, txnID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	k := string(key)
	lock, ok := s.locks[k]
	if !ok {
		return nil // Already aborted or committed
	}

	if lock.TxnID != txnID {
		return fmt.Errorf("key %s locked by %s, cannot abort %s", k, lock.TxnID, txnID)
	}

	delete(s.locks, k)
	return nil
}

type BatchOp struct {
	Key   []byte
	Value []byte
}

// OnePhaseCommit atomically Prepares and Commits multiple keys.
func (s *MemoryStore) OnePhaseCommit(ops []BatchOp, txnID string, commitTs uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. Validate (Prepare Phase)
	// Check all locks
	for _, op := range ops {
		k := string(op.Key)
		if lock, ok := s.locks[k]; ok {
			if lock.TxnID != txnID {
				return fmt.Errorf("key %s locked by %s", k, lock.TxnID)
			}
		}
	}

	// 2. Commit Phase
	// Write directly to MVCC (Skipping Lock map inserts since we hold s.mu)
	for _, op := range ops {
		encodedKey := EncodeKey(op.Key, commitTs)
		s.tree.ReplaceOrInsert(&item{key: encodedKey, value: op.Value})

		// Clean up any potential lock (if we're retrying or upgrading?)
		delete(s.locks, string(op.Key))
	}

	return nil
}

// GetSnapshotData serializes the entire store state.
func (s *MemoryStore) GetSnapshotData() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// Simple JSON dump?
	// BTree iteration to map?
	data := make(map[string][]byte)
	s.tree.Ascend(func(i btree.Item) bool {
		it := i.(*item)
		data[string(it.key)] = it.value
		return true
	})
	// Serialize map
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	if err := enc.Encode(data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// RestoreSnapshot restores state from data.
func (s *MemoryStore) RestoreSnapshot(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var kvs map[string][]byte
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&kvs); err != nil {
		return err
	}

	s.tree.Clear(false)
	for k, v := range kvs {
		s.tree.ReplaceOrInsert(&item{key: []byte(k), value: v})
	}
	return nil
}

// GetTxn reads at a timestamp, respecting locks.
func (s *MemoryStore) GetTxn(key []byte, readTs uint64) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	k := string(key)

	// 1. Check Locks
	if lock, ok := s.locks[k]; ok {
		// If locked, and we are NOT the owner. Return Error "Locked"
		return nil, fmt.Errorf("key %s locked by %s", k, lock.TxnID)
	}

	// 2. MVCC Read
	val, exists := s.mvccGetLocked(key, readTs)
	if !exists {
		return nil, nil // Not found
	}
	return val, nil
}

// mvccGetLocked helper (assumes lock held)
func (s *MemoryStore) mvccGetLocked(key []byte, readTs uint64) ([]byte, bool) {
	seekKey := EncodeKey(key, readTs)

	var foundVal []byte
	found := false

	s.tree.AscendGreaterOrEqual(&item{key: seekKey}, func(i btree.Item) bool {
		it := i.(*item)
		decodedKey, _ := DecodeKey(it.key) // We need DecodeKey helper available

		if !bytes.Equal(decodedKey, key) {
			return false // Different key, stop
		}

		// Found!
		foundVal = it.value
		found = true
		return false // Stop
	})

	return foundVal, found
}

// Legacy methods wrappers
func (s *MemoryStore) Put(key, value []byte) error {
	// Auto-commit legacy.
	txn := "legacy"
	if err := s.Prepare(key, value, txn); err != nil {
		return err
	}
	return s.Commit(key, txn, 1)
}

func (s *MemoryStore) Get(key []byte) ([]byte, error) {
	return s.GetTxn(key, ^uint64(0)) // Read Latest (MaxUint64)
}

func (s *MemoryStore) Close() error {
	return nil
}

// RunGC removes versions older than safeTs, keeping at least one version <= safeTs.
func (s *MemoryStore) RunGC(safeTs uint64) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	deletedCount := 0
	var keysToDelete []btree.Item

	// We can't delete while iterating easily in BTree with Ascend (safe? likely not).
	// Strategy: Collect keys to delete.
	// We need to group by UserKey.
	// Iteration order: UserKey Asc, TS Desc (Inverted TS).
	// So for a given UserKey:
	// 1. Ver(100)
	// 2. Ver(80)
	// 3. Ver(10)
	// If safeTs = 50.
	// 100 > 50. Keep.
	// 80 > 50. Keep.
	// 10 <= 50. Keep (First valid <= safeTs).
	// If there was Ver(5). 5 <= 50. Delete (Shadowed by 10).

	// Implementation:
	// Iterate.
	// Track current UserKey.
	// Track if we have found a "valid snapshot version" for this key (<= safeTs).
	// If we find another version <= safeTs AND we have one, DELETE it.

	type traversalState struct {
		currentUserKey []byte
		foundSnapshot  bool
	}
	state := &traversalState{}

	s.tree.Ascend(func(i btree.Item) bool {
		it := i.(*item)
		userKey, ts := DecodeKey(it.key)

		// Check if new key
		if !bytes.Equal(userKey, state.currentUserKey) {
			state.currentUserKey = userKey
			state.foundSnapshot = false
		}

		if ts <= safeTs {
			if !state.foundSnapshot {
				// This is the latest version <= safeTs (Snapshot Version). Keep it.
				state.foundSnapshot = true
			} else {
				// Already have a snapshot version. This is older history. GC it.
				keysToDelete = append(keysToDelete, i)
			}
		}
		// If ts > safeTs, keep it (future version relative to GC).

		return true
	})

	for _, k := range keysToDelete {
		s.tree.Delete(k)
		deletedCount++
	}

	return deletedCount, nil
}

func (s *MemoryStore) Scan(start, end []byte, iter func(key, value []byte) bool) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	s.tree.AscendGreaterOrEqual(&item{key: EncodeKey(start, ^uint64(0))}, func(i btree.Item) bool {
		it := i.(*item)
		k, _ := DecodeKey(it.key)
		if end != nil && bytes.Compare(k, end) >= 0 {
			return false
		}
		return iter(it.key, it.value)
	})
	return nil
}

// ScanRange exports all raw keys and values in the range [start, end).
// Returns a map of RawEncodedKey -> Value.
func (s *MemoryStore) ScanRange(start, end []byte) (map[string][]byte, error) {
	data := make(map[string][]byte)
	err := s.Scan(start, end, func(k, v []byte) bool {
		data[string(k)] = v
		return true
	})
	return data, err
}

// Ingest blindly writes raw keys and values to the store.
func (s *MemoryStore) Ingest(data map[string][]byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range data {
		s.tree.ReplaceOrInsert(&item{key: []byte(k), value: v})
	}
	return nil
}




