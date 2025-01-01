package raft

import (
	"context"
	"log"
	"time"

	"go.etcd.io/etcd/raft/v3"
	"go.etcd.io/etcd/raft/v3/raftpb"
)

// Node wraps etcd/raft.Node to provide a simpler interface.
type Node struct {
	ID        uint64
	RaftNode  raft.Node
	Storage   Storage // Interface!
	Transport Transport
	// Applier applies committed entries to the state machine
	Applier Applier
}

// Storage interface for raft persistence (subset of DiskStorage methods needed)
type Storage interface {
	raft.Storage
	Save(entries []raftpb.Entry, state raftpb.HardState) error
	CreateSnapshot(i uint64, cs *raftpb.ConfState, data []byte) (raftpb.Snapshot, error)
	ApplySnapshot(snap raftpb.Snapshot) error
	Close() error
}

type Applier interface {
	Apply(entry raftpb.Entry)
	GetSnapshot() ([]byte, error)
	Restore(data []byte) error
}

type Transport interface {
	Send(msgs []raftpb.Message)
}

// Config for the Node
type Config struct {
	ID      uint64
	Peers   []uint64
	WALPath string // New: Path to WAL file
}

// NewNode creates a new Raft node.
func NewNode(cfg Config, applier Applier, transport Transport) (*Node, error) {
	// Storage setup
	var storage Storage
	if cfg.WALPath != "" {
		ds, err := NewDiskStorage(cfg.WALPath)
		if err != nil {
			return nil, err
		}
		storage = ds
	} else {
		storage = &memoryStorageWrapper{raft.NewMemoryStorage()}
	}

	c := &raft.Config{
		ID:              cfg.ID,
		ElectionTick:    10,
		HeartbeatTick:   1,
		Storage:         storage,
		MaxSizePerMsg:   4096,
		MaxInflightMsgs: 256,
	}

	var peers []raft.Peer
	for _, p := range cfg.Peers {
		peers = append(peers, raft.Peer{ID: p})
	}

	var rn raft.Node

	// Check if existing state
	_, err := storage.FirstIndex()
	if err != nil {
		return nil, err
	}

	lastIndex, _ := storage.LastIndex()

	if lastIndex > 0 {
		rn = raft.RestartNode(c)
	} else {
		rn = raft.StartNode(c, peers)
	}

	return &Node{
		ID:        cfg.ID,
		RaftNode:  rn,
		Storage:   storage,
		Transport: transport,
		Applier:   applier,
	}, nil
}

// memoryStorageWrapper makes MemoryStorage satisfy our Storage interface (Save method)
type memoryStorageWrapper struct {
	*raft.MemoryStorage
}

func (m *memoryStorageWrapper) Save(entries []raftpb.Entry, state raftpb.HardState) error {
	m.Append(entries)
	if !raft.IsEmptyHardState(state) {
		m.SetHardState(state)
	}
	return nil
}
func (m *memoryStorageWrapper) Close() error { return nil }

// wrapper inherits CreateSnapshot/ApplySnapshot from raft.MemoryStorage

// Run starts the main loop. blocking.
func (n *Node) Run(ctx context.Context) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			n.RaftNode.Stop()
			return
		case <-ticker.C:
			n.RaftNode.Tick()
		case rd := <-n.RaftNode.Ready():
			// 1. Save
			if err := n.Storage.Save(rd.Entries, rd.HardState); err != nil {
				log.Fatalf("Failed to persist raft state: %v", err)
			}

			// 2. Send messages to peers
			n.Transport.Send(rd.Messages)

			// 3. Apply committed entries
			for _, entry := range rd.CommittedEntries {
				if entry.Type == raftpb.EntryNormal && len(entry.Data) > 0 {
					n.Applier.Apply(entry)
				}
			}

			// Snapshot Check
			if len(rd.CommittedEntries) > 0 {
				lastApplied := rd.CommittedEntries[len(rd.CommittedEntries)-1].Index
				fi, _ := n.Storage.FirstIndex()
				if lastApplied > fi && lastApplied-fi > 50 {
					// log.Printf("Triggering Snapshot at %d", lastApplied)
					data, err := n.Applier.GetSnapshot()
					if err == nil {
						cs := &raftpb.ConfState{Voters: []uint64{1, 2, 3}}
						_, err := n.Storage.CreateSnapshot(lastApplied, cs, data)
						if err != nil {
							log.Printf("Snapshot create error: %v", err)
						} else {
							log.Printf("Snapshot created at %d", lastApplied)
						}
					}
				}
			}

			if !raft.IsEmptySnap(rd.Snapshot) {
				log.Printf("Applying Snapshot index %d", rd.Snapshot.Metadata.Index)
				if err := n.Storage.ApplySnapshot(rd.Snapshot); err != nil {
					log.Printf("ApplySnapshot failed: %v", err)
				}
				if err := n.Applier.Restore(rd.Snapshot.Data); err != nil {
					log.Printf("Restore failed: %v", err)
				}
			}

			// 4. Advance
			n.RaftNode.Advance()
		}
	}
}

func (n *Node) Propose(ctx context.Context, data []byte) error {
	return n.RaftNode.Propose(ctx, data)
}

func (n *Node) Step(ctx context.Context, msg raftpb.Message) error {
	return n.RaftNode.Step(ctx, msg)
}





