// Package logset wires slog output to stdout or to a rotating file using
// lumberjack (size + age based rotation, backup pruning, gzip compression).
package logset

import (
	"io"
	"os"

	"gopkg.in/natefinch/lumberjack.v2"
)

// File describes a rolling log file destination.
type File struct {
	Path       string // empty => stdout
	MaxSizeMB  int    // megabytes per file before rotation (default 100)
	MaxBackups int    // retained rotated files (default 5)
	MaxAgeDays int    // kept days (default 14; 0 disables age)
	Compress   bool   // gzip rotated files
}

// New returns the output writer for the given configuration: stdout when no
// file path is set, otherwise a lumberjack rolling writer.
func New(cfg File) io.Writer {
	if cfg.Path == "" {
		return os.Stdout
	}
	if cfg.MaxSizeMB <= 0 {
		cfg.MaxSizeMB = 100
	}
	if cfg.MaxBackups < 0 {
		cfg.MaxBackups = 0
	}
	if cfg.MaxAgeDays < 0 {
		cfg.MaxAgeDays = 0
	}
	return &lumberjack.Logger{
		Filename:   cfg.Path,
		MaxSize:    cfg.MaxSizeMB,
		MaxBackups: cfg.MaxBackups,
		MaxAge:     cfg.MaxAgeDays,
		Compress:   cfg.Compress,
	}
}
