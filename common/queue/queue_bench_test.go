package queue

import "testing"

// BenchmarkQueueSaturatedPutPop exercises the steady state used by bounded
// delay history: the queue stays full while every Put follows one Pop.
func BenchmarkQueueSaturatedPutPop(b *testing.B) {
	q := NewWithCapacity[int](10)
	for i := 0; i < 10; i++ {
		q.Put(i)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Put(i)
		q.Pop()
	}
}
