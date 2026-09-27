// Package log is the single place that writes to stdout and stderr.
// Use it instead of fmt.Print* so that forbidigo can keep output paths in one place.
package log

import (
	"fmt"
	"io"
	"os"
)

// Logger writes normal output to out and diagnostics to err.
type Logger struct {
	out io.Writer
	err io.Writer
}

// Default writes to os.Stdout and os.Stderr.
var Default = New(os.Stdout, os.Stderr)

// New creates a Logger with the given writers.
func New(out, err io.Writer) *Logger {
	return &Logger{out: out, err: err}
}

// Printf writes formatted output to out.
func (l *Logger) Printf(format string, a ...any) {
	_, _ = fmt.Fprintf(l.out, format, a...)
}

// Println writes the operands to out followed by a newline.
func (l *Logger) Println(a ...any) {
	_, _ = fmt.Fprintln(l.out, a...)
}

// Errorf writes formatted output to err.
func (l *Logger) Errorf(format string, a ...any) {
	_, _ = fmt.Fprintf(l.err, format, a...)
}

// Errorln writes the operands to err followed by a newline.
func (l *Logger) Errorln(a ...any) {
	_, _ = fmt.Fprintln(l.err, a...)
}
