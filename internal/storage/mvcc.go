package storage

import (
	"encoding/binary"
	"math"
)

// EncodeKey appends the inverted timestamp to the key.
// Format: Key + (MaxUint64 - ts)
func EncodeKey(key []byte, ts uint64) []byte {
	// 8 bytes for uint64
	buf := make([]byte, len(key)+8)
	copy(buf, key)

	// Invert timestamp for descending order
	invTs := math.MaxUint64 - ts
	binary.BigEndian.PutUint64(buf[len(key):], invTs)

	return buf
}

// DecodeKey splits the key and timestamp.
func DecodeKey(joined []byte) ([]byte, uint64) {
	if len(joined) < 8 {
		return joined, 0 // Should not happen for valid MVCC keys
	}

	keyLen := len(joined) - 8
	key := joined[:keyLen]
	invTs := binary.BigEndian.Uint64(joined[keyLen:])
	ts := math.MaxUint64 - invTs

	return key, ts
}

// MVCCGet retrieves the latest valid version of the key visible at readTs.
// It scans for Key + (MaxUint64 - readTs) -> Key + MaxUint64(0)
// Because TS is inverted, bigger TS (newer) has smaller value.
// We want the newest version such that commitTS <= readTs.
// Encoded: Key + (Max - commitTS).
// We want commitTS <= readTs => Max - commitTS >= Max - readTs.
// So we scan starting from Max - readTs (inclusive) and go forward (increasing inverted value = decreasing real timestamp).
func MVCCGet(engine Engine, key []byte, readTs uint64) ([]byte, error) {
	// Start looking at readTs (which is stored as Max-readTs).
	// Since newer versions have SMALLER suffix (Max-New > Max-Old ?? No).
	// TS=100. Inv=Max-100.
	// TS=90.  Inv=Max-90.
	// Max-100 < Max-90.
	// So newer versions come FIRST in sort order.

	// We want the first entry where commitTS <= readTs.
	// => inverted stored value >= Max - readTs.
	// Wait.
	// Sorted: [Inv(110), Inv(100), Inv(90), Inv(80)...]
	// We want <= 100.
	// Inv(110) < Inv(100).
	// If we scan from Key+Inv(100), we see Inv(100), then Inv(90)...
	// This gives us descending order of TS.
	// So finding the first one is correct.

	start := EncodeKey(key, readTs)
	// End is Key + "infinity" (which is TS=0, Inv=Max).
	// Actually we need to stop if we hit a different key.
	// Or we use prefix scan and check key match.
	// Scan(start, end)
	// We can set End to Key + 0xFF (next prefix) to be safe,
	// OR just Key + Inv(0) which is Key + FFFFF...

	end := make([]byte, len(key)+1)
	copy(end, key)
	end[len(key)] = 0xFF // Stop if key changes
	// Note: this assumes key doesn't contain 0xFF? No.
	// If key is "A", end is "A\xFF".
	// If stored is "A"+... it is < "A\xFF" (assuming length difference handling or safe bytes).
	// Actually "A" + 8bytes < "A" + "\xFF" is good.
	// But what if Key="A\xFF"? Encoded "A\xFF" + TS.
	// Scan limits need to be careful.
	// Prefix scan usually: Scan(Prefix, Prefix_Successor).
	// Here we want specific range within prefix.

	var val []byte
	var found bool

	// fmt.Printf("Scanning for key=%s readTs=%d start=%x\n", key, readTs, start)

	err := engine.Scan(start, nil, func(k, v []byte) bool {
		// Verify key match (prefix)
		decodedKey, decTs := DecodeKey(k)
		// fmt.Printf("  Scanned: key=%s ts=%d\n", decodedKey, decTs)

		if string(decodedKey) != string(key) {
			return false // stop, different key
		}

		// Logic check:
		if decTs > readTs {
			// This shouldn't happen if we scanned correctly?
			// stored: Max-TS.
			// we verify TS <= readTs.
			// stored val (Max-TS) >= Max-readTs.
			// Max-TS >= Max-readTs => TS <= readTs.
			// So decTs must be <= readTs.
		}

		val = v
		found = true
		return false // stop after first match (latest version <= readTs)
	})

	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	return val, nil
}



