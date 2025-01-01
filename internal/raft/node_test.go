package raft

import (
	"context"
	"testing"
	"time"

	"go.etcd.io/etcd/raft/v3/raftpb"
)

type mockTransport struct {
	msgs []raftpb.Message
}

func (m *mockTransport) Send(msgs []raftpb.Message) {
	m.msgs = append(m.msgs, msgs...)
}

type mockApplier struct {
	committed [][]byte
}

func (m *mockApplier) Apply(entry raftpb.Entry) {
	m.committed = append(m.committed, entry.Data)
}

func (m *mockApplier) GetSnapshot() ([]byte, error) {
	return nil, nil // Dummy
}

func (m *mockApplier) Restore(data []byte) error {
	return nil // Dummy
}

func TestRaftNode_SingleNode(t *testing.T) {
	// Single node cluster (ID 1)
	cfg := Config{
		ID:    1,
		Peers: []uint64{1},
	}

	transport := &mockTransport{}
	applier := &mockApplier{}

	node, err := NewNode(cfg, applier, transport)
	if err != nil {
		t.Fatalf("NewNode failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start run loop in background
	go node.Run(ctx)

	// Propose a value
	data := []byte("hello raft")
	err = node.Propose(ctx, data)
	if err != nil {
		t.Fatalf("Propose failed: %v", err)
	}

	// Wait for commit (tick a few times)
	// Single node should elect itself leader quickly.
	// 2 seconds timeout
	deadline := time.Now().Add(2 * time.Second)
	found := false
	for time.Now().Before(deadline) {
		if len(applier.committed) > 0 {
			found = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if !found {
		t.Fatal("Timeout waiting for entry to apply")
	}

	if string(applier.committed[0]) != string(data) {
		t.Errorf("Expected committed data %s, got %s", data, applier.committed[0])
	}
}

