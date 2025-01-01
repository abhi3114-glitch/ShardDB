package meta

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"go.etcd.io/etcd/raft/v3/raftpb"
)

// Range definition: [Start, End)
type Range struct {
	StartKey []byte
	EndKey   []byte // Empty/Nil means positive infinity? Or 0xFF?
	// Using empty as -infinity for Start, and empty as +infinity for End is common.
	// Let's use specific bytes for now or nil.
	ShardID uint64
}

// ShardConfig
type ShardConfig struct {
	ID    uint64
	Addrs []string // URLs of replicas
}

// MetaStore holds the cluster state.
type MetaStore struct {
	mu     sync.RWMutex
	Ranges []Range
	Shards map[uint64]ShardConfig
}

func NewMetaStore() *MetaStore {
	// Initial state: One range covering everything mapped to Shard 1.
	// Start: nil (min), End: nil (max)
	return &MetaStore{
		Ranges: []Range{
			{StartKey: nil, EndKey: nil, ShardID: 1},
		},
		Shards: make(map[uint64]ShardConfig),
	}
}

// GetShard returns the ShardID for a given key.
func (s *MetaStore) GetShard(key []byte) uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Binary search or linear scan (Range list is small)
	// Ranges are sorted by StartKey.
	// Find the last range where StartKey <= key.
	// We want the insertion point.

	idx := sort.Search(len(s.Ranges), func(i int) bool {
		// return true if Ranges[i].StartKey > key
		r := s.Ranges[i]
		if r.StartKey == nil {
			return false
		}
		return bytes.Compare(r.StartKey, key) > 0
	})

	// idx is the first range STARTS AFTER key.
	// So required range is idx-1.
	if idx == 0 {
		if s.Ranges[0].StartKey == nil {
			return s.Ranges[0].ShardID
		}
		// Should not happen if we cover full space
		return 0
	}

	return s.Ranges[idx-1].ShardID
}

// Apply implements raft.Applier
func (s *MetaStore) Apply(entry raftpb.Entry) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Command definition
	type Command struct {
		Op       string
		Range    Range
		Shard    ShardConfig
		SplitKey []byte
		ShardID1 uint64
		ShardID2 uint64
	}

	var cmd Command
	if err := json.Unmarshal(entry.Data, &cmd); err != nil {
		fmt.Printf("Apply failed: %v\n", err)
		return
	}

	switch cmd.Op {
	case "REGISTER_SHARD":
		s.Shards[cmd.Shard.ID] = cmd.Shard
	case "UPDATE_RANGE":
		// Naive implementation: just append/update.
	case "SPLIT_RANGE":
		// Find range containing SplitKey
		key := cmd.SplitKey
		// Logic similar to GetShard
		idx := sort.Search(len(s.Ranges), func(i int) bool {
			r := s.Ranges[i]
			if r.StartKey == nil {
				return false
			}
			return bytes.Compare(r.StartKey, key) > 0
		})

		targetIdx := 0
		if idx > 0 {
			targetIdx = idx - 1
		}

		targetRange := s.Ranges[targetIdx]

		// Validate SplitKey is within targetRange (Start < Key < End)
		// Start check
		if targetRange.StartKey != nil && bytes.Compare(key, targetRange.StartKey) <= 0 {
			fmt.Printf("SplitKey %s <= StartKey %s\n", key, targetRange.StartKey)
			return
		}
		// End check
		if targetRange.EndKey != nil && bytes.Compare(key, targetRange.EndKey) >= 0 {
			fmt.Printf("SplitKey %s >= EndKey %s\n", key, targetRange.EndKey)
			return
		}

		// DO SPLIT
		// New Range 1: [Start, Key) -> ShardID1
		// New Range 2: [Key, End) -> ShardID2

		r1 := Range{StartKey: targetRange.StartKey, EndKey: key, ShardID: cmd.ShardID1}
		r2 := Range{StartKey: key, EndKey: targetRange.EndKey, ShardID: cmd.ShardID2}

		// Replace targetRange with r1, r2
		// Efficient slice insertion?
		// Append r2 to end, replace target with r1, then sort?
		// Or insert.

		s.Ranges[targetIdx] = r1
		// Insert r2 at targetIdx+1
		s.Ranges = append(s.Ranges, Range{})
		copy(s.Ranges[targetIdx+2:], s.Ranges[targetIdx+1:])
		s.Ranges[targetIdx+1] = r2

		// Sort is maintained if we insert correctly?
		// Yes, because r2.StartKey (Key) > r1.StartKey (Start).
		// And Key < End. Next Range Start >= End > Key.
		// So order preserved.

		fmt.Printf("Split Range at %s -> Shards %d, %d\n", key, cmd.ShardID1, cmd.ShardID2)

	default:
		fmt.Printf("Unknown op: %s\n", cmd.Op)
	}
}

// GetSnapshot implements raft.Applier
func (s *MetaStore) GetSnapshot() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// Serialize Ranges and Shards
	type state struct {
		Ranges []Range
		Shards map[uint64]ShardConfig
	}
	st := state{
		Ranges: s.Ranges,
		Shards: s.Shards,
	}
	return json.Marshal(st)
}

// Restore implements raft.Applier
func (s *MetaStore) Restore(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	type state struct {
		Ranges []Range
		Shards map[uint64]ShardConfig
	}
	var st state
	if err := json.Unmarshal(data, &st); err != nil {
		return err
	}
	s.Ranges = st.Ranges
	s.Shards = st.Shards
	return nil
}


