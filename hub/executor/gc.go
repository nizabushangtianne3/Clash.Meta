package executor

import (
	"runtime"
	"sync"
	"time"
)

const gcDebounceInterval = 100 * time.Millisecond

// gcScheduler coalesces configuration reloads into one background collection.
// Provider parsing can publish many applies back to back; an unconditional GC
// per apply turns that burst into repeated full mark phases.
type gcScheduler struct {
	collect func()

	mu         sync.Mutex
	timer      *time.Timer
	scheduled  bool
	collecting bool
	pending    bool
}

func newGCScheduler(interval time.Duration, collect func()) *gcScheduler {
	s := &gcScheduler{collect: collect}
	s.timer = time.AfterFunc(interval, s.run)
	return s
}

func (s *gcScheduler) Schedule() {
	s.mu.Lock()
	if s.collecting {
		s.pending = true
		s.mu.Unlock()
		return
	}
	if s.scheduled {
		s.mu.Unlock()
		return
	}
	s.scheduled = true
	s.mu.Unlock()
	s.timer.Reset(gcDebounceInterval)
}

func (s *gcScheduler) run() {
	s.mu.Lock()
	s.scheduled = false
	s.collecting = true
	s.mu.Unlock()

	s.collect()

	s.mu.Lock()
	s.collecting = false
	if s.pending {
		s.pending = false
		s.scheduled = true
		s.mu.Unlock()
		s.timer.Reset(gcDebounceInterval)
		return
	}
	s.mu.Unlock()
}

var runtimeGCScheduler = newGCScheduler(gcDebounceInterval, runtime.GC)
