package logger

import (
	"log/slog"
	"os"

	"github.com/lmittmann/tint"
)

// New returns a slog logger that writes colorized, second-precision timestamps
// to stderr. verbose=true enables debug level; otherwise info.
func New(verbose bool) *slog.Logger {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	return slog.New(tint.NewHandler(os.Stderr, &tint.Options{
		Level:      level,
		TimeFormat: "15:04:05",
	}))
}
