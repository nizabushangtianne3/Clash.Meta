package singledo

import (
	"sync"
	"time"
)

type call[T any] struct {
	val T
	err error
}

type Single[T any] struct {
	mux        sync.Mutex
	wait       time.Duration
	call       call[T]
	callCond   sync.Cond
	loading    bool
	generation uint64
	result     Result[T]
	hasResult  bool
}

type Result[T any] struct {
	Val  T
	Err  error
	Time time.Time
}

// Do single.Do likes sync.singleFlight
func (s *Single[T]) Do(fn func() (T, error)) (v T, err error, shared bool) {
	s.mux.Lock()
	if s.hasResult && time.Since(s.result.Time) < s.wait {
		s.mux.Unlock()
		return s.result.Val, s.result.Err, true
	}
	s.hasResult = false // The result has expired, clear it

	if s.loading {
		generation := s.generation
		s.callCond.Wait()
		shared := generation == s.generation
		value, callErr := s.call.val, s.call.err
		s.mux.Unlock()
		return value, callErr, shared
	}

	startGeneration := s.generation
	s.loading = true
	s.mux.Unlock()

	s.call.val, s.call.err = fn()

	s.mux.Lock()
	if s.generation == startGeneration {
		s.result = Result[T]{s.call.val, s.call.err, time.Now()}
		s.hasResult = true
	}
	s.loading = false
	s.callCond.Broadcast()
	s.mux.Unlock()
	return s.call.val, s.call.err, false
}

// Cached reports a Do result that has not expired yet, so a caller can answer
// without building the computation closure for the cache-hit path.
func (s *Single[T]) Cached() (v T, err error, ok bool) {
	s.mux.Lock()
	defer s.mux.Unlock()
	if s.hasResult && time.Since(s.result.Time) < s.wait {
		return s.result.Val, s.result.Err, true
	}
	return v, err, false
}

func (s *Single[T]) Reset() {
	s.mux.Lock()
	s.generation++
	s.loading = false
	s.hasResult = false
	s.callCond.Broadcast()
	s.mux.Unlock()
}

func NewSingle[T any](wait time.Duration) *Single[T] {
	s := &Single[T]{wait: wait}
	s.callCond.L = &s.mux
	return s
}
