package singledo

import (
	"sync"
	"testing"
	"time"

	"github.com/metacubex/mihomo/common/atomic"

	"github.com/stretchr/testify/assert"
)

func TestBasic(t *testing.T) {
	t.Parallel()
	single := NewSingle[int](time.Millisecond * 200)
	foo := 0
	shardCount := atomic.NewInt32(0)
	call := func() (int, error) {
		foo++
		time.Sleep(time.Millisecond * 20)
		return 0, nil
	}

	var wg sync.WaitGroup
	const n = 5
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			_, _, shard := single.Do(call)
			if shard {
				shardCount.Add(1)
			}
			wg.Done()
		}()
	}

	wg.Wait()
	assert.Equal(t, 1, foo)
	assert.Equal(t, int32(4), shardCount.Load())
}

func TestTimer(t *testing.T) {
	t.Parallel()
	single := NewSingle[int](time.Millisecond * 200)
	foo := 0
	callM := func() (int, error) {
		foo++
		return 0, nil
	}

	_, _, _ = single.Do(callM)
	time.Sleep(100 * time.Millisecond)
	_, _, shard := single.Do(callM)

	assert.Equal(t, 1, foo)
	assert.True(t, shard)
}

func TestReset(t *testing.T) {
	t.Parallel()
	single := NewSingle[int](time.Millisecond * 200)
	foo := 0
	callM := func() (int, error) {
		foo++
		return 0, nil
	}

	_, _, _ = single.Do(callM)
	single.Reset()
	_, _, _ = single.Do(callM)

	assert.Equal(t, 2, foo)
}

func TestResetDuringCallRetriesWaiter(t *testing.T) {
	t.Parallel()
	single := NewSingle[int](time.Minute)
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	firstResult := make(chan int, 1)
	go func() {
		value, _, _ := single.Do(func() (int, error) {
			close(firstStarted)
			<-releaseFirst
			return 1, nil
		})
		firstResult <- value
	}()
	<-firstStarted

	waiterResult := make(chan int, 1)
	go func() {
		value, _, _ := single.Do(func() (int, error) {
			return 2, nil
		})
		waiterResult <- value
	}()

	time.Sleep(10 * time.Millisecond)
	single.Reset()
	close(releaseFirst)

	assert.Equal(t, 1, <-firstResult)
	assert.Equal(t, 2, <-waiterResult)
}

// A reader that only looks must not take the lock the single flight is built
// on: it answers a finished result, reports a miss while the flight runs, and
// leaves a call in flight alone.
func TestCachedAnswersAFinishedResultAndStaysOutOfTheWayDuringOne(t *testing.T) {
	t.Parallel()
	single := NewSingle[int](time.Minute)
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _, _ = single.Do(func() (int, error) {
			close(started)
			<-release
			return 3, nil
		})
	}()
	<-started

	if value, _, ok := single.Cached(); ok {
		t.Fatalf("Cached answered %d before any result existed", value)
	}
	if _, ok := single.CachedValid(0, func(int) bool { return false }); ok {
		t.Fatal("CachedValid answered before any result existed")
	}
	close(release)
	<-done

	value, err, ok := single.Cached()
	assert.True(t, ok)
	assert.NoError(t, err)
	assert.Equal(t, 3, value)
}

// CachedValid drops a result that stale rejects once the age has elapsed, and
// keeps one that is younger than the age without consulting stale at all.
func TestCachedValidDropsOnlyAStaleResult(t *testing.T) {
	t.Parallel()
	single := NewSingle[int](time.Minute)
	calls := 0
	compute := func() (int, error) {
		calls++
		return 7, nil
	}
	_, _, _ = single.Do(compute)

	value, ok := single.CachedValid(time.Minute, func(v int) bool { return true })
	assert.True(t, ok)
	assert.Equal(t, 7, value)

	_, ok = single.CachedValid(0, func(v int) bool { return true })
	assert.False(t, ok)

	_, _, _ = single.Do(compute)
	assert.Equal(t, 2, calls, "a dropped result is computed again")

	_, ok = single.CachedValid(0, func(int) bool { return false })
	assert.True(t, ok, "a result stale does not reject is served")
}

func TestForgetAfterDropsOnlyAStaleResult(t *testing.T) {
	t.Parallel()
	single := NewSingle[int](time.Minute)
	calls := 0
	compute := func() (int, error) {
		calls++
		return 5, nil
	}
	_, _, _ = single.Do(compute)

	single.Forget(func(int) bool { return true })
	_, _, _ = single.Do(compute)
	assert.Equal(t, 2, calls)

	single.ForgetAfter(time.Minute, func(int) bool { return true })
	_, _, shared := single.Do(compute)
	assert.True(t, shared)
	assert.Equal(t, 2, calls, "a result younger than the age is kept")
}

// Reads, resets and computations all run at once: the value a reader reports
// is one the computation produced, never a half-written one.
func TestConcurrentCachedReadsAndResets(t *testing.T) {
	t.Parallel()
	single := NewSingle[int](time.Minute)
	stop := make(chan struct{})
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			if value, _, ok := single.Cached(); ok && (value < 0 || value > 999) {
				t.Errorf("Cached reported %d", value)
				return
			}
		}
	}()

	for reader := 0; reader < 3; reader++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				if value, ok := single.CachedValid(0, func(v int) bool { return v < 0 }); ok && value < 0 {
					t.Errorf("CachedValid served %d", value)
					return
				}
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		for round := 0; ; round++ {
			select {
			case <-stop:
				return
			default:
			}
			_, _, _ = single.Do(func() (int, error) { return round % 1000, nil })
			single.Reset()
		}
	}()

	time.Sleep(50 * time.Millisecond)
	close(stop)
	wg.Wait()
}
