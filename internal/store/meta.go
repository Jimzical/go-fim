package store

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.etcd.io/bbolt"
)

var (
	metaBucket  = []byte("meta")
	agentIDKey  = []byte("agent_id")
	apiTokenKey = []byte("api_token")
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

// GetAPIToken reads the API token stored at setup time. Returns "" if setup has
// not been completed yet (token absent from bbolt).
func GetAPIToken(db *bbolt.DB) (string, error) {
	var token string
	err := db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(metaBucket)
		if b == nil {
			return nil
		}
		if v := b.Get(apiTokenKey); v != nil {
			token = string(v)
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("api token: %w", err)
	}
	return token, nil
}

// SaveAPIToken persists the server-issued API token after a successful setup
// handshake. Overwrites any previously stored token.
func SaveAPIToken(db *bbolt.DB, token string) error {
	return db.Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(metaBucket)
		if err != nil {
			return err
		}
		return b.Put(apiTokenKey, []byte(token))
	})
}

// AgentID returns this agent's stable UUID. Lazy-inits on first call: generates
// a v4 UUID and stores it under meta/agent_id. Single Update txn (not
// View-then-Update) so concurrent first-run callers are race-free.
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
