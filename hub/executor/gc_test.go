package executor

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGCSchedulerCoalescesReloadBursts(t *testing.T) {
	var collections atomic.Int32
	scheduler := newGCScheduler(20*time.Millisecond, func() {
		collections.Add(1)
		time.Sleep(20 * time.Millisecond)
	})

	scheduler.Schedule()
	require.Eventually(t, func() bool { return collections.Load() == 1 }, 200*time.Millisecond, 2*time.Millisecond)

	for range 20 {
		scheduler.Schedule()
	}
	require.Eventually(t, func() bool { return collections.Load() == 2 }, 500*time.Millisecond, 5*time.Millisecond)
}

func TestGCSchedulerKeepsWorkQueuedDuringCollection(t *testing.T) {
	var collections atomic.Int32
	release := make(chan struct{})
	scheduler := newGCScheduler(10*time.Millisecond, func() {
		collections.Add(1)
		<-release
	})

	scheduler.Schedule()
	require.Eventually(t, func() bool { return collections.Load() == 1 }, 200*time.Millisecond, 2*time.Millisecond)
	scheduler.Schedule()
	close(release)

	require.Eventually(t, func() bool { return collections.Load() == 2 }, 500*time.Millisecond, 5*time.Millisecond)
}
