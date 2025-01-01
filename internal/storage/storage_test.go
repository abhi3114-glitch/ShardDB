package storage

import (
	"bytes"
	"fmt"
	"testing"
)

func TestMemoryStore(t *testing.T) {
	s := NewMemoryStore()
	// defer s.Close()

	k := []byte("key1")
	v := []byte("val1")

	// Put uses Auto-Commit TS=1
	s.Put(k, v)

	// Get (Legacy) uses GetTxn(Max) -> Should see TS=1
	got, err := s.Get(k)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if !bytes.Equal(got, v) {
		t.Errorf("Want %s, got %s", v, got)
	}

	// Scan test
	// Put key2
	s.Put([]byte("key2"), []byte("val2"))

	var foundKeys []string
	s.Scan([]byte("key1"), []byte("key3"), func(k, v []byte) bool {
		// Scan returns MVCC Encoded Keys!
		decodedKey, _ := DecodeKey(k)
		foundKeys = append(foundKeys, string(decodedKey))
		return true
	})

	// Should find key1 and key2
	// Dedupe needed? Yes, if multiple versions.
	// But here we insert once per key.
	// BTree scan order: key1_1, key2_1.

	if len(foundKeys) != 2 {
		t.Errorf("Scan expected 2 keys, got %d: %v", len(foundKeys), foundKeys)
	}
}

func TestMVCCEncoding(t *testing.T) {
	key := []byte("userKey")
	ts := uint64(100)

	encoded := EncodeKey(key, ts)
	decodedKey, decodedTs := DecodeKey(encoded)

	if !bytes.Equal(decodedKey, key) {
		t.Errorf("Decoded key mismatch")
	}
	if decodedTs != ts {
		t.Errorf("Decoded logic error. Want %d, got %d", ts, decodedTs)
	}

	// Order check
	tsOld := uint64(50)
	tsNew := uint64(100)

	encOld := EncodeKey(key, tsOld)
	encNew := EncodeKey(key, tsNew)

	// Inverted: New should be smaller (lexicographically first) if we want Newest First?
	// Wait, standard LSM: Newest First.
	// EncodeKey uses Inverted TS.
	// Max-100 < Max-50.
	// So encNew < encOld.

	if bytes.Compare(encNew, encOld) >= 0 {
		t.Errorf("Expected Newer timestamp to sort BEFORE Older timestamp")
	}
}

func TestMVCCGet(t *testing.T) {
	s := NewMemoryStore()
	key := []byte("accountA")

	// Helper to write version
	write := func(ts uint64, val string) {
		txn := fmt.Sprintf("txn%d", ts)
		s.Prepare(key, []byte(val), txn)
		s.Commit(key, txn, ts)
	}

	write(10, "v10")
	write(20, "v20")
	write(30, "v30")

	tests := []struct {
		readTs uint64
		want   string
	}{
		{5, ""},      // Too old
		{10, "v10"},  // Exact match
		{15, "v10"},  // Should see 10
		{20, "v20"},  // Exact match
		{25, "v20"},  // Should see 20
		{30, "v30"},  // Exact match
		{100, "v30"}, // Future -> see latest
	}

	for _, tt := range tests {
		// Use new GetTxn instead of helper MVCCGet (which uses Scan)
		// Or keep testing generic MVCCGet?
		// GetTxn is the native MVCC read now.
		got, err := s.GetTxn(key, tt.readTs)
		if err != nil {
			t.Errorf("GetTxn error: %v", err)
		}

		if tt.want == "" {
			if got != nil {
				t.Errorf("ReadTs %d: want nil, got %s", tt.readTs, got)
			}
		} else {
			if string(got) != tt.want {
				t.Errorf("ReadTs %d: want %s, got %s", tt.readTs, tt.want, got)
			}
		}
	}
}



