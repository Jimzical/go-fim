package walker

import (
	"context"
	"io/fs"
	"log/slog"
	"regexp"
	"sync/atomic"

	"github.com/charlievieth/fastwalk"
)

// FileMeta is what the walker emits for each regular file it visits.
// Path is the bbolt key, so json:"-" excludes it from the value bytes.
// Hash is filled in by the hasher stage.
type FileMeta struct {
	Path  string `json:"-"`
	Size  int64  `json:"size"`
	MTime int64  `json:"mtime"` // unix nanoseconds
	Hash  []byte `json:"hash,omitempty"`
}

// Walk walks root in parallel, sending one FileMeta on metaOut per regular
// file. Symlinks are skipped, dirs whose basename matches any exclude regex
// are pruned, any path in skipPaths (used for go-fim's own DB / history dir,
// which would otherwise self-modify and show up as diffs) is skipped, and
// per-entry errors are logged but do not stop traversal. Returns the count
// of regular files visited.
func Walk(ctx context.Context, logger *slog.Logger, root string, excludes []*regexp.Regexp, skipPaths []string, metaOut chan<- FileMeta) (int64, error) {
	skipSet := make(map[string]struct{}, len(skipPaths))
	for _, p := range skipPaths {
		skipSet[p] = struct{}{}
	}

	var count int64

	walkFn := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			logger.Warn("walk error", "path", path, "err", err)
			return nil
		}
		if (d.Type() & fs.ModeSymlink) != 0 { // skip symlinks
			return nil
		}
		if _, skip := skipSet[path]; skip {
			if d.IsDir() {
				return fs.SkipDir // prune the whole subtree (e.g. history/)
			}
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			for _, re := range excludes { // *regexp.Regexp is safe for concurrent use
				if re.MatchString(name) {
					logger.Debug("skipping directory", "path", path)
					return fs.SkipDir // prunes this subtree across all walker goroutines
				}
			}
			logger.Debug("entering directory", "path", path)
			return nil
		}
		if d.Type().IsRegular() {
			info, err := d.Info()
			if err != nil {
				logger.Warn("stat failed", "path", path, "err", err)
				return nil
			}
			meta := FileMeta{
				Path:  path,
				Size:  info.Size(),
				MTime: info.ModTime().UnixNano(),
			}

			select {
			case metaOut <- meta:
				atomic.AddInt64(&count, 1)
				logger.Debug("file", "path", path)
			case <-ctx.Done():
				return ctx.Err() // fastwalk stops when callback returns non-nil error
			}
		}
		return nil
	}

	if err := fastwalk.Walk(nil, root, walkFn); err != nil {
		return atomic.LoadInt64(&count), err
	}
	return atomic.LoadInt64(&count), nil
}
