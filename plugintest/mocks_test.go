package plugintest_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/posit-dev/launcher-go-sdk/api"
	"github.com/posit-dev/launcher-go-sdk/plugintest"
)

// TestMockStreamResponseWriter_RaceRegression reproduces issue #19:
// concurrent writes and reads against the mock must be race-free under -race.
func TestMockStreamResponseWriter_RaceRegression(t *testing.T) {
	sw := plugintest.NewMockStreamResponseWriter()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for range 1000 {
			_ = sw.WriteJobStatus("1", "job", api.StatusRunning, "", "")
		}
	}()

	go func() {
		defer wg.Done()
		for range 1000 {
			_ = len(sw.Statuses())
		}
	}()

	wg.Wait()

	if got := sw.StatusCount(); got != 1000 {
		t.Errorf("StatusCount() = %d, want 1000", got)
	}
}

// TestMockResponseWriter_RaceRegression exercises concurrent WriteJobs
// against the Jobs accessor.
func TestMockResponseWriter_RaceRegression(t *testing.T) {
	w := plugintest.NewMockResponseWriter()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for range 1000 {
			_ = w.WriteJobs([]*api.Job{{ID: "j", Status: api.StatusRunning}})
		}
	}()

	go func() {
		defer wg.Done()
		for range 1000 {
			_ = len(w.Jobs())
		}
	}()

	wg.Wait()

	if got := len(w.Jobs()); got != 1000 {
		t.Errorf("len(Jobs()) = %d, want 1000", got)
	}
}

// TestMockStreamResponseWriter_ConcurrentLoad exercises broad concurrent
// access on a single MockStreamResponseWriter: writes and reads on both
// halves of the embedded API plus a Reset() spammer. It is not a targeted
// probe of the original shadowed-mutex bug (the goroutines below touch
// disjoint memory locations under their own locks, so a regression that
// re-introduced a separate mutex would still pass under -race), but it
// does provide useful general race-detector coverage and exercises every
// new accessor under contention.
func TestMockStreamResponseWriter_ConcurrentLoad(t *testing.T) {
	w := plugintest.NewMockStreamResponseWriter()

	const iters = 500

	var wg sync.WaitGroup
	wg.Add(5)

	go func() {
		defer wg.Done()
		for range iters {
			_ = w.WriteError(errors.New("boom"))
		}
	}()

	go func() {
		defer wg.Done()
		for range iters {
			_ = w.WriteJobStatus("1", "job", api.StatusRunning, "", "")
		}
	}()

	go func() {
		defer wg.Done()
		for range iters {
			_ = len(w.Errors())
			_ = len(w.Statuses())
		}
	}()

	go func() {
		defer wg.Done()
		for range iters {
			_ = w.IsClosed()
		}
	}()

	go func() {
		defer wg.Done()
		for range iters / 5 {
			w.Reset()
		}
	}()

	wg.Wait()

	// After concurrent Reset(), exact counts depend on interleaving; we only
	// assert that observation is consistent and below the maximum that could
	// have been written.
	if got := len(w.Errors()); got > iters {
		t.Errorf("len(Errors()) = %d exceeds iters %d", got, iters)
	}
	if got := len(w.Statuses()); got > iters {
		t.Errorf("len(Statuses()) = %d exceeds iters %d", got, iters)
	}
}

// TestMockResponseWriter_DefensiveCopies verifies that mutating a returned
// slice does not affect the writer's recorded state.
func TestMockResponseWriter_DefensiveCopies(t *testing.T) {
	w := plugintest.NewMockResponseWriter()
	_ = w.WriteErrorf(api.CodeUnknown, "first")
	_ = w.WriteJobs([]*api.Job{{ID: "j1"}})
	_ = w.WriteControlJob(true, "ok")
	_ = w.WriteJobNetwork("host", []string{"1.2.3.4"})
	_ = w.WriteConfigReload(api.ReloadErrorNone, "", nil, nil, 0)

	// Mutate every returned slice; recorded state must not move.
	got := w.Errors()
	got = append(got, &api.Error{Code: api.CodeUnknown, Msg: "injected"})
	got[0] = &api.Error{Code: api.CodeUnknown, Msg: "rewritten"}
	if again := w.Errors(); len(again) != 1 || again[0].Msg != "first" {
		t.Errorf("Errors() leaked mutation: %+v", again)
	}

	jobs := w.Jobs()
	jobs = append(jobs, []*api.Job{{ID: "leak"}})
	if len(jobs) != 2 {
		t.Fatalf("local append did not extend slice: len=%d", len(jobs))
	}
	if again := w.Jobs(); len(again) != 1 {
		t.Errorf("Jobs() leaked outer mutation: len=%d", len(again))
	}

	results := w.ControlResults()
	results = append(results, plugintest.ControlResult{Complete: false, Message: "leak"})
	results[0].Message = "rewritten"
	if again := w.ControlResults(); len(again) != 1 || again[0].Message != "ok" {
		t.Errorf("ControlResults() leaked mutation: %+v", again)
	}

	nets := w.Networks()
	nets = append(nets, plugintest.NetworkInfo{Host: "leak"})
	nets[0].Host = "rewritten"
	nets[0].Addresses[0] = "9.9.9.9" // element-level mutation of the inner []string
	if again := w.Networks(); len(again) != 1 || again[0].Host != "host" ||
		len(again[0].Addresses) != 1 || again[0].Addresses[0] != "1.2.3.4" {
		t.Errorf("Networks() leaked mutation: %+v", again)
	}

	// LastJobs returns a freshly allocated slice header; append must not leak.
	last := w.LastJobs()
	last = append(last, &api.Job{ID: "leak"})
	if len(last) != 2 {
		t.Fatalf("local append did not extend LastJobs slice: len=%d", len(last))
	}
	if again := w.LastJobs(); len(again) != 1 {
		t.Errorf("LastJobs() leaked outer mutation: len=%d", len(again))
	}

	reloads := w.ConfigReloadResults()
	reloads = append(reloads, plugintest.ConfigReloadResult{ErrorMessage: "leak"})
	reloads[0].ErrorMessage = "rewritten"
	if again := w.ConfigReloadResults(); len(again) != 1 || again[0].ErrorMessage != "" {
		t.Errorf("ConfigReloadResults() leaked mutation: %+v", again)
	}
}

// TestMockStreamResponseWriter_DefensiveCopies covers the stream-only
// accessors.
func TestMockStreamResponseWriter_DefensiveCopies(t *testing.T) {
	w := plugintest.NewMockStreamResponseWriter()
	_ = w.WriteJobStatus("j1", "Job", api.StatusRunning, "", "msg")
	_ = w.WriteJobOutput("hello", api.OutputStdout)
	_ = w.WriteJobResourceUtil(1, 2, 3, 4)

	statuses := w.Statuses()
	statuses = append(statuses, plugintest.StatusUpdate{ID: "leak"})
	statuses[0].Status = "rewritten"
	if again := w.Statuses(); len(again) != 1 || again[0].Status != api.StatusRunning {
		t.Errorf("Statuses() leaked mutation: %+v", again)
	}

	outputs := w.Outputs()
	outputs = append(outputs, plugintest.OutputChunk{Output: "leak"})
	outputs[0].Output = "rewritten"
	if again := w.Outputs(); len(again) != 1 || again[0].Output != "hello" {
		t.Errorf("Outputs() leaked mutation: %+v", again)
	}

	utils := w.ResourceUtils()
	utils = append(utils, plugintest.ResourceUtilData{CPUPercent: 99})
	utils[0].CPUPercent = 99
	if again := w.ResourceUtils(); len(again) != 1 || again[0].CPUPercent != 1 {
		t.Errorf("ResourceUtils() leaked mutation: %+v", again)
	}
}

// TestMockResponseWriter_ClusterInfoIsDeepCopied verifies that ClusterInfo()
// returns a deep copy: the returned pointer, its slice fields, the nested
// ImageOpt.Images slice, and each ResourceProfile's inner Limits slice are
// all freshly allocated, so caller mutation at any level does not leak into
// the writer's stored state.
func TestMockResponseWriter_ClusterInfoIsDeepCopied(t *testing.T) {
	w := plugintest.NewMockResponseWriter()
	opts := plugintest.NewClusterOptions().
		WithQueue("default").
		WithLimit("cpuCount", "4").
		WithImage("ubuntu:24.04").
		Build()
	// Add a profile with its own nested Limits so we exercise the nested clone.
	opts.Profiles = []api.ResourceProfile{{
		Name:   "small",
		Limits: []api.ResourceLimit{{Type: "memory", Value: "1Gi"}},
	}}
	if err := w.WriteClusterInfo(opts); err != nil {
		t.Fatalf("WriteClusterInfo: %v", err)
	}

	got := w.ClusterInfo()
	if got == nil {
		t.Fatal("ClusterInfo() returned nil")
		return // unreachable, but satisfies staticcheck SA5011
	}

	// Mutate every slice the contract promises to deep-copy.
	got.Queues = nil
	got.Limits[0].Value = "rewritten"
	got.Limits = append(got.Limits, api.ResourceLimit{Type: "leak"})
	got.ImageOpt.Images[0] = "rewritten"
	got.Profiles[0].Limits[0].Value = "rewritten"

	again := w.ClusterInfo()
	if again == nil {
		t.Fatal("ClusterInfo() returned nil on second call")
		return // unreachable, but satisfies staticcheck SA5011
	}
	if len(again.Queues) != 1 || again.Queues[0] != "default" {
		t.Errorf("ClusterInfo() Queues leaked: %+v", again.Queues)
	}
	if len(again.Limits) != 1 || again.Limits[0].Value != "4" {
		t.Errorf("ClusterInfo() Limits leaked: %+v", again.Limits)
	}
	if len(again.ImageOpt.Images) != 1 || again.ImageOpt.Images[0] != "ubuntu:24.04" {
		t.Errorf("ClusterInfo() ImageOpt.Images leaked: %+v", again.ImageOpt.Images)
	}
	if len(again.Profiles) != 1 || len(again.Profiles[0].Limits) != 1 ||
		again.Profiles[0].Limits[0].Value != "1Gi" {
		t.Errorf("ClusterInfo() Profiles[0].Limits leaked: %+v", again.Profiles)
	}
}

// TestAccessorsReturnInsertionOrder asserts that bare-noun accessors return
// recorded entries in the order they were written.
func TestAccessorsReturnInsertionOrder(t *testing.T) {
	w := plugintest.NewMockStreamResponseWriter()
	_ = w.WriteJobStatus("a", "A", api.StatusPending, "", "")
	_ = w.WriteJobStatus("b", "B", api.StatusRunning, "", "")
	_ = w.WriteJobStatus("c", "C", api.StatusFinished, "", "")

	statuses := w.Statuses()
	if len(statuses) != 3 {
		t.Fatalf("got %d statuses, want 3", len(statuses))
	}
	wantIDs := []api.JobID{"a", "b", "c"}
	for i, s := range statuses {
		if s.ID != wantIDs[i] {
			t.Errorf("statuses[%d].ID = %q, want %q", i, s.ID, wantIDs[i])
		}
	}

	if w.IsClosed() {
		t.Error("IsClosed() = true before Close()")
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !w.IsClosed() {
		t.Error("IsClosed() = false after Close()")
	}
}
