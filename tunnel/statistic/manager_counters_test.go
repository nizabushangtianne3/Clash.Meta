package statistic

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The dashboard reads these counters every second and the swap path runs when
// a profile is replaced, so both are pinned: what a push leaves behind, what a
// swap takes away, and that a swap leaves nothing for the next one.
func TestManagerCountersAnswerWhatAPushLeftAndASwapTakes(t *testing.T) {
	manager := &Manager{stripes: newCounterStripes()}
	stripe := &manager.stripes[0]

	stripe.pushUploaded(100, false)
	stripe.pushUploaded(7, true)
	stripe.pushDownloaded(40, false)
	stripe.pushDownloaded(3, true)

	totalUp, totalDown := manager.Total()
	require.Equal(t, int64(107), totalUp)
	require.Equal(t, int64(43), totalDown)

	// The blips a handler tick fills are the swaps, not the pushes.
	require.Equal(t, int64(107), manager.swapUploadTemp())
	require.Equal(t, int64(43), manager.swapDownloadTemp())
	require.Equal(t, int64(0), manager.swapUploadTemp(), "a swap leaves the counter at zero")
	require.Equal(t, int64(0), manager.swapDownloadTemp())

	// Only what travelled through a proxy counts in the proxy counters.
	require.Equal(t, int64(100), manager.swapProxyUploadTemp())
	require.Equal(t, int64(40), manager.swapProxyDownloadTemp())
	require.Equal(t, int64(0), manager.swapProxyUploadTemp())
	require.Equal(t, int64(0), manager.swapProxyDownloadTemp())

	up, down := manager.Now()
	require.Equal(t, int64(0), up, "a blip only moves on a tick")
	require.Equal(t, int64(0), down)

	manager.uploadBlip.Store(manager.swapUploadTemp())
	manager.downloadBlip.Store(manager.swapDownloadTemp())
	require.Equal(t, int64(0), manager.uploadBlip.Load(), "nothing new was pushed")

	manager.ResetStatistic()
	totalUp, totalDown = manager.Total()
	require.Equal(t, int64(0), totalUp)
	require.Equal(t, int64(0), totalDown)
	require.Equal(t, int64(0), manager.swapProxyUploadTemp())
	require.Equal(t, int64(0), manager.swapProxyDownloadTemp())
}

// The count the dashboard answers is the table the manager holds, and the
// lookups a row needs go by the id the tracker carries.
func TestManagerSizeAndLookupFollowJoinsAndLeaves(t *testing.T) {
	manager := &Manager{stripes: newCounterStripes()}
	tracker := newTestTracker(t)

	require.Equal(t, 0, manager.Size())
	require.Nil(t, manager.Get(tracker.UUID.String()), "a tracker nobody joined is not there")

	manager.Join(tracker)
	require.Equal(t, 1, manager.Size())
	require.NotNil(t, manager.Get(tracker.UUID.String()))

	seen := 0
	manager.Range(func(c Tracker) bool {
		seen++
		require.Equal(t, tracker.UUID, c.Info().UUID)
		return true
	})
	require.Equal(t, 1, seen)

	manager.Leave(tracker)
	require.Equal(t, 0, manager.Size())
	require.Nil(t, manager.Get(tracker.UUID.String()))
}

// The table the host answers whole is encoded straight out of the manager, so
// the shape is pinned with and without a connection in it.
func TestManagerAppendJSONCarriesTheCountersAndTheRows(t *testing.T) {
	empty := &Manager{stripes: newCounterStripes()}
	require.Equal(t, `{"downloadTotal":0,"uploadTotal":0,"connections":[],"memory":0}`,
		string(empty.AppendJSON(nil)))

	manager := &Manager{stripes: newCounterStripes()}
	stripe := &manager.stripes[0]
	stripe.pushUploaded(12, false)
	stripe.pushDownloaded(34, false)
	tracker := newTestTracker(t)
	manager.Join(tracker)

	table := string(manager.AppendJSON(nil))
	require.True(t, strings.HasPrefix(table, `{"downloadTotal":34,"uploadTotal":12,"connections":[`),
		"table = %s", table)
	require.Contains(t, table, tracker.UUID.String())
	require.True(t, strings.HasSuffix(table, `],"memory":0}`), "table = %s", table)
}
