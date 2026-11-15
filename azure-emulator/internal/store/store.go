// Package store is the emulator's persistence layer: a tiny bucketed
// key/value store holding one JSON document per key. Keys are always
// lower-cased by callers (ARM IDs are case-insensitive), so prefix scans
// give cheap "children of this scope" queries.
package store

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

// Buckets used by the emulator.
const (
	BucketARM      = "arm"      // ARM resources keyed by lower-cased resource ID
	BucketGraph    = "graph"    // directory objects keyed by "<tid>/<collection>/<id>"
	BucketOps      = "ops"      // async operations
	BucketActivity = "activity" // activity log events
	BucketMeta     = "meta"     // emulator metadata (seeded marker, mg parents, ...)
	BucketRoleDefs = "roledefs" // custom role definitions keyed by guid
	BucketPolicy   = "policy"   // policy compliance states keyed by "<assignment>|<resource>"
	BucketData     = "data"     // data planes: Key Vault secrets, blobs, files
)

var allBuckets = []string{BucketARM, BucketGraph, BucketOps, BucketActivity, BucketMeta, BucketRoleDefs, BucketPolicy, BucketData}

// Store is implemented by the bbolt-backed store and the in-memory store.
type Store interface {
	Get(bucket, key string) ([]byte, bool, error)
	Put(bucket, key string, val []byte) error
	Delete(bucket, key string) (bool, error)
	// Scan calls fn for every key with the given prefix, in key order,
	// until fn returns false.
	Scan(bucket, prefix string, fn func(key string, val []byte) bool) error
	// Dump returns a full copy of the store (used for snapshots).
	Dump() (Snapshot, error)
	// Load replaces the whole store content with the snapshot.
	Load(Snapshot) error
	Close() error
}

// Snapshot is a portable copy of the whole store.
type Snapshot struct {
	Version   int                                   `json:"version"`
	CreatedAt time.Time                             `json:"createdAt"`
	Buckets   map[string]map[string]json.RawMessage `json:"buckets"`
}

// WriteSnapshot serialises a snapshot.
func WriteSnapshot(w io.Writer, s Snapshot) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", " ")
	return enc.Encode(s)
}

// ReadSnapshot parses a snapshot.
func ReadSnapshot(r io.Reader) (Snapshot, error) {
	var s Snapshot
	if err := json.NewDecoder(r).Decode(&s); err != nil {
		return s, fmt.Errorf("decoding snapshot: %w", err)
	}
	if s.Buckets == nil {
		s.Buckets = map[string]map[string]json.RawMessage{}
	}
	return s, nil
}

// GetJSON is a convenience wrapper decoding a document.
func GetJSON(s Store, bucket, key string, out any) (bool, error) {
	b, ok, err := s.Get(bucket, key)
	if err != nil || !ok {
		return ok, err
	}
	return true, json.Unmarshal(b, out)
}

// PutJSON is a convenience wrapper encoding a document.
func PutJSON(s Store, bucket, key string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.Put(bucket, key, b)
}

// ---------------------------------------------------------------- memory

type memStore struct {
	mu      sync.RWMutex
	buckets map[string]map[string][]byte
}

// NewMemory returns an ephemeral in-memory store.
func NewMemory() Store {
	m := &memStore{buckets: map[string]map[string][]byte{}}
	for _, b := range allBuckets {
		m.buckets[b] = map[string][]byte{}
	}
	return m
}

func (m *memStore) Get(bucket, key string) ([]byte, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.buckets[bucket][key]
	if !ok {
		return nil, false, nil
	}
	return append([]byte(nil), v...), true, nil
}

func (m *memStore) Put(bucket, key string, val []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.buckets[bucket] == nil {
		m.buckets[bucket] = map[string][]byte{}
	}
	m.buckets[bucket][key] = append([]byte(nil), val...)
	return nil
}

func (m *memStore) Delete(bucket, key string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.buckets[bucket][key]
	delete(m.buckets[bucket], key)
	return ok, nil
}

func (m *memStore) Scan(bucket, prefix string, fn func(string, []byte) bool) error {
	m.mu.RLock()
	keys := make([]string, 0)
	for k := range m.buckets[bucket] {
		if strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	vals := make([][]byte, len(keys))
	for i, k := range keys {
		vals[i] = append([]byte(nil), m.buckets[bucket][k]...)
	}
	m.mu.RUnlock()
	for i, k := range keys {
		if !fn(k, vals[i]) {
			break
		}
	}
	return nil
}

func (m *memStore) Dump() (Snapshot, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s := Snapshot{Version: 1, CreatedAt: time.Now().UTC(), Buckets: map[string]map[string]json.RawMessage{}}
	for b, kv := range m.buckets {
		s.Buckets[b] = map[string]json.RawMessage{}
		for k, v := range kv {
			s.Buckets[b][k] = append(json.RawMessage(nil), v...)
		}
	}
	return s, nil
}

func (m *memStore) Load(s Snapshot) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.buckets = map[string]map[string][]byte{}
	for _, b := range allBuckets {
		m.buckets[b] = map[string][]byte{}
	}
	for b, kv := range s.Buckets {
		if m.buckets[b] == nil {
			m.buckets[b] = map[string][]byte{}
		}
		for k, v := range kv {
			m.buckets[b][k] = append([]byte(nil), v...)
		}
	}
	return nil
}

func (m *memStore) Close() error { return nil }

// ---------------------------------------------------------------- bbolt

type boltStore struct{ db *bolt.DB }

// OpenBolt opens (or creates) a persistent store at path.
func OpenBolt(path string) (Store, error) {
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		for _, b := range allBuckets {
			if _, err := tx.CreateBucketIfNotExists([]byte(b)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, err
	}
	return &boltStore{db: db}, nil
}

func (s *boltStore) Get(bucket, key string) ([]byte, bool, error) {
	var out []byte
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			return nil
		}
		if v := b.Get([]byte(key)); v != nil {
			out = append([]byte(nil), v...)
		}
		return nil
	})
	return out, out != nil, err
}

func (s *boltStore) Put(bucket, key string, val []byte) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(bucket))
		if err != nil {
			return err
		}
		return b.Put([]byte(key), val)
	})
}

func (s *boltStore) Delete(bucket, key string) (bool, error) {
	existed := false
	err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			return nil
		}
		existed = b.Get([]byte(key)) != nil
		return b.Delete([]byte(key))
	})
	return existed, err
}

func (s *boltStore) Scan(bucket, prefix string, fn func(string, []byte) bool) error {
	type kv struct {
		k string
		v []byte
	}
	var items []kv
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			return nil
		}
		c := b.Cursor()
		p := []byte(prefix)
		for k, v := c.Seek(p); k != nil && strings.HasPrefix(string(k), prefix); k, v = c.Next() {
			items = append(items, kv{string(k), append([]byte(nil), v...)})
		}
		return nil
	})
	if err != nil {
		return err
	}
	// call fn outside the transaction so callers may write
	for _, it := range items {
		if !fn(it.k, it.v) {
			break
		}
	}
	return nil
}

func (s *boltStore) Dump() (Snapshot, error) {
	snap := Snapshot{Version: 1, CreatedAt: time.Now().UTC(), Buckets: map[string]map[string]json.RawMessage{}}
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.ForEach(func(name []byte, b *bolt.Bucket) error {
			m := map[string]json.RawMessage{}
			err := b.ForEach(func(k, v []byte) error {
				m[string(k)] = append(json.RawMessage(nil), v...)
				return nil
			})
			snap.Buckets[string(name)] = m
			return err
		})
	})
	return snap, err
}

func (s *boltStore) Load(snap Snapshot) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		var names [][]byte
		_ = tx.ForEach(func(name []byte, _ *bolt.Bucket) error {
			names = append(names, append([]byte(nil), name...))
			return nil
		})
		for _, n := range names {
			if err := tx.DeleteBucket(n); err != nil {
				return err
			}
		}
		for _, b := range allBuckets {
			if _, err := tx.CreateBucketIfNotExists([]byte(b)); err != nil {
				return err
			}
		}
		for name, kv := range snap.Buckets {
			b, err := tx.CreateBucketIfNotExists([]byte(name))
			if err != nil {
				return err
			}
			for k, v := range kv {
				if err := b.Put([]byte(k), v); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (s *boltStore) Close() error { return s.db.Close() }
