package congestion

import (
	"testing"
	"time"

	"github.com/metacubex/quic-go/congestion"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRingBufferGrowsByDoubling(t *testing.T) {
	var ring RingBuffer[int]
	ring.Init(2)

	ring.PushBack(1)
	ring.PushBack(2)
	assert.Equal(t, 2, ring.Len())
	assert.False(t, ring.Empty())

	// the third element has nowhere to go, so the backing array doubles
	ring.PushBack(3)
	assert.Equal(t, 3, ring.Len())
	assert.Equal(t, 1, *ring.Front())
	assert.Equal(t, 3, *ring.Back())
	assert.Equal(t, 2, *ring.Offset(1))

	// a fourth fills the new capacity exactly, a fifth doubles it again
	ring.PushBack(4)
	ring.PushBack(5)
	assert.Equal(t, 5, ring.Len())
	assert.Equal(t, 1, *ring.Front())
	assert.Equal(t, 5, *ring.Back())
}

func TestRingBufferStartsOneWide(t *testing.T) {
	var ring RingBuffer[string]

	// an unallocated buffer has no capacity at all, so the first push gives it one
	ring.PushBack("only")
	assert.Equal(t, 1, ring.Len())
	assert.Equal(t, "only", *ring.Front())
	assert.Equal(t, "only", *ring.Back())

	ring.PushBack("second")
	assert.Equal(t, 2, ring.Len())
	assert.Equal(t, "only", *ring.Front())
	assert.Equal(t, "second", *ring.Back())
}

func TestRingBufferWrapsAround(t *testing.T) {
	var ring RingBuffer[int]
	ring.Init(2)
	for i := 1; i <= 4; i++ {
		ring.PushBack(i)
	}

	// drop two from the front, then push enough to walk the head past the tail
	assert.Equal(t, 1, ring.PopFront())
	assert.Equal(t, 2, ring.PopFront())
	for _, i := range []int{5, 6, 7, 8} {
		ring.PushBack(i)
	}

	assert.Equal(t, 6, ring.Len())
	for i, want := range []int{3, 4, 5, 6, 7, 8} {
		assert.Equal(t, want, *ring.Offset(i), "offset %d", i)
	}
	assert.Equal(t, 3, *ring.Front())
	assert.Equal(t, 8, *ring.Back())
}

func TestRingBufferClearReturnsToZero(t *testing.T) {
	var ring RingBuffer[int]
	ring.Init(4)
	for i := 1; i <= 3; i++ {
		ring.PushBack(i)
	}

	ring.Clear()
	assert.Equal(t, 0, ring.Len())
	assert.True(t, ring.Empty())

	// the buffer is still allocated, so pushing starts again without growing
	ring.PushBack(9)
	assert.Equal(t, 1, ring.Len())
	assert.Equal(t, 9, *ring.Front())
}

func TestRingBufferRejectsReadingAnEmptyBuffer(t *testing.T) {
	var ring RingBuffer[int]

	assert.PanicsWithValue(t,
		"github.com/quic-go/quic-go/internal/utils/ringbuffer: pop from an empty queue",
		func() { ring.PopFront() })
	assert.PanicsWithValue(t,
		"github.com/quic-go/quic-go/internal/utils/ringbuffer: front from an empty queue",
		func() { ring.Front() })
	assert.PanicsWithValue(t,
		"github.com/quic-go/quic-go/internal/utils/ringbuffer: back from an empty queue",
		func() { ring.Back() })
	assert.PanicsWithValue(t,
		"github.com/quic-go/quic-go/internal/utils/ringbuffer: offset from invalid index",
		func() { ring.Offset(0) })

	ring.PushBack(1)
	assert.PanicsWithValue(t,
		"github.com/quic-go/quic-go/internal/utils/ringbuffer: offset from invalid index",
		func() { ring.Offset(1) })
}

func TestPacketNumberIndexedQueueKeepsInsertionOrder(t *testing.T) {
	queue := newPacketNumberIndexedQueue[string](4)

	assert.True(t, queue.IsEmpty())
	assert.Equal(t, int(0), queue.EntrySlotsUsed())

	require.True(t, queue.Emplace(1, ptr("a")))
	require.True(t, queue.Emplace(2, ptr("b")))

	assert.False(t, queue.IsEmpty())
	assert.Equal(t, 2, queue.NumberOfPresentEntries())
	assert.Equal(t, 2, queue.EntrySlotsUsed())
	assert.Equal(t, congestion.PacketNumber(1), queue.FirstPacket())
	assert.Equal(t, congestion.PacketNumber(2), queue.LastPacket())

	assert.Equal(t, "a", *queue.GetEntry(1))
	assert.Equal(t, "b", *queue.GetEntry(2))
	// past the last one ever inserted
	assert.Nil(t, queue.GetEntry(3))
}

func TestPacketNumberIndexedQueueRejectsBadInput(t *testing.T) {
	queue := newPacketNumberIndexedQueue[string](4)

	// an invalid packet number means nothing was sent
	assert.False(t, queue.Emplace(invalidPacketNumber, ptr("a")))
	// a nil entry is a caller mistake
	assert.False(t, queue.Emplace(1, nil))
	assert.True(t, queue.IsEmpty())

	require.True(t, queue.Emplace(1, ptr("a")))
	// inserting at or below the last packet is out of order
	assert.False(t, queue.Emplace(1, ptr("again")))
	assert.Equal(t, 1, queue.NumberOfPresentEntries())
	assert.Equal(t, "a", *queue.GetEntry(1))
}

func TestPacketNumberIndexedQueueFillsGaps(t *testing.T) {
	queue := newPacketNumberIndexedQueue[string](4)

	require.True(t, queue.Emplace(10, ptr("a")))
	require.True(t, queue.Emplace(20, ptr("c")))

	// eleven slots hold three packets, and the nine in between are absent
	assert.Equal(t, 11, queue.EntrySlotsUsed())
	assert.Equal(t, 2, queue.NumberOfPresentEntries())
	assert.Equal(t, congestion.PacketNumber(10), queue.FirstPacket())
	assert.Equal(t, congestion.PacketNumber(20), queue.LastPacket())

	for _, number := range []congestion.PacketNumber{11, 15, 19} {
		assert.Nil(t, queue.GetEntry(number), "packet %d", number)
	}
	assert.Equal(t, "a", *queue.GetEntry(10))
	assert.Equal(t, "c", *queue.GetEntry(20))
	assert.Nil(t, queue.GetEntry(9))
	assert.Nil(t, queue.GetEntry(21))
}

func TestPacketNumberIndexedQueueRemoves(t *testing.T) {
	queue := newPacketNumberIndexedQueue[string](4)
	require.True(t, queue.Emplace(1, ptr("a")))
	require.True(t, queue.Emplace(2, ptr("b")))
	require.True(t, queue.Emplace(3, ptr("c")))

	var removed []string
	require.True(t, queue.Remove(2, func(entry string) { removed = append(removed, entry) }))
	assert.Equal(t, []string{"b"}, removed)
	assert.Nil(t, queue.GetEntry(2))
	assert.Equal(t, 2, queue.NumberOfPresentEntries())
	// the slot is still held, so the last packet has not moved
	assert.Equal(t, 3, queue.EntrySlotsUsed())
	assert.Equal(t, congestion.PacketNumber(3), queue.LastPacket())

	// removing the same packet twice is a no-op the second time
	removed = nil
	assert.False(t, queue.Remove(2, func(entry string) { removed = append(removed, entry) }))
	assert.Nil(t, removed)
	assert.False(t, queue.Remove(99, func(entry string) { removed = append(removed, entry) }))
}

func TestPacketNumberIndexedQueueRemovesUpTo(t *testing.T) {
	queue := newPacketNumberIndexedQueue[string](4)
	require.True(t, queue.Emplace(1, ptr("a")))
	require.True(t, queue.Emplace(2, ptr("b")))
	require.True(t, queue.Emplace(4, ptr("d")))

	// everything below 5 goes, absent slots with it
	queue.RemoveUpTo(5)

	assert.True(t, queue.IsEmpty())
	assert.Equal(t, 0, queue.NumberOfPresentEntries())
	assert.Equal(t, 0, queue.EntrySlotsUsed())
	assert.Equal(t, congestion.PacketNumber(invalidPacketNumber), queue.FirstPacket())
	assert.Equal(t, congestion.PacketNumber(invalidPacketNumber), queue.LastPacket())
}

func TestWindowedFilterTracksTheMinimum(t *testing.T) {
	const window = 100
	filter := NewWindowedFilter[int64, int64](window, MinFilter[int64])

	filter.Update(10, 0)
	assert.Equal(t, int64(10), filter.GetBest())
	assert.Equal(t, int64(10), filter.GetSecondBest())
	assert.Equal(t, int64(10), filter.GetThirdBest())

	// a new low point replaces all three estimates
	filter.Update(5, 10)
	assert.Equal(t, int64(5), filter.GetBest())

	// a higher sample is not the minimum, so it changes nothing yet
	filter.Update(8, 20)
	assert.Equal(t, int64(5), filter.GetBest())
}

func TestWindowedFilterTracksTheMaximum(t *testing.T) {
	const window = 100
	filter := NewWindowedFilter[int64, int64](window, MaxFilter[int64])

	filter.Update(10, 0)
	assert.Equal(t, int64(10), filter.GetBest())

	filter.Update(15, 10)
	assert.Equal(t, int64(15), filter.GetBest())

	filter.Update(12, 20)
	assert.Equal(t, int64(15), filter.GetBest())
}

func TestWindowedFilterResetsWhenTheSampleOutlivesTheWindow(t *testing.T) {
	const window = 100
	filter := NewWindowedFilter[int64, int64](window, MinFilter[int64])

	filter.Update(5, 0)
	filter.Update(6, 10)
	assert.Equal(t, int64(5), filter.GetBest())

	// the last estimate is older than the window, so the new sample starts over
	filter.Update(7, 1000)
	assert.Equal(t, int64(7), filter.GetBest())
	assert.Equal(t, int64(7), filter.GetSecondBest())
	assert.Equal(t, int64(7), filter.GetThirdBest())
}

func TestWindowedFilterResetAndClear(t *testing.T) {
	filter := NewWindowedFilter[int64, int64](100, MinFilter[int64])
	filter.Update(5, 10)
	filter.Update(6, 20)

	filter.Reset(1, 30)
	assert.Equal(t, int64(1), filter.GetBest())
	assert.Equal(t, int64(1), filter.GetSecondBest())
	assert.Equal(t, int64(1), filter.GetThirdBest())

	filter.Clear()
	// a cleared filter reads as zero until something is recorded again
	assert.Equal(t, int64(0), filter.GetBest())
	filter.Update(9, 40)
	assert.Equal(t, int64(9), filter.GetBest())

	filter.SetWindowLength(50)
	assert.Equal(t, int64(9), filter.GetBest())
}

func TestBandwidthFromDelta(t *testing.T) {
	assert.Equal(t, Bandwidth(1), BitsPerSecond)
	assert.Equal(t, Bandwidth(8), BytesPerSecond)

	// a thousand bytes in a second is eight thousand bits in a second
	assert.Equal(t, Bandwidth(8000), BandwidthFromDelta(1000, time.Second))
	// the same bytes in half the time is twice the rate
	assert.Equal(t, Bandwidth(16000), BandwidthFromDelta(1000, 500*time.Millisecond))
	assert.Equal(t, Bandwidth(0), BandwidthFromDelta(0, time.Second))
}

func ptr[T any](value T) *T {
	return &value
}
