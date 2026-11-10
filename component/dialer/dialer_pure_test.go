package dialer

import (
	"context"
	"net"
	"net/netip"
	"testing"

	"github.com/metacubex/mihomo/component/iface"
	"github.com/metacubex/mihomo/component/resolver"

	"github.com/stretchr/testify/require"
)

// Every dial goes through this fold, and a wrong family here sends the whole
// connection down the wrong stack.
func TestNormalizeDialNetwork(t *testing.T) {
	for _, tc := range []struct {
		name    string
		network string
		forced  int
		want    string
	}{
		{"tcp stays tcp", "tcp", 0, "tcp"},
		{"tcp4", "tcp", 4, "tcp4"},
		{"tcp6", "tcp", 6, "tcp6"},
		{"a forced family rewrites the network it was given", "tcp4", 6, "tcp6"},
		{"udp stays udp", "udp", 0, "udp"},
		{"udp4", "udp", 4, "udp4"},
		{"udp6", "udp", 6, "udp6"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeDialNetwork(tc.network, tc.forced)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}

	_, err := normalizeDialNetwork("sctp", 0)
	require.ErrorIs(t, err, ErrorInvalidedNetworkStack)
}

func TestListenAddressIsLoopback(t *testing.T) {
	for _, tc := range []struct {
		address string
		want    bool
	}{
		{"127.0.0.1:7890", true},
		{"[::1]:7890", true},
		{"127.0.0.1", true},
		{"0.0.0.0:7890", false},
		{"8.8.8.8:53", false},
		{"192.168.1.2:0", false},
		{"localhost:7890", false},
	} {
		t.Run(tc.address, func(t *testing.T) {
			require.Equal(t, tc.want, listenAddressIsLoopback(tc.address))
		})
	}
}

// A family-gated network answers its own literal from the parse itself and
// refuses the other family before any resolver work: a tcp4 dial to a name
// that only has AAAA answers must fail here, not after a lookup.
func TestParseAddrGatesTheLiteralByFamily(t *testing.T) {
	ips, port, err := parseAddr(context.Background(), "tcp4", "127.0.0.1:80", nil)
	require.NoError(t, err)
	require.Equal(t, []netip.Addr{netip.MustParseAddr("127.0.0.1")}, ips)
	require.Equal(t, uint16(80), port)

	_, _, err = parseAddr(context.Background(), "tcp4", "[::1]:443", nil)
	require.ErrorIs(t, err, resolver.ErrIPVersion)

	if !resolver.DisableIPv6 {
		ips, port, err = parseAddr(context.Background(), "tcp6", "[::1]:443", nil)
		require.NoError(t, err)
		require.Equal(t, []netip.Addr{netip.MustParseAddr("::1")}, ips)
		require.Equal(t, uint16(443), port)

		_, _, err = parseAddr(context.Background(), "tcp6", "127.0.0.1:80", nil)
		require.ErrorIs(t, err, resolver.ErrIPVersion)
	}

	_, _, err = parseAddr(context.Background(), "tcp4", "127.0.0.1:not-a-port", nil)
	require.ErrorContains(t, err, "invalid port")

	_, _, err = parseAddr(context.Background(), "tcp4", "127.0.0.1", nil)
	require.Error(t, err, "an address without a port has no destination")
}

// The dual stack network answers a literal of either family, which is what a
// DIRECT dial to an address does without touching a resolver.
func TestParseAddrAnswersEitherFamilyOnTheDualStack(t *testing.T) {
	ips, port, err := parseAddr(context.Background(), "tcp", "127.0.0.1:80", nil)
	require.NoError(t, err)
	require.Equal(t, []netip.Addr{netip.MustParseAddr("127.0.0.1")}, ips)
	require.Equal(t, uint16(80), port)
}

// The interface picker answers with the local address a bind-interface dial
// would use, and it is asked on every bind-interface connection.
func TestLookupLocalAddrFromIfaceName(t *testing.T) {
	ifaces, err := net.Interfaces()
	require.NoError(t, err)

	var name string
	var wantV4 netip.Addr
	for _, candidate := range ifaces {
		addrs, err := candidate.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if netipAddr, ok := netip.AddrFromSlice(addr.(*net.IPNet).IP.To4()); ok && netipAddr.IsValid() {
				name, wantV4 = candidate.Name, netipAddr.Unmap()
				break
			}
		}
		if name != "" {
			break
		}
	}
	if name == "" {
		t.Skip("this machine has no interface with an IPv4 address")
	}

	addr, err := LookupLocalAddrFromIfaceName(name, "tcp", wantV4, 12345)
	require.NoError(t, err)
	tcpAddr, ok := addr.(*net.TCPAddr)
	require.True(t, ok, "a tcp network answers a TCPAddr")
	require.Equal(t, 12345, tcpAddr.Port)
	require.Equal(t, wantV4, netip.MustParseAddr(tcpAddr.IP.String()).Unmap(), "the address the interface itself carries")

	addr, err = LookupLocalAddrFromIfaceName(name, "udp", wantV4, 12345)
	require.NoError(t, err)
	_, ok = addr.(*net.UDPAddr)
	require.True(t, ok, "a udp network answers a UDPAddr")

	_, err = LookupLocalAddrFromIfaceName("no-such-interface-on-this-host", "tcp", wantV4, 12345)
	require.ErrorIs(t, err, iface.ErrIfaceNotFound)

	_, err = LookupLocalAddrFromIfaceName(name, "sctp", wantV4, 12345)
	require.ErrorIs(t, err, iface.ErrAddrNotFound, "a network that is neither tcp nor udp has no local address shape")
}
