package statistic

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTrackerIDsAreUniqueValidUUIDs(t *testing.T) {
	const count = 1000
	seen := make(map[[16]byte]struct{}, count)

	for range count {
		id := newTrackerID()
		_, repeated := seen[id]
		require.False(t, repeated, "tracker ID repeated")
		seen[id] = struct{}{}
		require.EqualValues(t, 4, (id[6]>>4)&0x0f)
		require.EqualValues(t, 2, (id[8]>>6)&0x03)
	}
}
