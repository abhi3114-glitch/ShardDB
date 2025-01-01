package wal

import (
	"encoding/binary"
	"hash/crc32"
	"io"
	"os"
	"sync"
)

// WAL represents a Write Ahead Log.
type WAL struct {
	mu   sync.Mutex
	f    *os.File
	path string
}

// Open opens or creates a WAL file.
func Open(path string) (*WAL, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}
	return &WAL{
		f:    f,
		path: path,
	}, nil
}

// Append writes an entry to the WAL.
// Format: Len(4) | Data(N) | CRC(4)
func (w *WAL) Append(data []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// 1. Write Length
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, uint32(len(data)))
	if _, err := w.f.Write(buf); err != nil {
		return err
	}

	// 2. Write Data
	if _, err := w.f.Write(data); err != nil {
		return err
	}

	// 3. Write CRC
	crc := crc32.ChecksumIEEE(data)
	binary.BigEndian.PutUint32(buf, crc)
	if _, err := w.f.Write(buf); err != nil {
		return err
	}

	// Sync to disk (optional optimization: batch sync, but for safety sync now)
	return w.f.Sync()
}

// Iterate reads all entries from the WAL calling handler for each.
func (w *WAL) Iterate(handler func(data []byte) error) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Seek to start
	if _, err := w.f.Seek(0, 0); err != nil {
		return err
	}

	for {
		// Read Length
		lenBuf := make([]byte, 4)
		if _, err := io.ReadFull(w.f, lenBuf); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		length := binary.BigEndian.Uint32(lenBuf)

		// Read Data
		data := make([]byte, length)
		if _, err := io.ReadFull(w.f, data); err != nil {
			return err
		}

		// Read CRC
		crcBuf := make([]byte, 4)
		if _, err := io.ReadFull(w.f, crcBuf); err != nil {
			return err
		}
		expectedCRC := binary.BigEndian.Uint32(crcBuf)
		actualCRC := crc32.ChecksumIEEE(data)

		if actualCRC != expectedCRC {
			// Corruption? Return error or just log?
			// For now allow caller to decide, return error.
			return io.ErrUnexpectedEOF // using generic err for now
		}

		if err := handler(data); err != nil {
			return err
		}
	}

	// Reset position to end for appending
	w.f.Seek(0, 2)
	return nil
}

func (w *WAL) Close() error {
	return w.f.Close()
}



