package shadowquic

import (
	"net"
	"net/netip"
	"testing"

	"github.com/metacubex/mihomo/common/pool"
	"github.com/metacubex/mihomo/transport/socks5"
	"github.com/stretchr/testify/require"
)

func TestAddrPortFromNetAddrAllocatesNothing(t *testing.T) {
	addr := net.UDPAddrFromAddrPort(netip.MustParseAddrPort("1.2.3.4:53"))

	allocations := testing.AllocsPerRun(100, func() {
		if _, err := AddrPortFromNetAddr(addr); err != nil {
			t.Fatalf("convert: %v", err)
		}
	})
	require.Zero(t, allocations)
}

type badNetAddr struct{}

func (badNetAddr) Network() string { return "bad" }
func (badNetAddr) String() string  { return "not-an-address" }
func TestAssociationWaitReadFromReturnsPooledPacket(t *testing.T) {
	ap := netip.MustParseAddrPort("203.0.113.7:443")
	a := &association{input: make(chan receivedPacket, 1)}
	payload := pool.Get(1200)
	copy(payload, []byte("packet"))
	payload = payload[:6]
	a.input <- receivedPacket{
		data:      payload,
		addr:      socksNetAddr{addr: socks5.AddrFromStdAddrPort(ap)},
		socksAddr: socks5.AddrFromStdAddrPort(ap),
	}

	data, addr, err := a.WaitReadFrom()
	require.NoError(t, err)
	require.Equal(t, []byte("packet"), data)
	require.Equal(t, ap, addr)
	pool.Put(data)
}

func TestAddrPortFromNetAddrRejectsANonAddress(t *testing.T) {
	_, err := AddrPortFromNetAddr(badNetAddr{})
	require.ErrorIs(t, err, errInvalidAddress)
}

// The reply path keyed its send maps by the formatted socks address, which cost
// a net.UDPAddr, its IP slice, the socks address and the key string on every
// datagram. The destination AddrPort is the key now.
func BenchmarkWriteBackDestinationKey(b *testing.B) {
	addrPort := netip.MustParseAddrPort("1.2.3.4:53")

	b.Run("addrport", func(b *testing.B) {
		sendIDs := map[netip.AddrPort]uint16{addrPort: 7}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, ok := sendIDs[addrPort]; !ok {
				b.Fatal("miss")
			}
		}
	})

	b.Run("socks-string", func(b *testing.B) {
		sendIDs := map[string]uint16{string(socks5.ParseAddrToSocksAddr(net.UDPAddrFromAddrPort(addrPort))): 7}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, ok := sendIDs[string(socks5.ParseAddrToSocksAddr(net.UDPAddrFromAddrPort(addrPort)))]; !ok {
				b.Fatal("miss")
			}
		}
	})
}
