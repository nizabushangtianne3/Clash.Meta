package deque

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A deque whose buffer has wrapped is the only interesting deque: the head has
// passed the tail and the data lives on both sides of the backing array.
func wrapped() *Deque[int] {
	q := &Deque[int]{}
	// Push more than the starting capacity so the buffer grows, then take from
	// the front until the head sits past the tail.
	for i := range 20 {
		q.PushBack(i)
	}
	for range 13 {
		q.PopFront()
	}
	if q.Len() != 7 {
		panic("wrapped(): expected 7 items, got " + strconv.Itoa(q.Len()))
	}
	return q
}

func TestPushAndPopKeepFIFOAndLIFO(t *testing.T) {
	q := &Deque[int]{}

	assert.Zero(t, q.Cap())
	assert.Zero(t, q.Len())

	q.PushBack(1)
	q.PushBack(2)
	q.PushBack(3)
	assert.Equal(t, 3, q.Len())
	assert.Equal(t, 1, q.Front())
	assert.Equal(t, 3, q.Back())

	// PopFront gives FIFO order.
	assert.Equal(t, 1, q.PopFront())
	assert.Equal(t, 2, q.PopFront())
	assert.Equal(t, 3, q.PopFront())
	assert.Zero(t, q.Len())

	// PopBack gives LIFO order.
	q.PushBack(1)
	q.PushBack(2)
	assert.Equal(t, 2, q.PopBack())
	assert.Equal(t, 1, q.PopBack())

	// PushFront and PopFront are the stack at the other end.
	q.PushFront(9)
	q.PushFront(8)
	assert.Equal(t, 8, q.Front())
	assert.Equal(t, 9, q.Back())
	assert.Equal(t, 8, q.PopFront())
	assert.Equal(t, 9, q.PopFront())
}

// The whole point of the ring buffer: the contents must read the same however
// the head and tail have moved relative to each other.
func TestWrappedDequeKeepsItsOrder(t *testing.T) {
	q := wrapped()

	expected := []int{13, 14, 15, 16, 17, 18, 19}
	got := make([]int, 0, q.Len())
	for i := range q.Len() {
		got = append(got, q.At(i))
	}
	assert.Equal(t, expected, got)

	// Front, Back and index lookups agree with the order.
	assert.Equal(t, 13, q.Front())
	assert.Equal(t, 19, q.Back())
	assert.Equal(t, 15, q.At(2))

	// Set writes through the window.
	q.Set(2, 99)
	assert.Equal(t, 99, q.At(2))

	// Popping from either end agrees.
	assert.Equal(t, 13, q.PopFront())
	assert.Equal(t, 19, q.PopBack())
	// The 15 written two steps ago is still in place.
	assert.Equal(t, []int{14, 99, 16, 17, 18}, q.AppendToSlice(nil))
}

func TestIndexAndRIndexSearchAcrossTheWrap(t *testing.T) {
	q := wrapped()

	assert.Equal(t, 0, q.Index(func(i int) bool { return i == 13 }))
	assert.Equal(t, 6, q.Index(func(i int) bool { return i == 19 }))
	assert.Equal(t, -1, q.Index(func(i int) bool { return i == 99 }))

	// RIndex searches from the back but still reports the front-to-back index,
	// so a repeated value resolves to its last position.
	dup := &Deque[int]{}
	dup.PushBack(1)
	dup.PushBack(2)
	dup.PushBack(1)
	dup.PushBack(3)
	dup.PushBack(1)

	assert.Equal(t, 0, dup.Index(func(i int) bool { return i == 1 }))
	assert.Equal(t, 4, dup.RIndex(func(i int) bool { return i == 1 }))
	assert.Equal(t, 1, dup.Index(func(i int) bool { return i == 2 }))
	assert.Equal(t, 1, dup.RIndex(func(i int) bool { return i == 2 }))

	// Across a wrap the same holds.
	assert.Equal(t, 0, q.RIndex(func(i int) bool { return i == 13 }))
	assert.Equal(t, 6, q.RIndex(func(i int) bool { return i == 19 }))
}

func TestIteratorsWalkBothWaysAndStopEarly(t *testing.T) {
	q := wrapped()

	forward := make([]int, 0, q.Len())
	for item := range q.Iter() {
		forward = append(forward, item)
	}
	assert.Equal(t, []int{13, 14, 15, 16, 17, 18, 19}, forward)

	backward := make([]int, 0, q.Len())
	for item := range q.RIter() {
		backward = append(backward, item)
	}
	assert.Equal(t, []int{19, 18, 17, 16, 15, 14, 13}, backward)

	// Breaking out of the iterator leaves the deque intact.
	seen := 0
	for range q.Iter() {
		seen++
		if seen == 2 {
			break
		}
	}
	assert.Equal(t, 2, seen)
	assert.Equal(t, 7, q.Len())
}

func TestIterPopConsumesAndStopsEarly(t *testing.T) {
	q := wrapped()

	// A full walk drains it.
	for range q.IterPopFront() {
	}
	assert.Zero(t, q.Len())

	// Stopping early leaves the rest in place.
	q = wrapped()
	taken := make([]int, 0, 3)
	for item := range q.IterPopFront() {
		taken = append(taken, item)
		if len(taken) == 3 {
			break
		}
	}
	assert.Equal(t, []int{13, 14, 15}, taken)
	assert.Equal(t, []int{16, 17, 18, 19}, q.AppendToSlice(nil))

	// The same from the back.
	fromBack := make([]int, 0, 2)
	for item := range q.IterPopBack() {
		fromBack = append(fromBack, item)
		if len(fromBack) == 2 {
			break
		}
	}
	assert.Equal(t, []int{19, 18}, fromBack)
	assert.Equal(t, []int{16, 17}, q.AppendToSlice(nil))
}

func TestInsertAndRemoveKeepTheOrder(t *testing.T) {
	q := wrapped()

	q.Insert(2, 99)
	assert.Equal(t, []int{13, 14, 99, 15, 16, 17, 18, 19}, q.AppendToSlice(nil))

	// Insert at the ends is the same as the corresponding push.
	q.Insert(0, 7)
	assert.Equal(t, 7, q.Front())
	q.Insert(q.Len(), 8)
	assert.Equal(t, 8, q.Back())

	// An out of range index pushes onto the nearer end rather than panicking.
	q.Insert(1000, 5)
	assert.Equal(t, 5, q.Back())
	q.Insert(-1000, 6)
	assert.Equal(t, 6, q.Front())

	// Index 3 is now the 14 the insertions pushed along.
	assert.Equal(t, 14, q.Remove(3))
	assert.Equal(t, 6, q.Remove(0))
	assert.Equal(t, 5, q.Remove(q.Len()-1))
	assert.Equal(t, []int{7, 13, 99, 15, 16, 17, 18, 19, 8}, q.AppendToSlice(nil))
}

func TestSwapExchangesTwoPositions(t *testing.T) {
	q := wrapped()

	q.Swap(0, 6)
	assert.Equal(t, 19, q.At(0))
	assert.Equal(t, 13, q.At(6))

	q.Swap(1, 1)
	assert.Equal(t, 14, q.At(1))
}

func TestRotateMovesFrontToBack(t *testing.T) {
	q := &Deque[int]{}
	q.PushBack(1)
	q.PushBack(2)
	q.PushBack(3)

	q.Rotate(1)
	assert.Equal(t, []int{2, 3, 1}, q.AppendToSlice(nil))

	// Negative rotates back to front.
	q.Rotate(-1)
	assert.Equal(t, []int{1, 2, 3}, q.AppendToSlice(nil))

	// A multiple of the length is no rotation.
	q.Rotate(3)
	assert.Equal(t, []int{1, 2, 3}, q.AppendToSlice(nil))

	// One element or none is a no-op.
	single := &Deque[int]{}
	single.PushBack(1)
	single.Rotate(5)
	assert.Equal(t, []int{1}, single.AppendToSlice(nil))
	empty := &Deque[int]{}
	empty.Rotate(5)
	assert.Zero(t, empty.Len())
}

func TestGrowGuaranteesRoom(t *testing.T) {
	q := &Deque[int]{}

	q.Grow(20)
	assert.GreaterOrEqual(t, q.Cap(), 20)
	assert.Zero(t, q.Len())

	for i := range 20 {
		q.PushBack(i)
	}
	assert.Equal(t, 20, q.Len())
	assert.Equal(t, 19, q.Back())

	// A negative count is a programming error.
	assert.Panics(t, func() { q.Grow(-1) })
}

func TestCopyAndTheSliceHelpersRoundTrip(t *testing.T) {
	src := wrapped()

	dst := &Deque[int]{}
	assert.Equal(t, src.Len(), dst.Copy(*src))
	assert.Equal(t, []int{13, 14, 15, 16, 17, 18, 19}, dst.AppendToSlice(nil))

	// Copying again replaces the contents.
	src.PopFront()
	assert.Equal(t, src.Len(), dst.Copy(*src))
	assert.Equal(t, []int{14, 15, 16, 17, 18, 19}, dst.AppendToSlice(nil))

	// AppendToSlice appends to what it is handed.
	out := []int{-1, -2}
	out = dst.AppendToSlice(out)
	assert.Equal(t, []int{-1, -2, 14, 15, 16, 17, 18, 19}, out)

	// CopyOutSlice fills what it is handed and reports how much it took.
	buf := make([]int, 3)
	assert.Equal(t, 3, dst.CopyOutSlice(buf))
	assert.Equal(t, []int{14, 15, 16}, buf)

	// CopyInSlice replaces the contents from a slice.
	dst.CopyInSlice([]int{7, 8, 9})
	assert.Equal(t, []int{7, 8, 9}, dst.AppendToSlice(nil))

	// Copying an empty slice is a clear.
	dst.CopyInSlice(nil)
	assert.Zero(t, dst.Len())
}

func TestClearKeepsTheAllocation(t *testing.T) {
	q := wrapped()
	capacity := q.Cap()

	q.Clear()
	assert.Zero(t, q.Len())
	assert.Equal(t, capacity, q.Cap())

	// The stale items must not be reachable afterwards.
	q.PushBack(42)
	assert.Equal(t, []int{42}, q.AppendToSlice(nil))
}

// A base capacity is a floor the shrink path must not cross.
func TestSetBaseCapHoldsTheFloor(t *testing.T) {
	q := &Deque[int]{}
	q.SetBaseCap(64)

	q.PushBack(1)
	q.PushBack(2)
	require.Equal(t, 2, q.Len())

	for q.Len() > 0 {
		q.PopFront()
	}
	assert.Zero(t, q.Len())
	assert.GreaterOrEqual(t, q.Cap(), 8)

	q.PushBack(3)
	assert.Equal(t, 3, q.PopFront())
}

func TestEmptyQueuePanicsRatherThanReturningAZero(t *testing.T) {
	q := &Deque[int]{}

	assert.PanicsWithValue(t, "deque: PopFront() called on empty queue", func() { q.PopFront() })
	assert.PanicsWithValue(t, "deque: PopBack() called on empty queue", func() { q.PopBack() })
}

func TestNilDequeAnswersZero(t *testing.T) {
	var q *Deque[int]

	assert.Zero(t, q.Len())
	assert.Zero(t, q.Cap())
	assert.Equal(t, -1, q.Index(func(int) bool { return true }))
	assert.Equal(t, -1, q.RIndex(func(int) bool { return true }))

	// Pushing onto a nil deque is a programming error, not a silent no-op.
	assert.Panics(t, func() { q.PushBack(1) })
}

// An iterated deque must not lose data when the caller breaks in the middle
// and then keeps using it.
func TestBreakThenKeepUsing(t *testing.T) {
	q := &Deque[int]{}
	for i := range 30 {
		q.PushBack(i)
	}

	taken := 0
	for range q.Iter() {
		taken++
		if taken == 5 {
			break
		}
	}
	assert.Equal(t, 5, taken)
	assert.Equal(t, 30, q.Len())

	for i := 30; i < 40; i++ {
		q.PushBack(i)
	}
	assert.Equal(t, 40, q.Len())
	assert.Equal(t, 39, q.Back())

	q.PopFront()
	assert.Equal(t, 39, q.Len())
	assert.Equal(t, 1, q.Front())
}

func ExampleDeque_fifo() {
	q := &Deque[int]{}
	q.PushBack(1)
	q.PushBack(2)
	q.PushBack(3)

	fmt.Println(q.PopFront(), q.PopFront(), q.PopFront(), q.Len())
	// Output: 1 2 3 0
}
