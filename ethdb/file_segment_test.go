package ethdb_test

import (
	"bytes"
	"encoding/binary"
	"math"
	"math/rand"
	"os"
	"reflect"
	"testing"
	"testing/quick"

	"github.com/bcskill/bcschain/v3/common"

	"github.com/bcskill/bcschain/v3/ethdb"
)

func TestFileSegment_Get(t *testing.T) {
	t.Run("OK", func(t *testing.T) {
		path := MustTempFile()
		defer os.Remove(path)

		// Encode key/values to file segment.
		enc := ethdb.NewFileSegmentEncoder(path)
		if err := enc.Open(); err != nil {
			t.Fatal(err)
		} else if err := enc.EncodeKeyValue([]byte("foo"), []byte("bar")); err != nil {
			t.Fatal(err)
		} else if err := enc.Flush(); err != nil {
			t.Fatal(err)
		} else if err := enc.Close(); err != nil {
			t.Fatal(err)
		}

		// Open as file segment.
		s := ethdb.NewFileSegment("test", path)
		if err := s.Open(); err != nil {
			t.Fatal(err)
		}
		defer s.Close()

		// Fetch existing keys.
		if v, err := s.Get([]byte("foo")); err != nil {
			t.Fatal(err)
		} else if string(v) != "bar" {
			t.Fatalf("unexpected value: %q", v)
		}

		// Fetch unknown key.
		if v, err := s.Get([]byte("no_such_key")); err != common.ErrNotFound {
			t.Fatalf("unexpected error: %s", err)
		} else if v != nil {
			t.Fatalf("expected nil value, got %q", v)
		}

		// Close file segment.
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
	})
}

// Ensure ethdb.FileSegment can fetch keys using randomized test data.
func TestFileSegment_Quick(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}

	const maxCount = 10000
	const maxKeyLen = 2000
	const maxValueLen = 10000

	quick.Check(func(keys, values [][]byte) bool {
		path := MustTempFile()
		defer os.Remove(path)

		// Write data to file.
		if err := EncodeToFileSegment(path, keys, values); err != nil {
			t.Fatal(err)
		}

		// Open as file.
		s := ethdb.NewFileSegment("test", path)
		if err := s.Open(); err != nil {
			t.Fatal(err)
		}
		defer s.Close()

		// Verify all key/value pairs exist.
		for i := range keys {
			if v, err := s.Get(keys[i]); err != nil {
				t.Fatal(err)
			} else if !bytes.Equal(values[i], v) {
				t.Fatalf("value mismatch: (key=%q) expected %q, got %q", keys[i], values[i], v)
			}
		}

		// Verify we can iterate them in order.
		itr, i := s.Iterator(), 0
		for ; itr.Next(); i++ {
			if !bytes.Equal(itr.Key(), keys[i]) {
				t.Fatalf("iterator key mismatch:\nexpected %x\ngot %x", keys[i], itr.Key())
			} else if !bytes.Equal(itr.Value(), values[i]) {
				t.Fatal("iterator value mismatch")
			}
		}
		if i != len(keys) {
			t.Fatal("short iterator")
		}

		return true
	}, &quick.Config{
		MaxCount: 10,
		Values: func(args []reflect.Value, rand *rand.Rand) {
			n := rand.Intn(maxCount-1) + 1
			args[0] = reflect.ValueOf(generateKeys(n, 1, maxKeyLen-1, rand))
			args[1] = reflect.ValueOf(generateValues(n, 0, maxValueLen, rand))
		},
	})
}

func BenchmarkFileSegment_Get(b *testing.B) {
	path := MustTempFile()
	defer os.Remove(path)

	// Encode key/value pairs.
	const n = 100000
	keys, values := make([][]byte, n), make([][]byte, n)
	for i := 0; i < n; i++ {
		keys[i] = make([]byte, 32)
		binary.BigEndian.PutUint64(keys[i], uint64(i))
		values[i] = make([]byte, 1024)
	}
	EncodeToFileSegment(path, keys, values)

	// Determine random access pattern.
	perm := rand.Perm(n)

	// Open as file segment.
	s := ethdb.NewFileSegment("test", path)
	if err := s.Open(); err != nil {
		b.Fatal(err)
	}
	defer s.Close()

	b.ResetTimer()
	b.ReportAllocs()

	// Lookup key/value pairs.
	for i := 0; i < b.N; i++ {
		key := keys[perm[i%len(perm)]]
		if v, err := s.Get(key); err != nil {
			b.Fatal(err)
		} else if v == nil {
			b.Fatal("key not found")
		}
	}
}

// EncodeToFileSegment encodes a set of key/value pairs to an ethdb.FileSegment at path.
func EncodeToFileSegment(path string, keys, values [][]byte) error {
	// Build file segment.
	enc := ethdb.NewFileSegmentEncoder(path)
	if err := enc.Open(); err != nil {
		return err
	}
	defer enc.Close()

	// Write all keys.
	for i := range keys {
		if err := enc.EncodeKeyValue(keys[i], values[i]); err != nil {
			return err
		}
	}

	// Flush all data.
	if err := enc.Flush(); err != nil {
		return err
	} else if err := enc.Close(); err != nil {
		return err
	}
	return nil
}

// generateKeys returns a set of n unique, randomly generated keys.
func generateKeys(n, min, max int, rand *rand.Rand) [][]byte {
	a := make([][]byte, n)
	for i, m := 0, make(map[string]struct{}); i < len(a); i++ {
		a[i] = generateBytes(min, max, rand)
		if _, ok := m[string(a[i])]; ok {
			i--
			continue
		}
		m[string(a[i])] = struct{}{}
	}
	return a
}

// generateValues returns a set of n randomly generated values.
func generateValues(n, min, max int, rand *rand.Rand) [][]byte {
	a := make([][]byte, n)
	for i := range a {
		a[i] = generateBytes(min, max, rand)
	}
	return a
}

// generateBytes returns a randomly generated byte slice between min & max length.
func generateBytes(min, max int, rand *rand.Rand) []byte {
	b := make([]byte, rand.Intn(max-min)+min)
	for i := range b {
		b[i] = byte(rand.Intn(math.MaxInt8))
	}
	return b
}
