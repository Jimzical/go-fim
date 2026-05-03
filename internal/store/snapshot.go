package store

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"go.etcd.io/bbolt"

	"github.com/Jimzical/go-fim/internal/walker"
)

const batchSize = 5000

var filesBucket = []byte("files")

// ChangeKind is what happened to a path between the old snapshot and now.
type ChangeKind int

const (
	Created ChangeKind = iota
	Modified
	Deleted
)

func (k ChangeKind) String() string {
	switch k {
	case Created:
		return "created"
	case Modified:
		return "modified"
	case Deleted:
		return "deleted"
	default:
		return "unknown"
	}
}

// Symbol is a one-char marker for terse output (`+`, `~`, `-`).
func (k ChangeKind) Symbol() string {
	switch k {
	case Created:
		return "+"
	case Modified:
		return "~"
	case Deleted:
		return "-"
	default:
		return "?"
	}
}

// Change is one row in the diff report.
type Change struct {
	Kind ChangeKind
	Path string
}

// Summary is what Run returns: per-kind counts plus the change list itself.
// NumUnchanged is tracked for the "no changes" stdout line; unchanged paths
// are not appended to Changes (would be noisy).
type Summary struct {
	NumCreated   int
	NumModified  int
	NumDeleted   int
	NumUnchanged int
	Changes      []Change
}

func (s *Summary) record(kind ChangeKind, path string) {
	switch kind {
	case Created:
		s.NumCreated++
	case Modified:
		s.NumModified++
	case Deleted:
		s.NumDeleted++
	}
	s.Changes = append(s.Changes, Change{Kind: kind, Path: path})
}

// Run consumes hashed FileMeta from hashedIn, diffs each against the prior
// snapshot, writes new/modified entries to bbolt in batches, and after the
// channel closes deletes keys for paths that disappeared. Returns a Summary
// describing the diff.
//
// On cancellation the snapshot may be partially updated — bbolt guarantees
// per-transaction atomicity, not per-run.
func Run(ctx context.Context, logger *slog.Logger, db *bbolt.DB, hashedIn <-chan walker.FileMeta) (Summary, error) {
	priorSnapshot, err := loadOld(db)
	if err != nil {
		return Summary{}, fmt.Errorf("load old snapshot: %w", err)
	}
	logger.Debug("loaded old snapshot", "entries", len(priorSnapshot))

	var summary Summary
	batch := make([]walker.FileMeta, 0, batchSize)

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := writeBatch(db, batch); err != nil {
			return fmt.Errorf("flush batch: %w", err)
		}
		logger.Debug("flushed batch", "n", len(batch))
		batch = batch[:0]
		return nil
	}

loop:
	for {
		select {
		case <-ctx.Done():
			return summary, ctx.Err()
		case m, ok := <-hashedIn:
			if !ok {
				break loop
			}
			if prior, exists := priorSnapshot[m.Path]; exists {
				delete(priorSnapshot, m.Path) // mark seen so it doesn't show up as deleted
				if bytes.Equal(prior.Hash, m.Hash) {
					summary.NumUnchanged++
					continue // entry already correct in bbolt; skip the write
				}
				summary.record(Modified, m.Path)
			} else {
				summary.record(Created, m.Path)
			}
			batch = append(batch, m)
			if len(batch) >= batchSize {
				if err := flush(); err != nil {
					return summary, err
				}
			}
		}
	}

	if err := flush(); err != nil {
		return summary, err
	}

	// Anything left in priorSnapshot was there before but not seen this run = deleted.
	if len(priorSnapshot) > 0 {
		deletedPaths := make([]string, 0, len(priorSnapshot))
		for p := range priorSnapshot {
			deletedPaths = append(deletedPaths, p)
			summary.record(Deleted, p)
		}
		if err := deleteKeys(db, deletedPaths); err != nil {
			return summary, fmt.Errorf("delete missing keys: %w", err)
		}
		logger.Debug("deleted missing keys", "n", len(deletedPaths))
	}

	return summary, nil
}

func loadOld(db *bbolt.DB) (map[string]walker.FileMeta, error) {
	priorSnapshot := make(map[string]walker.FileMeta)
	err := db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(filesBucket)
		if b == nil {
			return nil // first run
		}
		return b.ForEach(func(k, v []byte) error {
			var m walker.FileMeta
			if err := json.Unmarshal(v, &m); err != nil {
				return fmt.Errorf("unmarshal %q: %w", k, err)
			}
			// k's backing array is only valid inside the txn; copy via string().
			m.Path = string(k)
			priorSnapshot[m.Path] = m
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return priorSnapshot, nil
}

func writeBatch(db *bbolt.DB, batch []walker.FileMeta) error {
	return db.Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(filesBucket)
		if err != nil {
			return err
		}
		for _, m := range batch {
			val, err := json.Marshal(m)
			if err != nil {
				return fmt.Errorf("marshal %q: %w", m.Path, err)
			}
			if err := b.Put([]byte(m.Path), val); err != nil {
				return err
			}
		}
		return nil
	})
}

func deleteKeys(db *bbolt.DB, paths []string) error {
	return db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(filesBucket)
		if b == nil {
			return nil
		}
		for _, p := range paths {
			if err := b.Delete([]byte(p)); err != nil {
				return fmt.Errorf("delete %q: %w", p, err)
			}
		}
		return nil
	})
}
