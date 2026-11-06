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
