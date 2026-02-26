package plugintest

import (
	"testing"

	"github.com/posit-dev/launcher-go-sdk/api"
)

// AssertNoError asserts that no errors were written to the ResponseWriter.
func AssertNoError(t *testing.T, w *MockResponseWriter) {
	t.Helper()
	if w.HasError() {
		t.Errorf("expected no errors, but got %d error(s): %v", len(w.Errors), w.Errors)
	}
}

// AssertError asserts that at least one error was written.
func AssertError(t *testing.T, w *MockResponseWriter) {
	t.Helper()
	if !w.HasError() {
		t.Error("expected an error, but none was written")
	}
}

// AssertErrorCode asserts that an error with the specified code was written.
func AssertErrorCode(t *testing.T, w *MockResponseWriter, code api.ErrCode) {
	t.Helper()
	if !w.HasError() {
		t.Errorf("expected error with code %v, but no errors were written", code)
		return
	}
	found := false
	for _, err := range w.Errors {
		if err.Code == code {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error with code %v, but got: %v", code, w.Errors)
	}
}

// AssertErrorMessage asserts that an error containing the specified message was written.
func AssertErrorMessage(t *testing.T, w *MockResponseWriter, msg string) {
	t.Helper()
	if !w.HasError() {
		t.Errorf("expected error containing %q, but no errors were written", msg)
		return
	}
	found := false
	for _, err := range w.Errors {
		if contains(err.Msg, msg) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error containing %q, but got: %v", msg, w.Errors)
	}
}

// AssertJobCount asserts that the expected number of jobs were written.
func AssertJobCount(t *testing.T, w *MockResponseWriter, expected int) {
	t.Helper()
	actual := len(w.AllJobs())
	if actual != expected {
		t.Errorf("expected %d job(s), but got %d", expected, actual)
	}
}

// AssertJobStatus asserts that a job with the specified ID has the expected status.
func AssertJobStatus(t *testing.T, job *api.Job, expected string) {
	t.Helper()
	if job == nil {
		t.Error("job is nil")
		return
	}
	if job.Status != expected {
		t.Errorf("expected job %s to have status %q, but got %q", job.ID, expected, job.Status)
	}
}

// AssertJobID asserts that a job has the expected ID.
func AssertJobID(t *testing.T, job *api.Job, expected string) {
	t.Helper()
	if job == nil {
		t.Error("job is nil")
		return
	}
	if job.ID != expected {
		t.Errorf("expected job ID %q, but got %q", expected, job.ID)
	}
}

// AssertJobUser asserts that a job has the expected user.
func AssertJobUser(t *testing.T, job *api.Job, expected string) {
	t.Helper()
	if job == nil {
		t.Error("job is nil")
		return
	}
	if job.User != expected {
		t.Errorf("expected job user %q, but got %q", expected, job.User)
	}
}

// AssertJobExitCode asserts that a job has the expected exit code.
func AssertJobExitCode(t *testing.T, job *api.Job, expected int) {
	t.Helper()
	if job == nil {
		t.Error("job is nil")
		return
	}
	if job.ExitCode == nil {
		t.Error("job exit code is nil")
		return
	}
	if *job.ExitCode != expected {
		t.Errorf("expected job exit code %d, but got %d", expected, *job.ExitCode)
	}
}

// AssertStreamClosed asserts that Close() was called on a StreamResponseWriter.
func AssertStreamClosed(t *testing.T, w *MockStreamResponseWriter) {
	t.Helper()
	if !w.Closed {
		t.Error("expected stream to be closed, but Close() was not called")
	}
}

// AssertStreamNotClosed asserts that Close() was not called on a StreamResponseWriter.
func AssertStreamNotClosed(t *testing.T, w *MockStreamResponseWriter) {
	t.Helper()
	if w.Closed {
		t.Error("expected stream to be open, but Close() was called")
	}
}

// AssertStatusCount asserts that the expected number of status updates were written.
func AssertStatusCount(t *testing.T, w *MockStreamResponseWriter, expected int) {
	t.Helper()
	actual := w.StatusCount()
	if actual != expected {
		t.Errorf("expected %d status update(s), but got %d", expected, actual)
	}
}

// AssertOutputCount asserts that the expected number of output chunks were written.
func AssertOutputCount(t *testing.T, w *MockStreamResponseWriter, expected int) {
	t.Helper()
	actual := w.OutputCount()
	if actual != expected {
		t.Errorf("expected %d output chunk(s), but got %d", expected, actual)
	}
}

// AssertControlComplete asserts that a control operation completed.
func AssertControlComplete(t *testing.T, w *MockResponseWriter, expectedMsg string) {
	t.Helper()
	if len(w.ControlResults) == 0 {
		t.Error("expected control result, but none was written")
		return
	}
	result := w.ControlResults[len(w.ControlResults)-1]
	if !result.Complete {
		t.Error("expected control operation to be complete, but it was not")
	}
	if expectedMsg != "" && result.Message != expectedMsg {
		t.Errorf("expected control message %q, but got %q", expectedMsg, result.Message)
	}
}

// AssertControlIncomplete asserts that a control operation did not complete.
func AssertControlIncomplete(t *testing.T, w *MockResponseWriter) {
	t.Helper()
	if len(w.ControlResults) == 0 {
		t.Error("expected control result, but none was written")
		return
	}
	result := w.ControlResults[len(w.ControlResults)-1]
	if result.Complete {
		t.Error("expected control operation to be incomplete, but it was complete")
	}
}

// AssertJobHasTag asserts that a job has the specified tag.
func AssertJobHasTag(t *testing.T, job *api.Job, tag string) {
	t.Helper()
	if job == nil {
		t.Error("job is nil")
		return
	}
	for _, t := range job.Tags {
		if t == tag {
			return
		}
	}
	t.Errorf("expected job to have tag %q, but it does not (tags: %v)", tag, job.Tags)
}

// AssertJobHasEnv asserts that a job has an environment variable with the specified name.
func AssertJobHasEnv(t *testing.T, job *api.Job, name string) {
	t.Helper()
	if job == nil {
		t.Error("job is nil")
		return
	}
	for _, env := range job.Env {
		if env.Name == name {
			return
		}
	}
	t.Errorf("expected job to have env var %q, but it does not", name)
}

// AssertJobEnvValue asserts that a job's environment variable has the expected value.
func AssertJobEnvValue(t *testing.T, job *api.Job, name, expectedValue string) {
	t.Helper()
	if job == nil {
		t.Error("job is nil")
		return
	}
	for _, env := range job.Env {
		if env.Name == name {
			if env.Value != expectedValue {
				t.Errorf("expected env var %q to have value %q, but got %q", name, expectedValue, env.Value)
			}
			return
		}
	}
	t.Errorf("expected job to have env var %q, but it does not", name)
}

// AssertJobMatchesFilter asserts that a job matches the given filter.
func AssertJobMatchesFilter(t *testing.T, job *api.Job, filter *api.JobFilter) {
	t.Helper()
	if job == nil {
		t.Error("job is nil")
		return
	}
	if !filter.Includes(job) {
		t.Errorf("expected job %s to match filter, but it does not", job.ID)
	}
}

// AssertJobDoesNotMatchFilter asserts that a job does not match the given filter.
func AssertJobDoesNotMatchFilter(t *testing.T, job *api.Job, filter *api.JobFilter) {
	t.Helper()
	if job == nil {
		t.Error("job is nil")
		return
	}
	if filter.Includes(job) {
		t.Errorf("expected job %s not to match filter, but it does", job.ID)
	}
}

// FindJobByID finds a job by ID in a list of jobs.
func FindJobByID(jobs []*api.Job, id string) *api.Job {
	for _, job := range jobs {
		if job.ID == id {
			return job
		}
	}
	return nil
}

// FindJobsByStatus returns all jobs with the specified status.
func FindJobsByStatus(jobs []*api.Job, status string) []*api.Job {
	var matching []*api.Job
	for _, job := range jobs {
		if job.Status == status {
			matching = append(matching, job)
		}
	}
	return matching
}

// FindJobsByUser returns all jobs for the specified user.
func FindJobsByUser(jobs []*api.Job, user string) []*api.Job {
	var matching []*api.Job
	for _, job := range jobs {
		if job.User == user {
			matching = append(matching, job)
		}
	}
	return matching
}

// contains checks if a string contains a substring.
func contains(s, substr string) bool {
	return substr == "" || len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsRec(s, substr))
}

func containsRec(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
