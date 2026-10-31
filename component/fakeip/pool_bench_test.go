package fakeip

import (
	"fmt"
	"net/netip"
	"sync/atomic"
	"testing"
)

func benchmarkPool(size int) *Pool {
	pool, err := New(Options{IPNet: netip.MustParsePrefix("198.18.0.0/15"), Size: size})
	if err != nil {
		panic(err)
	}
	return pool
}

func benchmarkNames(count int) []string {
	names := make([]string, count)
	for index := range names {
		names[index] = fmt.Sprintf("lookup-%07d.example.org", index)
	}
	return names
}

func BenchmarkPoolLookupUnique(b *testing.B) {
	pool := benchmarkPool(0)
	names := benchmarkNames(128 * 1024)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pool.Lookup(names[i%len(names)])
	}
}

func BenchmarkPoolLookupUniqueParallel(b *testing.B) {
	pool := benchmarkPool(0)
	names := benchmarkNames(256 * 1024)
	var cursor atomic.Int64

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			index := int(cursor.Add(1)) % len(names)
			pool.Lookup(names[index])
		}
	})
}

func BenchmarkPoolLookupWarmParallel(b *testing.B) {
	pool := benchmarkPool(0)
	names := benchmarkNames(4096)
	for _, name := range names {
		pool.Lookup(name)
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			pool.Lookup(names[int(lookupCursor.Add(1))%len(names)])
		}
	})
}

var lookupCursor atomic.Int64
