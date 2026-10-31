package statistic

import (
	"net"
	"testing"

	C "github.com/metacubex/mihomo/constant"
)

var benchmarkIsolatedChain = C.Chain{"DIRECT"}

var benchmarkIsolatedConn = &isolatedBenchmarkConn{}

// isolatedBenchmarkConn removes net.Pipe from the creation benchmark; the pipe
// otherwise dominates allocations before the tracker is even constructed.
type isolatedBenchmarkConn struct{ C.Conn }

func (c *isolatedBenchmarkConn) Chains() C.Chain               { return benchmarkIsolatedChain }
func (c *isolatedBenchmarkConn) ProviderChains() C.Chain       { return nil }
func (c *isolatedBenchmarkConn) AppendToChains(C.ProxyAdapter) {}
func (c *isolatedBenchmarkConn) RemoteDestination() string     { return "" }

func BenchmarkNewTCPTrackerIsolated(b *testing.B) {
	manager := newStripedManager()
	metadata := benchmarkMetadata()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tracker := NewTCPTracker(benchmarkIsolatedConn, manager, metadata, nil, 0, 0, true)
		manager.Leave(tracker)
	}
}

func BenchmarkNewTCPTracker(b *testing.B) {
	manager := newStripedManager()
	local, remote := net.Pipe()
	b.Cleanup(func() { _ = remote.Close() })

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tracker := NewTCPTracker(newStatisticTestConn(local), manager, benchmarkMetadata(), nil, 0, 0, true)
		manager.Leave(tracker)
	}
}

func BenchmarkTCPTrackerUnwrap(b *testing.B) {
	manager := newStripedManager()
	local, remote := net.Pipe()
	b.Cleanup(func() { _ = remote.Close() })
	tracker := NewTCPTracker(newStatisticTestConn(local), manager, benchmarkMetadata(), nil, 0, 0, true)
	b.Cleanup(func() { _ = tracker.Close() })

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tracker.UnwrapReader()
		tracker.UnwrapWriter()
	}
}
