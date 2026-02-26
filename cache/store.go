package cache

import (
	"sort"
	"sync"
	"time"

	"github.com/posit-dev/launcher-go-sdk/api"
)

// Tracks jobs and their updates.
type jobStore interface {
	// View passes a callback a read-only view of the job with the given ID,
	// or nil if there is none.
	View(id string, fn func(*api.Job)) error

	// Updates passes a callback the existing job with the given ID, or nil
	// if there is none. The return value is taken as the updated value.
	Update(id string, fn func(existing *api.Job) *api.Job) (bool, error)

	// Jobs returns an iterator over a read-only view of all jobs for a
	// given user (with an optional filter).
	Jobs(user string, filter *api.JobFilter) func(func(*api.Job) bool)

	// JobsForUser queries a read-only view of all jobs for a given user
	// (with an optional filter) and passes it to the given callback.
	JobsForUser(user string, filter *api.JobFilter, fn func([]*api.Job)) error

	// Delete removes the job with the given ID from the store.
	Delete(id string) error

	// Count is the number of jobs in the store.
	Count() (int, error)

	// Close frees up resources, if needed.
	Close() error
}

// Creates an in-memory job store.
func newInMemoryStore() jobStore {
	return &inMemoryStore{
		jobs: make(map[string]*api.Job),
	}
}

type inMemoryStore struct {
	sync.RWMutex
	jobs map[string]*api.Job
}

func (s *inMemoryStore) Close() error {
	return nil
}

func (s *inMemoryStore) Count() (int, error) {
	s.RLock()
	defer s.RUnlock()
	return len(s.jobs), nil
}

func (s *inMemoryStore) View(id string, fn func(*api.Job)) error {
	s.RLock()
	defer s.RUnlock()
	job := s.jobs[id]
	if job != nil {
		snapshot := *job
		fn(&snapshot)
	} else {
		fn(nil)
	}
	return nil
}

func (s *inMemoryStore) Update(id string, fn func(*api.Job) *api.Job) (bool, error) {
	s.Lock()
	defer s.Unlock()
	var new *api.Job
	cur, ok := s.jobs[id]
	// Pass a snapshot to fn so it cannot mutate the stored object directly.
	// This also lets syncJob2 correctly diff cur against the callback's result.
	var snapshot *api.Job
	if cur != nil {
		c := *cur
		snapshot = &c
	}
	in := fn(snapshot)
	if in == nil {
		return false, nil
	}
	if !ok {
		// Work from a copy to ensure ownership.
		new := &api.Job{}
		*new = *in
		s.jobs[id] = new
		return false, nil
	}
	new, updated := syncJob2(cur, in)
	s.jobs[id] = new
	return updated, nil
}

func (s *inMemoryStore) Jobs(user string, filter *api.JobFilter) func(func(*api.Job) bool) {
	return func(yield func(*api.Job) bool) {
		s.RLock()
		defer s.RUnlock()
		for _, job := range s.jobs {
			if user != "*" && job.User != user {
				continue
			}
			if filter != nil && !filter.Includes(job) {
				continue
			}
			snapshot := *job
			if !yield(&snapshot) {
				break
			}
		}
	}
}

func (s *inMemoryStore) JobsForUser(user string, filter *api.JobFilter, fn func([]*api.Job)) error {
	s.RLock()
	defer s.RUnlock()
	out := []*api.Job{}
	ids := []string{}
	for _, job := range s.jobs {
		if user != "*" && job.User != user {
			continue
		}
		if filter != nil && !filter.Includes(job) {
			continue
		}
		ids = append(ids, job.ID)
	}
	// Deterministic output order.
	sort.Strings(ids)
	for _, id := range ids {
		snapshot := *s.jobs[id]
		if filter == nil {
			out = append(out, &snapshot)
		} else {
			out = append(out, snapshot.WithFields(filter.Fields))
		}
	}
	fn(out)
	return nil
}

func (s *inMemoryStore) Delete(id string) error {
	s.Lock()
	defer s.Unlock()
	delete(s.jobs, id)
	return nil
}

// syncJob2 updates the current job with fields from an input. This is necessary
// because semantically only a subset of job fields can actually be updated.
// Returns the updated job and a boolean indicating whether a change was made.
func syncJob2(job *api.Job, in *api.Job) (*api.Job, bool) {
	updated := false
	if in.Status != job.Status {
		job.Status = in.Status
		updated = true
	}
	if in.StatusMsg != job.StatusMsg {
		job.StatusMsg = in.StatusMsg
		updated = true
	}
	if in.StatusCode != job.StatusCode {
		job.StatusCode = in.StatusCode
		updated = true
	}
	if in.Pid != nil && (job.Pid == nil || *in.Pid != *job.Pid) {
		job.Pid = in.Pid
		updated = true
	}
	if in.Host != "" && in.Host != job.Host {
		job.Host = in.Host
		updated = true
	}
	if in.ExitCode != nil && (job.ExitCode == nil || *in.ExitCode != *job.ExitCode) {
		job.ExitCode = in.ExitCode
		updated = true
	}
	if in.Submitted != nil && (job.Submitted == nil || !in.Submitted.Equal(*job.Submitted)) {
		job.Submitted = in.Submitted
		updated = true
	}
	// Allow manually updating the LastUpdated timestamp.
	if in.LastUpdated != nil {
		job.LastUpdated = in.LastUpdated
		updated = true
		return job, updated
	}
	if !updated {
		return job, false
	}
	now := time.Now().UTC()
	job.LastUpdated = &now
	return job, updated
}
