package outbound

import (
	"testing"

	N "github.com/metacubex/mihomo/common/net"
	"github.com/metacubex/mihomo/component/dialer"
	C "github.com/metacubex/mihomo/constant"
	"github.com/stretchr/testify/require"
)

func TestBaseDialOptionsAreBuiltOnceAndImmutable(t *testing.T) {
	base := NewBase(BaseOption{
		Name:        "bench",
		Type:        C.Direct,
		TFO:         true,
		Interface:   "eth0",
		RoutingMark: 128,
		Prefer:      C.IPv4Prefer,
	})

	first := base.DialOptions()
	require.Len(t, first, 4)
	require.Len(t, first, cap(first), "append must not overwrite the shared backing array")
	require.Equal(t, first, base.DialOptions())

	extended := append(first, dialer.WithResolver(nil))
	require.Len(t, extended, 5)
	require.Len(t, first, 4)
	require.Len(t, first, cap(first))
}

func TestPacketConnLocalAddrIsBuiltOnce(t *testing.T) {
	base := NewBase(BaseOption{Name: "bench", Type: C.Direct, Prefer: C.DualStack})
	pc := NewPacketConn(&nopPacketConn{}, base)

	first := pc.LocalAddr()
	require.Equal(t, first, pc.LocalAddr())
	require.Equal(t, "bench", first.Network())
	require.NotEmpty(t, first.String())
	require.Equal(t, "0.0.0.0:0", first.(N.CustomAddr).RawAddr().String())

	allocations := testing.AllocsPerRun(100, func() {
		if pc.LocalAddr() != first {
			t.Fatal("packet local address identity changed")
		}
	})
	require.Zero(t, allocations)
}
