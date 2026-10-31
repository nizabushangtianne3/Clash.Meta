package statistic

import (
	"testing"

	C "github.com/metacubex/mihomo/constant"
)

func BenchmarkNewTCPTrackerChains(b *testing.B) {
	manager := newStripedManager()
	metadata := benchmarkMetadata()
	connChains := C.Chain{"Proxy", "Hysteria"}
	providerChains := C.Chain{"subscription"}
	conn := &chainBenchmarkConn{
		isolatedBenchmarkConn: isolatedBenchmarkConn{},
		chains:                connChains,
		providerChains:        providerChains,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tracker := NewTCPTracker(conn, manager, metadata, nil, 0, 0, true)
		manager.Leave(tracker)
	}
}

type chainBenchmarkConn struct {
	isolatedBenchmarkConn
	chains         C.Chain
	providerChains C.Chain
}

func (c *chainBenchmarkConn) Chains() C.Chain         { return c.chains }
func (c *chainBenchmarkConn) ProviderChains() C.Chain { return c.providerChains }
