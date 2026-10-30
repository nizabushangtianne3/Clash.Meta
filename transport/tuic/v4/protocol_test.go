package v4

import (
	"bytes"
	"net/netip"
	"testing"

	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/transport/socks5"
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
func TestAddressKeepsDomainWireShape(t *testing.T) {
	addr := NewAddress(&C.Metadata{Host: "example.com", DstPort: 443})
	require.Equal(t, 15, addr.BytesLen())

	var wire bytes.Buffer
	require.NoError(t, addr.WriteTo(&wire))
	require.Equal(t, append([]byte{AtypDomainName, 11}, append([]byte("example.com"), 0x01, 0xbb)...), wire.Bytes())

	got, err := ReadAddress(&wire)
	require.NoError(t, err)
	require.Equal(t, addr, got)
	require.Equal(t, "example.com:443", got.String())
	require.Equal(t, socks5.Addr([]byte{socks5.AtypDomainName, 11, 'e', 'x', 'a', 'm', 'p', 'l', 'e', '.', 'c', 'o', 'm', 0x01, 0xbb}), got.SocksAddr())
}

func TestReadAddressRejectsUnknownType(t *testing.T) {
	_, err := ReadAddress(bytes.NewBuffer([]byte{0xfe, 0x00, 0x00}))
	require.Error(t, err)
}
