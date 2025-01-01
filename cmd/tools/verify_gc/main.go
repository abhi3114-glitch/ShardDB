package main

import (
	"fmt"

	"github.com/myuser/sharddb/internal/storage"
)

// Unit test for GC logic directly against MemoryStore
// (Integration test via RPC is harder without exposing Debug GC API)
func main() {
	s := storage.NewMemoryStore()

	key := []byte("keyA")

	// Insert Version 10
	s.Prepare(key, []byte("val10"), "txn1")
	s.Commit(key, "txn1", 10)

	// Insert Version 20
	s.Prepare(key, []byte("val20"), "txn2")
	s.Commit(key, "txn2", 20)

	// Insert Version 30
	s.Prepare(key, []byte("val30"), "txn3")
	s.Commit(key, "txn3", 30)

	fmt.Println("Initial State Created.")

	// Verify current read at 25 gets 20
	val, _ := s.GetTxn(key, 25)
	if string(val) != "val20" {
		fmt.Printf("FAIL: Pre-GC Read 25 expected val20, got %s\n", val)
		return
	}

	// Run GC(25)
	// Should keep 20 (Latest <= 25).
	// Should delete 10 (Shadowed by 20).
	// Should keep 30 (Future).

	fmt.Println("Running GC(25)...")
	deleted, _ := s.RunGC(25)
	fmt.Printf("Deleted items: %d\n", deleted)

	// Verify 10 is gone?
	// GetTxn(key, 15) -> Should find nothing? Or 20?
	// MemoryStore Read: Ascend >= Encode(Key, 15).
	// If 10 is gone, 20 is > 15 (Inverted logic: 20 < 15? No).
	// InvertedTS: Max-20 < Max-15.
	// So 20 comes BEFORE 15.
	// So we see 20. 20 <= 15 is FALSE.
	// 30. 30 <= 15 FALSE.
	// We see nothing.
	// Correct.

	val15, _ := s.GetTxn(key, 15)
	if val15 != nil {
		fmt.Printf("FAIL: Read 15 expected nil (deleted), got %s\n", val15)
	} else {
		fmt.Println("PASS: Version 10 is gone.")
	}

	val25, _ := s.GetTxn(key, 25)
	if string(val25) != "val20" {
		fmt.Printf("FAIL: Read 25 expected val20, got %s\n", val25)
	} else {
		fmt.Println("PASS: Version 20 remained.")
	}

	val35, _ := s.GetTxn(key, 35)
	if string(val35) != "val30" {
		fmt.Printf("FAIL: Read 35 expected val30, got %s\n", val35)
	} else {
		fmt.Println("PASS: Version 30 remained.")
	}
}


