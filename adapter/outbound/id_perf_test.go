package outbound

import (
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/metacubex/mihomo/common/utils"
)

var idSink string

// PacketConn IDs only need to distinguish addresses inside this process; keep
// a CSPRNG UUID out of every UDP flow setup.
func BenchmarkPacketConnIDGeneration(b *testing.B) {
	b.Run("counter", func(b *testing.B) {
		var counter atomic.Uint64
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			idSink = strconv.FormatUint(counter.Add(1), 10)
		}
	})
	b.Run("uuid", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			idSink = utils.NewUUIDV4().String()
		}
	})
}
