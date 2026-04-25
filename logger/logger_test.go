package logger

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newBufferLogger returns a Workbench-style logger writing to an in-memory
// buffer at debug level, suitable for assertion-based tests.
func newBufferLogger(t *testing.T) (*slog.Logger, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	return newWorkbenchLogger("test", &buf, slog.LevelDebug), &buf
}

// extractProps returns the comma-separated entries from the trailing
// "[k: v, ...]" block of a single Workbench-formatted log line, or nil if
// the line has no trailing block. The leading "[program]" segment is not
// matched because the trailing block is identified by a " [" prefix and
// closing "]" at end of line.
func extractProps(line string) []string {
	line = strings.TrimRight(line, "\n")
	if !strings.HasSuffix(line, "]") {
		return nil
	}
	line = line[:len(line)-1]
	open := strings.LastIndex(line, " [")
	if open == -1 {
		return nil
	}
	inner := line[open+2:]
	if inner == "" {
		return nil
	}
	return strings.Split(inner, ", ")
}

func TestHandleScalarAttrs(t *testing.T) {
	cases := []struct {
		name string
		attr slog.Attr
		want string
	}{
		{"bool true", slog.Bool("ok", true), "ok: true"},
		{"bool false", slog.Bool("ok", false), "ok: false"},
		{"string", slog.String("name", "alice"), "name: alice"},
		{"int", slog.Int("n", 42), "n: 42"},
		{"float", slog.Float64("f", 1.5), "f: 1.5"},
		{"duration", slog.Duration("d", 250*time.Millisecond), "d: 250ms"},
		{"time", slog.Time("t", time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)), "t: 2024-01-15 10:30:00 +0000 UTC"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lgr, buf := newBufferLogger(t)
			lgr.Warn("m", tc.attr)
			props := extractProps(buf.String())
			if len(props) != 1 || props[0] != tc.want {
				t.Errorf("got %v, want [%q]", props, tc.want)
			}
		})
	}
}

func TestHandleInlineGroup(t *testing.T) {
	lgr, buf := newBufferLogger(t)
	lgr.Warn("op",
		slog.Group("job",
			slog.String("id", "42"),
			slog.String("user", "bob"),
		),
	)
	props := extractProps(buf.String())
	want := []string{"job.id: 42", "job.user: bob"}
	if len(props) != len(want) {
		t.Fatalf("got %v, want %v", props, want)
	}
	for i, w := range want {
		if props[i] != w {
			t.Errorf("props[%d] = %q, want %q", i, props[i], w)
		}
	}
}

// jobDetails mirrors the type from issue #15: a domain type that attaches
// structured fields via slog.LogValuer.
type jobDetails struct {
	ID   string
	User string
}

func (j jobDetails) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("id", j.ID),
		slog.String("user", j.User),
	)
}

func TestHandleLogValuerGroup(t *testing.T) {
	lgr, buf := newBufferLogger(t)
	lgr.Warn("op", "job", jobDetails{ID: "99", User: "carmen"})
	props := extractProps(buf.String())
	want := []string{"job.id: 99", "job.user: carmen"}
	if len(props) != len(want) {
		t.Fatalf("got %v, want %v", props, want)
	}
	for i, w := range want {
		if props[i] != w {
			t.Errorf("props[%d] = %q, want %q", i, props[i], w)
		}
	}
}

func TestHandleNestedGroups(t *testing.T) {
	lgr, buf := newBufferLogger(t)
	lgr.Warn("op",
		slog.Group("a",
			slog.Group("b",
				slog.String("c", "v"),
			),
		),
	)
	props := extractProps(buf.String())
	if len(props) != 1 || props[0] != "a.b.c: v" {
		t.Errorf("got %v, want [\"a.b.c: v\"]", props)
	}
}

func TestHandleWithGroupAndInlineGroup(t *testing.T) {
	lgr, buf := newBufferLogger(t)
	lgr.WithGroup("outer").Warn("op",
		slog.Group("inner",
			slog.String("k", "v"),
		),
	)
	props := extractProps(buf.String())
	if len(props) != 1 || props[0] != "outer.inner.k: v" {
		t.Errorf("got %v, want [\"outer.inner.k: v\"]", props)
	}
}

func TestHandleWithAttrsAndInlineGroup(t *testing.T) {
	lgr, buf := newBufferLogger(t)
	lgr.With("svc", "api").Warn("op",
		slog.Group("job",
			slog.String("id", "42"),
		),
	)
	props := extractProps(buf.String())
	want := []string{"svc: api", "job.id: 42"}
	if len(props) != len(want) {
		t.Fatalf("got %v, want %v", props, want)
	}
	for i, w := range want {
		if props[i] != w {
			t.Errorf("props[%d] = %q, want %q", i, props[i], w)
		}
	}
}

func TestHandleEmptyKeyGroupInlines(t *testing.T) {
	lgr, buf := newBufferLogger(t)
	lgr.Warn("op",
		slog.Group("",
			slog.String("k", "v"),
		),
	)
	props := extractProps(buf.String())
	if len(props) != 1 || props[0] != "k: v" {
		t.Errorf("got %v, want [\"k: v\"]", props)
	}
}

func TestHandleEmptyGroupOmitted(t *testing.T) {
	lgr, buf := newBufferLogger(t)
	lgr.Warn("op",
		slog.String("before", "x"),
		slog.Group("empty"),
		slog.String("after", "y"),
	)
	props := extractProps(buf.String())
	want := []string{"before: x", "after: y"}
	if len(props) != len(want) {
		t.Fatalf("got %v, want %v", props, want)
	}
	for i, w := range want {
		if props[i] != w {
			t.Errorf("props[%d] = %q, want %q", i, props[i], w)
		}
	}
}

func TestHandleAnyError(t *testing.T) {
	lgr, buf := newBufferLogger(t)
	lgr.Warn("op", slog.Any("err", errors.New("boom")))
	props := extractProps(buf.String())
	if len(props) != 1 || props[0] != "err: boom" {
		t.Errorf("got %v, want [\"err: boom\"]", props)
	}
}

func TestHandleAnyNonError(t *testing.T) {
	lgr, buf := newBufferLogger(t)
	lgr.Warn("op", slog.Any("obj", struct{ X int }{X: 1}))
	props := extractProps(buf.String())
	if len(props) != 1 {
		t.Fatalf("got %v, want exactly one entry", props)
	}
	// Format is fmt.Sprint of the value; pin only the key prefix and
	// non-emptiness so this does not over-fit fmt's struct formatting.
	if !strings.HasPrefix(props[0], "obj: ") || props[0] == "obj: " {
		t.Errorf("got %q, want non-empty value with prefix \"obj: \"", props[0])
	}
}

// TestIssue15Reproducer is the verbatim test from issue #15. It exercises
// the full NewLogger -> file path and asserts that scalar, slog.Group, and
// LogValuer-as-group attributes all reach the log file.
func TestIssue15Reproducer(t *testing.T) {
	dir := t.TempDir()
	lgr, err := NewLogger("test", false, dir)
	if err != nil {
		t.Fatal(err)
	}

	// Case 0: simple scalar attributes.
	lgr.Warn("operation failed", "id", "11", "user", "alice")

	// Case 1: slog.Group.
	lgr.Warn("operation failed",
		slog.Group("job",
			slog.String("id", "42"),
			slog.String("user", "bob"),
		))

	// Case 2: slog.LogValuer returning a group.
	lgr.Warn("operation failed", "job", jobDetails{ID: "99", User: "carmen"})

	output, err := os.ReadFile(filepath.Join(dir, "test.log")) //nolint:gosec // dir is from t.TempDir()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("captured output:\n%s", output)

	for _, want := range []string{"11", "alice", "42", "bob", "99", "carmen"} {
		if !strings.Contains(string(output), want) {
			t.Errorf("field %q is missing from log output", want)
		}
	}
}

func TestHandleGroupKeyWithDot(t *testing.T) {
	lgr, buf := newBufferLogger(t)
	lgr.Warn("op",
		slog.Group("a.b",
			slog.String("c", "v"),
		),
	)
	props := extractProps(buf.String())
	if len(props) != 1 || props[0] != "a.b.c: v" {
		t.Errorf("got %v, want [\"a.b.c: v\"]", props)
	}
}

func TestHandleMixedScalarAndSubgroup(t *testing.T) {
	lgr, buf := newBufferLogger(t)
	lgr.Warn("op",
		slog.Group("outer",
			slog.String("x", "1"),
			slog.Group("inner",
				slog.String("y", "2"),
			),
		),
	)
	props := extractProps(buf.String())
	want := []string{"outer.x: 1", "outer.inner.y: 2"}
	if len(props) != len(want) {
		t.Fatalf("got %v, want %v", props, want)
	}
	for i, w := range want {
		if props[i] != w {
			t.Errorf("props[%d] = %q, want %q", i, props[i], w)
		}
	}
}

func TestHandleEmptySubgroupInsideNonEmpty(t *testing.T) {
	lgr, buf := newBufferLogger(t)
	lgr.Warn("op",
		slog.Group("outer",
			slog.String("x", "1"),
			slog.Group("empty"),
		),
	)
	props := extractProps(buf.String())
	if len(props) != 1 || props[0] != "outer.x: 1" {
		t.Errorf("got %v, want [\"outer.x: 1\"]", props)
	}
}

// scalarValuer is a LogValuer whose LogValue returns a non-group scalar
// value. This exercises the code path where Resolve() yields a scalar that
// then falls through to the leaf-append branch in appendAttr.
type scalarValuer struct {
	s string
}

func (v scalarValuer) LogValue() slog.Value {
	return slog.StringValue(v.s)
}

func TestHandleLogValuerScalar(t *testing.T) {
	lgr, buf := newBufferLogger(t)
	lgr.Warn("op", "status", scalarValuer{s: "foo"})
	props := extractProps(buf.String())
	if len(props) != 1 || props[0] != "status: foo" {
		t.Errorf("got %v, want [\"status: foo\"]", props)
	}
}
