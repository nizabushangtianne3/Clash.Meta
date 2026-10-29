package outbound

import (
	"testing"

	C "github.com/metacubex/mihomo/constant"
)

func BenchmarkBaseDialOptionsEmpty(b *testing.B) {
	base := NewBase(BaseOption{Name: "bench", Type: C.Direct, Prefer: C.DualStack})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = base.DialOptions()
	}
}

func BenchmarkBaseDialOptionsFull(b *testing.B) {
	base := NewBase(BaseOption{
		Name:        "bench",
		Type:        C.Direct,
		UDP:         true,
		XUDP:        true,
		TFO:         true,
		MPTCP:       true,
		Interface:   "eth0",
		RoutingMark: 128,
		Prefer:      C.IPv4Prefer,
	})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = base.DialOptions()
	}
}
