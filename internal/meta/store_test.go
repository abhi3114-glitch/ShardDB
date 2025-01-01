package meta

import (
	"testing"
)

func TestGetShard(t *testing.T) {
	s := NewMetaStore()

	// Default: everything -> 1
	if id := s.GetShard([]byte("abc")); id != 1 {
		t.Errorf("Default: want 1, got %d", id)
	}

	// Add splits manually (simulating UPDATE_RANGE)
	// Range 1: nil -> "m" (Shard 1)
	// Range 2: "m" -> nil (Shard 2)

	s.Ranges = []Range{
		{StartKey: nil, EndKey: []byte("m"), ShardID: 1},
		{StartKey: []byte("m"), EndKey: nil, ShardID: 2},
	}

	tests := []struct {
		key  string
		want uint64
	}{
		{"a", 1},
		{"l", 1},
		{"m", 2}, // inclusive start
		{"z", 2},
	}

	for _, tt := range tests {
		if got := s.GetShard([]byte(tt.key)); got != tt.want {
			t.Errorf("Key %s: want %d, got %d", tt.key, tt.want, got)
		}
	}
}




