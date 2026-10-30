package v5

import (
	"bytes"
	"net/netip"
	"testing"

	C "github.com/metacubex/mihomo/constant"
	"github.com/stretchr/testify/require"
)

func TestAddressAddrPortRoundTrip(t *testing.T) {
	for _, addrStr := range []string{"1.2.3.4:5353", "[2001:db8::1]:5353"} {
		want := netip.MustParseAddrPort(addrStr)
		require.Equal(t, want, NewAddressAddrPort(want).AddrPort(), addrStr)
	}
}

func TestAddressAddrPortRejectsADomainAddress(t *testing.T) {
	domain := NewAddress(&C.Metadata{Host: "example.com", DstPort: 5353})
	require.False(t, domain.AddrPort().IsValid())
}

type fallbackBufferedWriter struct {
	bytes.Buffer
}

func (w *fallbackBufferedWriter) WriteByte(b byte) error {
	w.Buffer.WriteByte(b)
	return nil
}

func TestAddressConcreteAndFallbackWritersKeepWireShape(t *testing.T) {
	addresses := []Address{
		NewAddressAddrPort(netip.MustParseAddrPort("203.0.113.7:443")),
		NewAddressAddrPort(netip.MustParseAddrPort("[2001:db8::7]:443")),
		NewAddress(&C.Metadata{Host: "example.com", DstPort: 443}),
		{typ: AtypNone},
	}
	for _, addr := range addresses {
		var fast bytes.Buffer
		require.NoError(t, addr.WriteTo(&fast))

		var fallback fallbackBufferedWriter
		require.NoError(t, addr.WriteTo(&fallback))
		require.Equal(t, fast.Bytes(), fallback.Bytes())
		require.Equal(t, addr.BytesLen(), fast.Len())
	}
}
