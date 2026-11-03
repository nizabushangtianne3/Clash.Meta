package queue

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueuePutBounded(t *testing.T) {
	q := New[int](3)
	q.PutBounded(3, 1)
	q.PutBounded(3, 2)
	q.PutBounded(3, 3)
	assert.Equal(t, int64(3), q.Len())

	q.PutBounded(3, 4)
	assert.Equal(t, int64(3), q.Len())
	assert.Equal(t, []int{2, 3, 4}, q.Copy())

	q.PutBounded(0, 5)
	assert.Equal(t, int64(3), q.Len(), "a non-positive limit must reject the item")
}

func TestQueuePop(t *testing.T) {
	q := New[int](10)
	q.PutBounded(10, 1)
	q.PutBounded(10, 2)
	q.PutBounded(10, 3)

	assert.Equal(t, 1, q.Pop())
	assert.Equal(t, int64(2), q.Len())
	assert.Equal(t, 2, q.Pop())
	assert.Equal(t, int64(1), q.Len())
	assert.Equal(t, 3, q.Pop())
	assert.Equal(t, int64(0), q.Len())
}

func TestQueuePopEmpty(t *testing.T) {
	q := New[int](0)

	assert.Equal(t, 0, q.Pop())
	assert.Equal(t, int64(0), q.Len())
}

func TestQueueLast(t *testing.T) {
	q := New[int](10)
	q.PutBounded(10, 1)
	q.PutBounded(10, 2)
	q.PutBounded(10, 3)

	assert.Equal(t, 3, q.Last())
	assert.Equal(t, int64(3), q.Len())

	emptyQ := New[int](0)
	assert.Equal(t, 0, emptyQ.Last())
}

func TestQueueCopy(t *testing.T) {
	q := New[int](10)
	q.PutBounded(10, 1)
	q.PutBounded(10, 2)
	q.PutBounded(10, 3)

	copied := q.Copy()
	require.Equal(t, []int{1, 2, 3}, copied)

	copied[0] = 99
	assert.Equal(t, 1, q.Pop(), "modifying the copy must not affect the queue")
}

func TestQueueLen(t *testing.T) {
	q := New[int](10)
	assert.Equal(t, int64(0), q.Len())

	q.PutBounded(10, 1)
	q.PutBounded(10, 2)
	assert.Equal(t, int64(2), q.Len())

	q.Pop()
	assert.Equal(t, int64(1), q.Len())
}

func TestQueueRangeVisitsItemsWithoutCopying(t *testing.T) {
	q := NewWithCapacity[int](3)
	q.PutBounded(3, 1)
	q.PutBounded(3, 2)
	q.PutBounded(3, 3)
	q.PutBounded(3, 4)
	q.Pop()

	var items []int
	q.Range(func(item int) bool {
		items = append(items, item)
		return len(items) < 2
	})
	require.Equal(t, []int{3, 4}, items)
}

func TestQueueRingKeepsOrderAcrossTheArrayBoundary(t *testing.T) {
	q := NewWithCapacity[int](3)
	q.PutBounded(3, 1)
	q.PutBounded(3, 2)
	q.PutBounded(3, 3)
	assert.Equal(t, 1, q.Pop())

	q.PutBounded(3, 4)
	items := q.Copy()
	assert.Equal(t, []int{2, 3, 4}, items)
	assert.Equal(t, int64(3), q.Len())
	assert.Equal(t, 4, q.Last())
}

func TestQueueNew(t *testing.T) {
	q1 := New[int](0)
	assert.NotNil(t, q1)
	assert.Equal(t, int64(0), q1.Len())

	q2 := New[int](10)
	assert.NotNil(t, q2)
	assert.Equal(t, int64(0), q2.Len())

	q3 := New[string](5)
	assert.NotNil(t, q3)
	assert.Equal(t, int64(0), q3.Len())
}

func TestQueueConcurrency(t *testing.T) {
	q := New[int](100)
	const workers = 10
	const operations = 100

	var wg sync.WaitGroup
	wg.Add(workers * 2)
	for worker := range workers {
		go func() {
			defer wg.Done()
			for operation := range operations {
				q.PutBounded(100, worker*operations+operation+1)
				time.Sleep(time.Microsecond)
			}
		}()
	}
	for range workers {
		go func() {
			defer wg.Done()
			for range operations {
				_ = q.Pop()
				time.Sleep(time.Microsecond)
			}
		}()
	}
	wg.Wait()

	assert.LessOrEqual(t, q.Len(), int64(100))
}

func TestQueueWithDifferentTypes(t *testing.T) {
	qString := New[string](5)
	qString.PutBounded(5, "hello")
	qString.PutBounded(5, "world")
	assert.Equal(t, int64(2), qString.Len())
	assert.Equal(t, "hello", qString.Pop())
	assert.Equal(t, "world", qString.Pop())

	type person struct {
		Name string
		Age  int
	}
	qPerson := New[person](5)
	qPerson.PutBounded(5, person{Name: "Alice", Age: 30})
	qPerson.PutBounded(5, person{Name: "Bob", Age: 25})
	assert.Equal(t, int64(2), qPerson.Len())

	first := qPerson.Pop()
	assert.Equal(t, "Alice", first.Name)
	second := qPerson.Pop()
	assert.Equal(t, "Bob", second.Name)
}
