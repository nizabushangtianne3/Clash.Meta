package queue

import (
	"testing"
	"time"
)

type copyItem struct {
	Time  time.Time
	Delay uint16
}

func BenchmarkQueueCopyTen(b *testing.B) {
	q := NewWithCapacity[copyItem](10)
	item := copyItem{Delay: 100}
	for range 10 {
		q.PutBounded(10, item)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if copied := q.Copy(); len(copied) != 10 {
			b.Fatal("short copy")
		}
	}
}
