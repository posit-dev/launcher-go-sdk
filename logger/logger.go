// Package logger provides logging utilities for Launcher plugins.
package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"strings"
)

// NewLogger creates a logger that emulates Launcher plugin logging conventions.
// The name appears in the formatted log messages; debug controls whether
// debug-level messages are logged and whether the debug-level log file is
// created; and loggingDir is the directory where the log file (and possibly the
// debug log file) should be written. Logs are also written to standard error.
// To disable file-based logging entirely pass an empty string as loggingDir.
// Note that this function will return a logger that can write to standard
// error, even in the case of an error -- which is useful when logging fatal
// errors.
func NewLogger(name string, debug bool, loggingDir string) (*slog.Logger, error) {
	var err error
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}
	if loggingDir == "" {
		return newWorkbenchLogger(name, os.Stderr, level), nil
	}
	fname := name + ".log"
	logFile, err := os.Create(path.Join(loggingDir, fname))
	if err != nil {
		return nil, err
	}
	if !debug {
		sink := io.MultiWriter(os.Stderr, logFile)
		return newWorkbenchLogger(name, sink, level), nil
	}
	fname = name + "-debug.log"
	debugFile, err := os.Create(path.Join(loggingDir, fname))
	if err != nil {
		return nil, err
	}
	sink := io.MultiWriter(os.Stderr, logFile, debugFile)
	return newWorkbenchLogger(name, sink, level), nil
}

// MustNewLogger is like NewLogger, but prints a message to standard error on
// failure and then aborts. This is recommended.
func MustNewLogger(name string, debug bool, loggingDir string) *slog.Logger {
	lgr, err := NewLogger(name, debug, loggingDir)
	if err != nil {
		lgr, _ = NewLogger(name, true, "")
		lgr.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}
	return lgr
}

// newWorkbenchLogger returns an [slog.Logger] with a handler that writes Workbench-style logs.
func newWorkbenchLogger(programID string, sink io.Writer, level slog.Level) *slog.Logger {
	handler := &workbenchHandler{
		sink:      sink,
		level:     level,
		programID: programID,
		attrs:     []slog.Attr{},
		groups:    []string{},
	}
	return slog.New(handler)
}

// workbenchHandler is a [slog.Handler] that writes Workbench-style logs.
type workbenchHandler struct {
	sink      io.Writer
	level     slog.Level
	programID string
	attrs     []slog.Attr
	groups    []string
}

// Enabled returns true if a message at a [slog.Level] would be logged.
func (h *workbenchHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

// Handle handles a [slog.Record].
func (h *workbenchHandler) Handle(_ context.Context, r slog.Record) error {
	group := strings.Join(append(h.groups, ""), ".")
	var props []string
	for _, attr := range h.attrs {
		formatted := formatAttr(group, attr)
		if formatted != "" {
			props = append(props, formatted)
		}
	}
	r.Attrs(func(attr slog.Attr) bool {
		formatted := formatAttr(group, attr)
		if formatted != "" {
			props = append(props, formatted)
		}
		return true
	})
	propStr := ""
	if len(props) != 0 {
		propStr = " [" + strings.Join(props, ", ") + "]"
	}
	_, err := fmt.Fprintf(h.sink, "%s [%s] %s %s%s\n",
		r.Time.Format(timestampFormat), h.programID,
		r.Level.String(), r.Message, propStr)
	return err
}

// Level returns the current [slog.Level].
func (h *workbenchHandler) Level() slog.Level {
	return h.level
}

// WithAttrs returns a new [workbenchHandler] with additional attributes.
func (h *workbenchHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &workbenchHandler{
		sink:      h.sink,
		level:     h.level,
		programID: h.programID,
		attrs:     append(h.attrs, attrs...),
		groups:    h.groups,
	}
}

// WithGroup returns a new [workbenchHandler] with an additional group.
func (h *workbenchHandler) WithGroup(name string) slog.Handler {
	return &workbenchHandler{
		sink:      h.sink,
		level:     h.level,
		programID: h.programID,
		attrs:     h.attrs,
		groups:    append(h.groups, name),
	}
}

func formatAttr(group string, attr slog.Attr) string {
	prefix := group + attr.Key + ": "
	value := attr.Value.Resolve()
	switch value.Kind() {
	case slog.KindBool:
		if value.Bool() {
			return prefix + "true"
		}
		return prefix + "false"
	case slog.KindString, slog.KindFloat64, slog.KindInt64, slog.KindUint64,
		slog.KindDuration, slog.KindTime:
		return prefix + value.String()
	case slog.KindAny:
		if err, ok := value.Any().(error); ok {
			return prefix + err.Error()
		}
		fallthrough
	case slog.KindGroup, slog.KindLogValuer:
		return ""
	}
	return ""
}

const timestampFormat = "2006-01-02T15:04:05.000000Z"
