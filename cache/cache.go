// Package cache provides an in-memory job cache with pub/sub for status updates.
package cache

import (
	"container/list"
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/posit-dev/launcher-go-sdk/api"
	"github.com/posit-dev/launcher-go-sdk/launcher"
)

// JobCache is an in-memory cache of jobs that permits subscribing to granular updates.
// This type is safe to use across multiple goroutines.
type JobCache struct {
	mu            sync.RWMutex
	lgr           *slog.Logger
	store         jobStore
	ch            chan *statusUpdate
	updates       *subManager
	updatesByID   map[api.JobID]*subManager
	updatesByUser map[string]*subManager
	done          chan struct{}
}

// NewJobCache returns a JobCache backed by an in-memory store.
func NewJobCache(ctx context.Context, lgr *slog.Logger) (*JobCache, error) {
	r := &JobCache{
		lgr:           lgr,
		store:         newInMemoryStore(),
		ch:            make(chan *statusUpdate, 64),
		updatesByID:   make(map[api.JobID]*subManager),
		updatesByUser: make(map[string]*subManager),
		done:          make(chan struct{}),
	}
	r.updates = &subManager{}
	go func() {
		defer func() {
			if v := recover(); v != nil {
				r.lgr.Error("Panic in status update goroutine",
					"panic", v)
			}
			closed := r.updates.Close()
			for _, mgr := range r.updatesByID {
				closed += mgr.Close()
			}
			for _, mgr := range r.updatesByUser {
				closed += mgr.Close()
			}
			r.lgr.Debug("Removed remaining status update subscribers",
				"count", closed)
			r.done <- struct{}{}
		}()
		r.lgr.Debug("Listening for job status updates")
		for {
			select {
			case <-ctx.Done():
			case u, ok := <-r.ch:
				if !ok {
					break
				}
				r.lgr.Debug("Got status update; acquiring lock",
					"id", u.ID, "user", u.User, "status", u.Status)
				r.mu.Lock()
				notified := r.updates.Notify(u)
				if mgr, ok := r.updatesByID[u.ID]; ok {
					notified += mgr.Notify(u)
				}
				if mgr, ok := r.updatesByUser[u.User]; ok {
					notified += mgr.Notify(u)
				}
				r.mu.Unlock()
				if notified == 0 {
					r.lgr.Debug("No subscribers to notify")
				} else {
					r.lgr.Debug(
						"Subscribers notified of status update",
						"count", notified)
				}
				continue
			}
			break
		}
	}()
	return r, nil
}

// Close sends any remaining status updates and closes open channels.
func (r *JobCache) Close() error {
	r.lgr.Debug("Closing job cache")
	close(r.ch)
	<-r.done
	if err := r.store.Close(); err != nil {
		return fmt.Errorf("failed to close job cache: %w", err)
	}
	return nil
}

// Lookup passes a callback the cached job with a given id or returns an error.
func (r *JobCache) Lookup(user string, id api.JobID, fn func(*api.Job)) error {
	notfound := false
	//nolint:errcheck // in-memory store never returns errors from View
	r.store.View(string(id), func(job *api.Job) {
		if job == nil || (user != "*" && job.User != user) {
			notfound = true
			return
		}
		fn(job)
	})
	if !notfound {
		return nil
	}
	if user == "*" {
		return api.Errorf(api.CodeJobNotFound, "Job %s not found", id)
	}
	return api.Errorf(api.CodeJobNotFound, "Job %s not found for user %s", id,
		user)
}

// AddOrUpdate either caches a copy of a given job object or updates an existing
// one based on it. It then notifies any clients streaming updates.
func (r *JobCache) AddOrUpdate(job *api.Job) error {
	// updated flag not needed; new-vs-existing is already logged inside the callback.
	_, err := r.store.Update(job.ID, func(cur *api.Job) *api.Job {
		if cur == nil {
			r.lgr.Debug("Added job to store", "id", job.ID,
				"status", job.Status)
			r.notify(job)
			return job
		}
		if job.Status != cur.Status || job.StatusMsg != cur.StatusMsg {
			r.notify(job)
		}
		return job
	})
	if err != nil {
		return fmt.Errorf("failed to write job to store: %w", err)
	}
	return nil
}

// Update passes a callback the cached job with a given id or returns an error.
// The result of the callback is written back to the cache.
func (r *JobCache) Update(user string, id api.JobID, fn func(*api.Job) *api.Job) error {
	notfound := false
	updated, err := r.store.Update(string(id), func(cur *api.Job) *api.Job {
		if cur == nil || (user != "*" && cur.User != user) {
			notfound = true
			return cur
		}
		oldStatus, oldStatusMsg := cur.Status, cur.StatusMsg
		job := fn(cur)
		if job.Status != oldStatus || job.StatusMsg != oldStatusMsg {
			r.notify(job)
		}
		return job
	})
	if err != nil {
		return fmt.Errorf("failed to write job to store: %w", err)
	}
	if notfound {
		return api.Errorf(api.CodeJobNotFound,
			"Job %s not found for user %s", id, user)
	}
	if updated {
		r.lgr.Debug("Updated job in store", "id", id)
	}
	return nil
}

// WriteJob is a convenience method for implementing Plugin.GetJob(). It looks
// up the job by ID and writes it (or an error) to the ResponseWriter. The
// coupling with ResponseWriter is intentional — it reduces boilerplate in
// plugin implementations at the cost of a direct dependency on the launcher
// package.
func (r *JobCache) WriteJob(w launcher.ResponseWriter, user string, id api.JobID) {
	err := r.Lookup(user, id, func(job *api.Job) {
		//nolint:errcheck // fire-and-forget convenience wrapper
		w.WriteJobs([]*api.Job{job})
	})
	if err != nil {
		//nolint:errcheck // nothing useful to do if writing the error also fails
		w.WriteError(err)
		return
	}
}

// WriteJobs is a convenience method for implementing Plugin.GetJobs(). It
// queries jobs matching the filter and writes them to the ResponseWriter.
func (r *JobCache) WriteJobs(w launcher.ResponseWriter, user string, filter *api.JobFilter) {
	//nolint:errcheck // in-memory store never returns errors from JobsForUser
	r.store.JobsForUser(user, filter, func(jobs []*api.Job) {
		//nolint:errcheck // fire-and-forget convenience wrapper
		w.WriteJobs(jobs)
	})
}

// RunningJobContext returns a context that is canceled when a job is no longer
// running. It is useful for implementing Plugin.GetJobOutput() and
// Plugin.GetJobResourceUtil().
func (r *JobCache) RunningJobContext(parent context.Context, user string, id api.JobID) (context.Context, error) {
	var done bool
	err := r.Lookup(user, id, func(job *api.Job) {
		done = api.TerminalStatus(job.Status)
	})
	if err != nil {
		return nil, err
	}
	if done {
		return nil, api.Errorf(api.CodeJobNotRunning,
			"The job is not currently running")
	}
	ctx, cancel := context.WithCancel(parent)
	ch := make(chan *statusUpdate, 1)
	r.subscribeToID(ctx, id, ch)
	// Re-check terminal status after subscribing to close the race window
	// where the job could have ended between the initial lookup and the
	// subscription.
	err = r.Lookup(user, id, func(job *api.Job) {
		done = api.TerminalStatus(job.Status)
	})
	if done || err != nil {
		cancel()
		return nil, api.Errorf(api.CodeJobNotRunning,
			"The job is not currently running")
	}
	go func() {
		for {
			select {
			case <-parent.Done():
				cancel()
				return
			case u, ok := <-ch:
				if !ok || api.TerminalStatus(u.Status) {
					cancel()
					return
				}
			}
		}
	}()
	return ctx, nil
}

// StreamJobStatus can be used to implement Plugin.GetJobStatus().
func (r *JobCache) StreamJobStatus(ctx context.Context, w launcher.StreamResponseWriter, user string, id api.JobID) {
	done := false
	err := r.Lookup(user, id, func(job *api.Job) {
		//nolint:errcheck // fire-and-forget convenience wrapper
		w.WriteJobStatus(api.JobID(job.ID), job.Status, job.StatusMsg)
		// Break off early if we know there will be no further updates.
		if api.TerminalStatus(job.Status) {
			done = true
			return
		}
	})
	if err != nil {
		//nolint:errcheck // nothing useful to do if writing the error also fails
		w.WriteError(err)
		return
	}
	if done {
		return
	}
	ch := make(chan *statusUpdate, 1)
	r.subscribeToID(ctx, id, ch)
	for {
		select {
		case <-ctx.Done():
			return
		case j, ok := <-ch:
			if !ok {
				return
			}
			//nolint:errcheck // fire-and-forget convenience wrapper
			w.WriteJobStatus(j.ID, j.Status, j.StatusMsg)
		}
	}
}

// StreamJobStatuses can be used to implement Plugin.GetJobStatuses().
func (r *JobCache) StreamJobStatuses(ctx context.Context, w launcher.StreamResponseWriter, user string) {
	//nolint:errcheck // in-memory store never returns errors from JobsForUser
	r.store.JobsForUser(user, nil, func(jobs []*api.Job) {
		for _, job := range jobs {
			//nolint:errcheck // fire-and-forget convenience wrapper
			w.WriteJobStatus(api.JobID(job.ID), job.Status,
				job.StatusMsg)
		}
	})
	ch := make(chan *statusUpdate, 1)
	r.subscribeToUser(ctx, user, ch)
	for {
		select {
		case <-ctx.Done():
			return
		case j, ok := <-ch:
			if !ok {
				return
			}
			//nolint:errcheck // fire-and-forget convenience wrapper
			w.WriteJobStatus(j.ID, j.Status, j.StatusMsg)
		}
	}
}

// All returns a read-only iterator over all jobs in the cache.
func (r *JobCache) All(yield func(*api.Job) bool) {
	r.store.Jobs("*", nil)(yield)
}

// subscribeToID registers a channel to receive notifications when the job with
// the given ID changes status. After the passed context is closed, notifications
// will cease and the channel will be closed.
func (r *JobCache) subscribeToID(ctx context.Context, id api.JobID, ch chan<- *statusUpdate) {
	r.mu.Lock()
	defer r.mu.Unlock()
	mgr := r.updatesByID[id]
	if mgr == nil {
		mgr = &subManager{}
		r.updatesByID[id] = mgr
	}
	mgr.Subscribe(ctx, ch)
}

// subscribeToUser registers a channel to receive notifications when any job
// owned by the given user changes status. After the passed context is closed,
// notifications will cease and the channel will be closed.
func (r *JobCache) subscribeToUser(ctx context.Context, user string, ch chan<- *statusUpdate) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if user == "*" {
		r.updates.Subscribe(ctx, ch)
		r.lgr.Debug("Added subscription for all users", "count",
			r.updates.Count())
		return
	}
	mgr := r.updatesByUser[user]
	if mgr == nil {
		mgr = &subManager{}
		r.updatesByUser[user] = mgr
	}
	mgr.Subscribe(ctx, ch)
}

// Prune returns an iterator over "stale" jobs, i.e. those that (1) have a
// terminal status; and (2) have not been updated in the given interval. When
// the iterator finishes all yielded jobs are removed from the cache. The
// iterator can be used to clean up external resources associated with the job
// before they are deleted.
func (r *JobCache) Prune(interval time.Duration) func(func(*api.Job) bool) {
	cutoff := time.Now().Add(-interval)
	terminal := &api.JobFilter{
		Statuses: []string{
			api.StatusCanceled, api.StatusFailed,
			api.StatusFinished, api.StatusKilled,
		},
	}
	return func(yield func(*api.Job) bool) {
		var candidates []string
		for job := range r.store.Jobs("*", terminal) {
			if job.LastUpdated == nil ||
				job.LastUpdated.After(cutoff) {
				continue
			}
			if !yield(job) {
				break
			}
			candidates = append(candidates, job.ID)
		}
		for _, id := range candidates {
			if err := r.store.Delete(id); err != nil {
				r.lgr.Error("Failed to prune job", "id", id,
					"error", err)
			}
		}
	}
}

// subManager manages the lifecycle and notifications of individual
// subscriptions. This type is *not* safe to use concurrently from multiple
// goroutines; you must serialize operations that use it.
type subManager struct {
	// Note: we use a linked list internally because we're constantly adding
	// and removing elements, and using a slice for that is icky.
	subs list.List
}

// Count returns the number of subscriptions.
func (s *subManager) Count() int {
	return s.subs.Len()
}

// Subscribe registers a channel to receive notifications when jobs change
// status. Notifications will not be sent after the passed context is canceled
// and the channel will be closed.
func (s *subManager) Subscribe(ctx context.Context, ch chan<- *statusUpdate) {
	s.subs.PushBack(&subscriber{ctx, ch})
}

// Notify sends the updated job status to existing subscribers, if any.
func (s *subManager) Notify(u *statusUpdate) int {
	elt := s.subs.Front()
	notified := 0
	for elt != nil {
		sub, ok := elt.Value.(*subscriber)
		if !ok {
			panic("unexpected type in subscriber list")
		}
		if sub.Context.Err() != nil {
			close(sub.Channel)
			next := elt.Next()
			s.subs.Remove(elt)
			elt = next
			continue
		}
		select {
		case sub.Channel <- u:
			notified++
		default:
			// Subscriber isn't consuming fast enough; skip this update.
			// The subscriber will miss this status change but may receive
			// later updates. No error is reported to the subscriber.
		}
		elt = elt.Next()
	}
	return notified
}

// Close closes the channels of any remaining subscribers and returns the number
// of channels closed this way. It does not send an update.
func (s *subManager) Close() int {
	var closed int
	elt := s.subs.Front()
	for elt != nil {
		removed := s.subs.Remove(elt)
		sub, ok := removed.(*subscriber)
		if !ok {
			panic("unexpected type in subscriber list")
		}
		close(sub.Channel)
		elt = s.subs.Front()
		closed++
	}
	return closed
}

type statusUpdate struct {
	ID        api.JobID
	User      string
	Status    string
	StatusMsg string
}

func newStatusUpdateFromJob(job *api.Job) *statusUpdate {
	return &statusUpdate{
		api.JobID(job.ID), job.User, job.Status, job.StatusMsg,
	}
}

// notify sends a status update to the internal channel without blocking. This
// is important because sends may happen while holding a store lock. If the
// channel buffer is full the update is dropped and a warning is logged;
// subscribers may miss intermediate status changes but will receive subsequent
// updates if they continue listening.
func (r *JobCache) notify(job *api.Job) {
	u := newStatusUpdateFromJob(job)
	select {
	case r.ch <- u:
	default:
		r.lgr.Warn("Status update channel full, dropping update",
			"id", job.ID, "status", job.Status)
	}
}

type subscriber struct {
	Context context.Context
	Channel chan<- *statusUpdate
}
