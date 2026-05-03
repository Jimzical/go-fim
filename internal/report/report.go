// Package report builds the JSON payload for a scan and persists the last N
// reports to a local history directory. The same payload shape will be POSTed
// to the control plane in Phase 4c — keep this struct stable.
package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Jimzical/go-fim/internal/store"
)

const dirPerm = 0o755

// HistoryMaxN caps how many regular report-*.json files Writer keeps after
// each Save. Queued (.unsent) reports are tracked separately by UnsentMaxN.
const HistoryMaxN = 10

// Report is the wire/disk format for one scan. AgentID is the stable bbolt
// UUID; AgentName + ScanPath are operator-supplied display fields plumbed
// through from the YAML config (omitted from disk output in standalone mode
// where they're never set).
type Report struct {
	AgentID     string        `json:"agent_id,omitempty"`
	AgentName   string        `json:"agent_name,omitempty"`
	ScanPath    string        `json:"scan_path,omitempty"`
	Timestamp   time.Time     `json:"timestamp"`
	TotalFiles  int64         `json:"total_files"`
	NumCreated  int           `json:"num_created"`
	NumModified int           `json:"num_modified"`
	NumDeleted  int           `json:"num_deleted"`
	Changes     []ChangeEntry `json:"changes"`
}

// ChangeEntry is one row of the `changes` array. Kind is the string form
// ("created"/"modified"/"deleted") rather than the int — the receiver (server
// or human reading the file) shouldn't depend on store.ChangeKind's iota.
type ChangeEntry struct {
	Kind string `json:"kind"`
	Path string `json:"path"`
}

// FromSummary translates a store.Summary into the wire format. AgentID/Name
// are filled in by the caller in 4b once we have them.
func FromSummary(totalFiles int64, s store.Summary) Report {
	changes := make([]ChangeEntry, len(s.Changes)) // make so empty marshals as [] not null
	for i, c := range s.Changes {
		changes[i] = ChangeEntry{Kind: c.Kind.String(), Path: c.Path}
	}
	return Report{
		Timestamp:   time.Now().UTC(),
		TotalFiles:  totalFiles,
		NumCreated:  s.NumCreated,
		NumModified: s.NumModified,
		NumDeleted:  s.NumDeleted,
		Changes:     changes,
	}
}

// Writer persists Reports to Dir, keeping at most MaxN.
type Writer struct {
	Dir  string
	MaxN int
}

// Save writes r as report-<ISO8601>.json under w.Dir, then prunes the oldest
// files until at most MaxN remain. Returns the absolute path written.
func (w *Writer) Save(r Report) (string, error) {
	if err := os.MkdirAll(w.Dir, dirPerm); err != nil {
		return "", fmt.Errorf("mkdir %q: %w", w.Dir, err)
	}

	name := fmt.Sprintf("report-%s.json", r.Timestamp.Format("2006-01-02T15-04-05Z"))
	path := filepath.Join(w.Dir, name)

	f, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("create %q: %w", path, err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r); err != nil {
		return path, fmt.Errorf("encode %q: %w", path, err)
	}

	if err := w.prune(); err != nil {
		return path, fmt.Errorf("prune: %w", err)
	}
	return path, nil
}

// prune deletes oldest report-*.json files until MaxN remain. Other files in
// the dir are left alone (we only own the ones we created).
func (w *Writer) prune() error {
	entries, err := os.ReadDir(w.Dir)
	if err != nil {
		return err
	}

	reports := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if strings.HasPrefix(n, "report-") && strings.HasSuffix(n, ".json") {
			reports = append(reports, n)
		}
	}
	if len(reports) <= w.MaxN {
		return nil
	}

	// ISO timestamps are zero-padded and UTC, so lexicographic order = chronological order.
	sort.Strings(reports)
	for _, n := range reports[:len(reports)-w.MaxN] {
		if err := os.Remove(filepath.Join(w.Dir, n)); err != nil {
			return fmt.Errorf("remove %q: %w", n, err)
		}
	}
	return nil
}
