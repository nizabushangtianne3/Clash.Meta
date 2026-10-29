package outbound

import (
	"testing"

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
