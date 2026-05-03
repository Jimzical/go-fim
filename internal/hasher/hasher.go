package hasher

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"sync"

	"github.com/Jimzical/go-fim/internal/walker"
)

// numWorkers is the hash pool size. Hashing is CPU+disk-bound; ~8 is the
// typical saturation point on modern hardware.
const numWorkers = 8

// Run fans FileMeta from metaIn across numWorkers goroutines, fills in Hash
// on each, and fans them back out on hashedOut. Returns when metaIn is
// drained and all workers have finished, or when ctx is cancelled. The caller
// owns closing hashedOut — only the producer should signal "no more entries".
func Run(ctx context.Context, logger *slog.Logger, metaIn <-chan walker.FileMeta, hashedOut chan<- walker.FileMeta) error {
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	for i := range numWorkers {
		go func(id int) {
			defer wg.Done()
			worker(ctx, logger, id, metaIn, hashedOut)
		}(i)
	}

	wg.Wait()
	return nil
}

func worker(ctx context.Context, logger *slog.Logger, id int, metaIn <-chan walker.FileMeta, hashedOut chan<- walker.FileMeta) {
	for {
		select {
		case <-ctx.Done():
			return
		case m, ok := <-metaIn:
			if !ok {
				return
			}
			m.Hash = computeHash(m)
			select {
			case hashedOut <- m:
				logger.Debug("hashed", "worker", id, "path", m.Path)
			case <-ctx.Done():
				return
			}
		}
	}
}

// computeHash is a placeholder: it hashes (size, mtime) instead of file contents
// so we don't open files yet. Same byte shape as a real content hash, so
// swapping in `os.Open` + `io.Copy(h, f)` later is a one-function change.
func computeHash(m walker.FileMeta) []byte {
	s := fmt.Sprintf("%d:%d", m.Size, m.MTime)
	h := sha256.Sum256([]byte(s))
	return h[:]
}
