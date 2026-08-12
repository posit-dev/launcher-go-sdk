package settings

import "sync/atomic"

// Snapshot holds a single immutable generation of some value of type T,
// swapped atomically on reload. It mirrors the C++
// impls::Snapshot<T> (impls/SettingsSnapshot.hpp): every read goes through
// [Snapshot.Load], which atomically loads the current generation into a
// local value and returns it — never through a shared pointer a concurrent
// [Snapshot.Store] could mutate underneath the reader. Because T is copied
// out on every Load, an individual read can never tear: a reader either
// sees the whole old generation or the whole new one, whichever was current
// when it called Load.
//
// The zero value of Snapshot is NOT ready for use — a nil *T load would
// panic. Always construct with [NewSnapshot], which seeds the first
// generation at construction time. The C++ implementation hit a real bug
// where a snapshot's ctor-time seeding was missing, silently ignoring a
// non-default startup value until the first reload; NewSnapshot's
// mandatory initial value exists specifically to make that bug impossible
// to reintroduce here.
type Snapshot[T any] struct {
	ptr atomic.Pointer[T]
}

// NewSnapshot creates a Snapshot seeded with initial as its first
// generation.
func NewSnapshot[T any](initial T) *Snapshot[T] {
	s := &Snapshot[T]{}
	s.ptr.Store(&initial)
	return s
}

// Load atomically loads the current generation and returns a copy of it.
// The returned value is independent of any subsequent [Snapshot.Store] —
// callers may hold onto it for as long as they like.
func (s *Snapshot[T]) Load() T {
	return *s.ptr.Load()
}

// Store atomically publishes v as the new current generation. Readers that
// already called Load keep the value they got; only subsequent Load calls
// observe v.
func (s *Snapshot[T]) Store(v T) {
	s.ptr.Store(&v)
}
