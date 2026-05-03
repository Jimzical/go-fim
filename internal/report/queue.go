package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// UnsentSuffix is appended to a saved report file when its POST fails with a
// transient error. The renamed file no longer matches Writer.prune's glob
// (`report-*.json`), so it survives FIFO rotation until replay succeeds.
const UnsentSuffix = ".unsent"

// UnsentMaxN caps how many queued reports we retain. Long outages drop the
// oldest entries (FIFO) — recent diffs are more useful than ancient ones for
// an FIM, and this prevents unbounded disk growth if the server is gone.
const UnsentMaxN = 10

// MarkUnsent atomically renames a saved report to flag it for replay on the
// next run, then prunes oldest queued reports beyond UnsentMaxN. path is the
// absolute path returned by Writer.Save.
func MarkUnsent(path string) error {
	if err := os.Rename(path, path+UnsentSuffix); err != nil {
		return fmt.Errorf("mark unsent %q: %w", path, err)
	}
	pending, err := ListUnsent(filepath.Dir(path))
	if err != nil {
		return err
	}
	if len(pending) <= UnsentMaxN {
		return nil
	}
	// pending is sorted oldest-first; drop the leading overflow.
	for _, p := range pending[:len(pending)-UnsentMaxN] {
		if err := os.Remove(p); err != nil {
			return fmt.Errorf("prune unsent %q: %w", p, err)
		}
	}
	return nil
}

// ListUnsent returns absolute paths of all queued reports in dir, oldest first.
// Filenames are `report-<ISO8601>.json.unsent`; the timestamp is fixed-width
// and zero-padded so lexical sort = chronological.
func ListUnsent(dir string) ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "report-*.json"+UnsentSuffix))
	if err != nil {
		return nil, fmt.Errorf("glob %q: %w", dir, err)
	}
	sort.Strings(matches)
	return matches, nil
}

// LoadFromFile reads a queued (or saved) report file back into a Report.
func LoadFromFile(path string) (Report, error) {
	var r Report
	data, err := os.ReadFile(path)
	if err != nil {
		return r, fmt.Errorf("read %q: %w", path, err)
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return r, fmt.Errorf("unmarshal %q: %w", path, err)
	}
	return r, nil
}
