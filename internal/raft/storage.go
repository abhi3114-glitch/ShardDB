package raft

import (
	"encoding/json"

	"github.com/myuser/sharddb/internal/storage/wal"
	"go.etcd.io/etcd/raft/v3"
	"go.etcd.io/etcd/raft/v3/raftpb"
)

// DiskStorage wraps MemoryStorage and persists to WAL.
type DiskStorage struct {
	*raft.MemoryStorage
	wal *wal.WAL
}

func NewDiskStorage(walPath string) (*DiskStorage, error) {
	w, err := wal.Open(walPath)
	if err != nil {
		return nil, err
	}

	mem := raft.NewMemoryStorage()
	ds := &DiskStorage{
		MemoryStorage: mem,
		wal:           w,
	}

	// Replay WAL
	err = w.Iterate(func(data []byte) error {
		var r Record
		if err := json.Unmarshal(data, &r); err != nil {
			return err
		}

		if r.Type == RecordEntry {
			var ent raftpb.Entry
			if err := ent.Unmarshal(r.Data); err != nil {
				return err
			}
			mem.Append([]raftpb.Entry{ent})
		} else if r.Type == RecordHardState {
			var hs raftpb.HardState
			if err := hs.Unmarshal(r.Data); err != nil {
				return err
			}
			mem.SetHardState(hs)
		} else if r.Type == RecordSnapshot {
			var snap raftpb.Snapshot
			if err := snap.Unmarshal(r.Data); err != nil {
				return err
			}
			mem.ApplySnapshot(snap)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return ds, nil
}

// Record types for WAL
const (
	RecordEntry     = 1
	RecordHardState = 2
	RecordSnapshot  = 3
)

type Record struct {
	Type int
	Data []byte
}

func (ds *DiskStorage) Save(entries []raftpb.Entry, state raftpb.HardState) error {
	// Persist Entries
	for _, ent := range entries {
		b, err := ent.Marshal()
		if err != nil {
			return err
		}
		if err := ds.writeRecord(RecordEntry, b); err != nil {
			return err
		}
	}

	// Persist HardState
	if !raft.IsEmptyHardState(state) {
		b, err := state.Marshal()
		if err != nil {
			return err
		}
		if err := ds.writeRecord(RecordHardState, b); err != nil {
			return err
		}
	}

	// Update Memory
	ds.MemoryStorage.Append(entries)
	if !raft.IsEmptyHardState(state) {
		ds.MemoryStorage.SetHardState(state)
	}

	return nil
}

func (ds *DiskStorage) writeRecord(typ int, data []byte) error {
	r := Record{Type: typ, Data: data}
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	return ds.wal.Append(b)
}

func (ds *DiskStorage) Close() error {
	return ds.wal.Close()
}

// Snapshot Persistence

func (ds *DiskStorage) CreateSnapshot(i uint64, cs *raftpb.ConfState, data []byte) (raftpb.Snapshot, error) {
	snap, err := ds.MemoryStorage.CreateSnapshot(i, cs, data)
	if err != nil {
		return raftpb.Snapshot{}, err
	}

	b, err := snap.Marshal()
	if err != nil {
		return raftpb.Snapshot{}, err
	}
	if err := ds.writeRecord(RecordSnapshot, b); err != nil {
		return raftpb.Snapshot{}, err
	}

	if err := ds.MemoryStorage.Compact(i); err != nil {
		return raftpb.Snapshot{}, err
	}
	return snap, nil
}

func (ds *DiskStorage) ApplySnapshot(snap raftpb.Snapshot) error {
	if err := ds.MemoryStorage.ApplySnapshot(snap); err != nil {
		return err
	}
	b, err := snap.Marshal()
	if err != nil {
		return err
	}
	return ds.writeRecord(RecordSnapshot, b)
}




