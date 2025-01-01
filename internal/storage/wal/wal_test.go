package wal

import (
	"bytes"
	"os"
	"testing"
)

func TestWAL(t *testing.T) {
	tmpFile := "test_wal.log"
	defer os.Remove(tmpFile)

	w, err := Open(tmpFile)
	if err != nil {
		t.Fatalf("Failed to open WAL: %v", err)
	}

	entries := [][]byte{
		[]byte("entry1"),
		[]byte("entry2-longer"),
		[]byte("entry3"),
	}

	for _, e := range entries {
		if err := w.Append(e); err != nil {
			t.Fatalf("Failed to append: %v", err)
		}
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Failed to close WAL: %v", err)
	}

	// Reopen and verify
	w2, err := Open(tmpFile)
	if err != nil {
		t.Fatalf("Failed to reopen WAL: %v", err)
	}
	defer w2.Close()

	var readEntries [][]byte
	err = w2.Iterate(func(data []byte) error {
		// make copy because buffer might be reused if optimization added later (currently not reused but safe)
		d := make([]byte, len(data))
		copy(d, data)
		readEntries = append(readEntries, d)
		return nil
	})
	if err != nil {
		t.Fatalf("Iterate failed: %v", err)
	}

	if len(readEntries) != len(entries) {
		t.Fatalf("Expected %d entries, got %d", len(entries), len(readEntries))
	}

	for i, e := range entries {
		if !bytes.Equal(e, readEntries[i]) {
			t.Errorf("Entry %d mismatch. Want %s, got %s", i, e, readEntries[i])
		}
	}
}


