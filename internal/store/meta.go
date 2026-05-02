package store

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.etcd.io/bbolt"
)

var (
	metaBucket = []byte("meta")
	agentIDKey = []byte("agent_id")
)

// Open opens (or creates) the bbolt database at path. bbolt is single-writer:
// the Timeout caps how long we wait if another process holds the file lock.
func Open(path string) (*bbolt.DB, error) {
	db, err := bbolt.Open(path, 0600, &bbolt.Options{Timeout: 2 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("open db %q: %w", path, err)
	}
	return db, nil
}

// AgentID returns this agent's stable UUID. Lazy-inits on first call: generates
// a v4 UUID, stores it under meta/agent_id, and returns it. One Update txn
// (not View-then-Update) so first-run init is race-free; on the steady-state
// path bbolt only fsyncs if Put actually ran.
func AgentID(db *bbolt.DB) (string, error) {
	var id string
	err := db.Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(metaBucket)
		if err != nil {
			return err
		}
		if v := b.Get(agentIDKey); v != nil {
			id = string(v)
			return nil
		}
		id = uuid.NewString()
		return b.Put(agentIDKey, []byte(id))
	})
	if err != nil {
		return "", fmt.Errorf("agent id: %w", err)
	}
	return id, nil
}
