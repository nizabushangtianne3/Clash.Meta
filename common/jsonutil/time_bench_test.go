package jsonutil

import (
	"testing"
	"time"
)

func BenchmarkAppendTime(b *testing.B) {
	zero := time.Time{}
	now := time.Date(2026, 11, 4, 5, 6, 7, 123456789, time.UTC)
	dst := make([]byte, 0, 64)

	b.Run("zero", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			dst = AppendTime(dst[:0], zero)
		}
	})
	b.Run("value", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			dst = AppendTime(dst[:0], now)
		}
	})
}
