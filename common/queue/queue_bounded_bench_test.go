package queue

import (
	"testing"
	"time"
)

type boundedItem struct {
	Time  time.Time
	Delay uint16
}

func BenchmarkQueueRecordBoundedCurrent(b *testing.B) {
	q := NewWithCapacity[boundedItem](10)
	item := boundedItem{Delay: 100}
	for range 10 {
		q.PutBounded(10, item)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.PutBounded(10, item)
		if q.Len() > 10 {
			q.Pop()
		}
	}
}
