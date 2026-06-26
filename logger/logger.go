// Package logger provides logging utilities for Launcher plugins.
package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// NewLogger creates a logger from the resolved configuration.
// name appears in formatted log messages. Obtain cfg via [LoadConfig] to
// respect Workbench's logging.conf. When cfg.Type is [DestinationStderr] or
// cfg.Dir is empty the logger writes to stderr only and returns no error.
func NewLogger(name string, cfg Config) (*slog.Logger, error) {
	if cfg.Type == DestinationStderr || cfg.Dir == "" {
		return newWorkbenchLogger(name, os.Stderr, cfg.Level), nil
	}
	if err := os.MkdirAll(cfg.Dir, 0o775); err != nil { //nolint:gosec // log dir from trusted plugin config
		return nil, err
	}
	// logFile is intentionally kept open for the process lifetime; the logger
	// writes to it until the process exits.
	logFile, err := os.Create(filepath.Join(cfg.Dir, name+".log")) //nolint:gosec // log paths from trusted plugin config
	if err != nil {
		return nil, err
	}
	return newWorkbenchLogger(name, io.MultiWriter(os.Stderr, logFile), cfg.Level), nil
}

// MustNewLogger is like NewLogger but logs the error to stderr and calls
// os.Exit(1) on failure. Deferred functions do not run. This is recommended
// for plugin main functions.
func MustNewLogger(name string, cfg Config) *slog.Logger {
	lgr, err := NewLogger(name, cfg)
	if err != nil {
		// stderr-only NewLogger cannot fail; if it somehow does, fall back to fmt.
		fallback, fallbackErr := NewLogger(name, Config{Type: DestinationStderr, Level: slog.LevelDebug})
		if fallbackErr != nil {
			fmt.Fprintf(os.Stderr, "FATAL: failed to initialize logger: %v (fallback also failed: %v)\n", err, fallbackErr)
			os.Exit(1)
		}
		fallback.Error("Failed to initialize logger", "error", err)
		os.Exit(1)
	}
	return lgr
}

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

type workbenchHandler struct {
	sink      io.Writer
	level     slog.Level
	programID string
	attrs     []slog.Attr
	groups    []string
}

func (h *workbenchHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *workbenchHandler) Handle(_ context.Context, r slog.Record) error {
	prefix := strings.Join(append(h.groups, ""), ".")
	var props []string
	for _, attr := range h.attrs {
		props = appendAttr(props, prefix, attr)
	}
	r.Attrs(func(attr slog.Attr) bool {
		props = appendAttr(props, prefix, attr)
		return true
	})
	propStr := ""
	if len(props) != 0 {
		propStr = " [" + strings.Join(props, ", ") + "]"
	}
	_, err := fmt.Fprintf(h.sink, "%s [%s] %s %s%s\n",
		r.Time.Format(timestampFormat), h.programID,
		levelString(r.Level), r.Message, propStr)
	return err
}

func (h *workbenchHandler) Level() slog.Level {
	return h.level
}

func (h *workbenchHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &workbenchHandler{
		sink:      h.sink,
		level:     h.level,
		programID: h.programID,
		attrs:     append(h.attrs[:len(h.attrs):len(h.attrs)], attrs...),
		groups:    h.groups,
	}
}

func (h *workbenchHandler) WithGroup(name string) slog.Handler {
	return &workbenchHandler{
		sink:      h.sink,
		level:     h.level,
		programID: h.programID,
		attrs:     h.attrs,
		groups:    append(h.groups[:len(h.groups):len(h.groups)], name),
	}
}

// appendAttr appends formatted leaf entries for attr to props. prefix is
// the dotted key prefix accumulated from enclosing groups; it is either
// empty or ends in ".". Handle establishes this invariant; appendAttr
// preserves it when building childPrefix. Group values are walked
// recursively, extending the prefix; an empty key on a group inlines its
// children at the current prefix; empty groups are omitted entirely.
func appendAttr(props []string, prefix string, attr slog.Attr) []string {
	value := attr.Value.Resolve()
	if value.Kind() == slog.KindGroup {
		children := value.Group()
		if len(children) == 0 {
			return props
		}
		childPrefix := prefix
		if attr.Key != "" {
			childPrefix = prefix + attr.Key + "."
		}
		for _, c := range children {
			props = appendAttr(props, childPrefix, c)
		}
		return props
	}
	if attr.Equal(slog.Attr{}) {
		return props
	}
	if attr.Key == "" {
		return props
	}
	return append(props, prefix+attr.Key+": "+formatValue(value))
}

// formatValue renders a resolved scalar [slog.Value] for the Workbench log
// format. The caller is responsible for routing group values through
// appendAttr; this function does not handle KindGroup.
func formatValue(v slog.Value) string {
	switch v.Kind() {
	case slog.KindBool:
		if v.Bool() {
			return "true"
		}
		return "false"
	case slog.KindString, slog.KindFloat64, slog.KindInt64, slog.KindUint64,
		slog.KindDuration, slog.KindTime:
		return v.String()
	case slog.KindAny:
		if err, ok := v.Any().(error); ok {
			return err.Error()
		}
		return v.String()
	case slog.KindGroup, slog.KindLogValuer:
		// Unreachable in normal operation: appendAttr calls Resolve()
		// before dispatching, which exhausts KindLogValuer wrappers, and
		// routes KindGroup values through its own recursion. Neither kind
		// reaches formatValue under normal use. Defensive only.
		return v.String()
	}
	// Unreachable: the switch covers every slog.Kind. Required by the Go
	// compiler because the switch has no default arm.
	return v.String()
}

// levelString maps a [slog.Level] to the level token expected by the Workbench
// launcher. slog uses "WARN"; the launcher requires "WARNING".
func levelString(level slog.Level) string {
	if level == slog.LevelWarn {
		return "WARNING"
	}
	return level.String()
}

const timestampFormat = "2006-01-02T15:04:05.000000Z"
