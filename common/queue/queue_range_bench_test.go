package queue

import (
	"testing"
	"time"
)

type rangeItem struct {
	Time  time.Time
	Delay uint16
}

func BenchmarkQueueRangeTen(b *testing.B) {
	q := NewWithCapacity[rangeItem](10)
	item := rangeItem{Delay: 100}
	for range 10 {
		q.PutBounded(10, item)
	}
	visited := 0
	callback := func(rangeItem) bool {
		visited++
		return visited < 10
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		visited = 0
		q.Range(callback)
		if visited != 10 {
			b.Fatal("short range")
		}
	}
}
