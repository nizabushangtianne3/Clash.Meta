package queue

import (
	"sync"
)

// Queue is a bounded-on-demand FIFO. Once it reaches steady state, Pops reuse
// slots freed by earlier Puts instead of advancing a slice and copying the
// remaining history on later growth.
type Queue[T any] struct {
	entries []T
	head    int
	count   int
	lock    sync.RWMutex
}

// Put add the item to the queue.
func (q *Queue[T]) Put(items ...T) {
	if len(items) == 0 {
		return
	}

	q.lock.Lock()
	q.reserve(len(items))
	for _, item := range items {
		q.entries[(q.head+q.count)%cap(q.entries)] = item
		q.count++
	}
	q.lock.Unlock()
}

// Pop returns the head of items.
func (q *Queue[T]) Pop() (head T) {
	q.lock.Lock()
	defer q.lock.Unlock()
	if q.count == 0 {
		return head
	}

	head = q.entries[q.head]
	var zero T
	q.entries[q.head] = zero
	q.head = (q.head + 1) % cap(q.entries)
	q.count--
	return head
}

// Last returns the last of item.
func (q *Queue[T]) Last() (last T) {
	q.lock.RLock()
	defer q.lock.RUnlock()
	if q.count == 0 {
		return last
	}
	return q.entries[(q.head+q.count-1)%cap(q.entries)]
}

// Copy get the copy of queue.
func (q *Queue[T]) Copy() (items []T) {
	q.lock.RLock()
	items = make([]T, q.count)
	for i := range items {
		items[i] = q.entries[(q.head+i)%cap(q.entries)]
	}
	q.lock.RUnlock()
	return items
}

// Len returns the number of items in this queue.
func (q *Queue[T]) Len() int64 {
	q.lock.RLock()
	defer q.lock.RUnlock()
	return int64(q.count)
}

// New is a constructor for a new concurrent safe queue.
func New[T any](hint int64) *Queue[T] {
	return NewWithCapacity[T](hint)
}

// NewWithCapacity returns a queue whose backing ring is preallocated to hold
// hint items.
func NewWithCapacity[T any](hint int64) *Queue[T] {
	if hint < 0 {
		hint = 0
	}
	return &Queue[T]{
		entries: make([]T, hint),
	}
}

// reserve must be called while the write lock is held.
func (q *Queue[T]) reserve(additional int) {
	oldCap := cap(q.entries)
	if q.count+additional <= oldCap {
		return
	}

	newCap := oldCap
	if newCap == 0 {
		newCap = 1
	}
	for q.count+additional > newCap {
		newCap *= 2
	}
	if newCap < q.count+additional {
		newCap = q.count + additional
	}

	newEntries := make([]T, newCap)
	for i := 0; i < q.count; i++ {
		newEntries[i] = q.entries[(q.head+i)%oldCap]
	}
	q.entries = newEntries
	q.head = 0
}
