//go:build !no_zerotier

package outbound

import (
	"fmt"
	"testing"

	"github.com/metacubex/mihomo/log"
	ZTTransport "github.com/metacubex/zerotier-go/transport"
	"github.com/stretchr/testify/require"
)

// The library formats its log lines through this callback, which used to build
// the message and call the logger before anything checked the level. The guard
// has to return before it names the proxy: a zero value has no Base, so reaching
// z.Name() would panic.
func TestZeroTierTransportLogCostsNothingWhenTheLevelIsOff(t *testing.T) {
	previous := log.Level()
	log.SetLevel(log.SILENT)
	t.Cleanup(func() { log.SetLevel(previous) })

	proxy := &ZeroTier{}
	allocations := testing.AllocsPerRun(100, func() {
		proxy.logFromTransport(ZTTransport.LogDebug, "peer %s selected route %d", "203.0.113.7", 3)
		proxy.logFromTransport(ZTTransport.LogInfo, "node %s is offline", "abcdef")
	})
	require.Zero(t, allocations)
}

func BenchmarkZeroTierTransportLog(b *testing.B) {
	previous := log.Level()
	log.SetLevel(log.SILENT)
	b.Cleanup(func() { log.SetLevel(previous) })

	proxy := &ZeroTier{}

	b.Run("guarded", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			proxy.logFromTransport(ZTTransport.LogDebug, "peer %s selected route %d", "203.0.113.7", 3)
		}
	})

	b.Run("unguarded", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			message := fmt.Sprintf("peer %s selected route %d", "203.0.113.7", 3)
			log.Debugln("[ZeroTier](%s) %s", "zt", message)
		}
	})
}
