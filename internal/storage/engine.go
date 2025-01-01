package storage

// Engine defines the interface for the local storage engine.
// It supports basic Key-Value operations.
type Engine interface {
	// Put writes a key-value pair.
	Put(key []byte, value []byte) error

	// Get retrieves a value by key.
	Get(key []byte) ([]byte, error)

	// Scan iterates over a range of keys.
	// start is inclusive, end is exclusive.
	// handler is called for each key-value pair; if it returns false, iteration stops.
	Scan(start, end []byte, handler func(key, value []byte) bool) error

	// Transactional Support
	GetTxn(key []byte, readTs uint64) ([]byte, error)       // MVCC Read
	Prepare(key, value []byte, txnID string) error          // Write Intent (Lock)
	Commit(key []byte, txnID string, commitTs uint64) error // Convert Intent to Version
	Abort(key []byte, txnID string) error                   // Remove Intent

	// Close closes the storage engine.
	Close() error
}



