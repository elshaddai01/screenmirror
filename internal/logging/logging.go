// Package logging provides a tiny leveled logger so connect/disconnect
// events and setup problems are visible without being noisy.
package logging

import (
	"log"
	"os"
)

type Logger struct {
	verbose bool
	out     *log.Logger
	errOut  *log.Logger
}

func New(verbose bool) *Logger {
	return &Logger{
		verbose: verbose,
		out:     log.New(os.Stdout, "", log.Ltime),
		errOut:  log.New(os.Stderr, "", log.Ltime),
	}
}

func (l *Logger) Info(format string, args ...any) {
	l.out.Printf("[INFO]  "+format, args...)
}

func (l *Logger) Warn(format string, args ...any) {
	l.errOut.Printf("[WARN]  "+format, args...)
}

func (l *Logger) Error(format string, args ...any) {
	l.errOut.Printf("[ERROR] "+format, args...)
}

// Debug only prints when verbose logging is enabled (-v).
func (l *Logger) Debug(format string, args ...any) {
	if !l.verbose {
		return
	}
	l.out.Printf("[DEBUG] "+format, args...)
}
