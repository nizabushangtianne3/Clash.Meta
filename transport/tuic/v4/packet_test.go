package v4

import (
	"bufio"
	"bytes"
	"net"
	"net/netip"
	"testing"

	N "github.com/metacubex/mihomo/common/net"
	"github.com/stretchr/testify/require"
)

type readFromConn struct {
	net.Conn
	r *bytes.Reader
}

func (c *readFromConn) Read(p []byte) (int, error) { return c.r.Read(p) }

func TestReadFromReturnsIndependentAddresses(t *testing.T) {
	first := NewPacket(1, 4,
		NewAddressAddrPort(netip.MustParseAddrPort("192.0.2.1:443")),
		[]byte{0xaa, 0xbb, 0xcc, 0xdd})
	second := NewPacket(1, 4,
		NewAddressAddrPort(netip.MustParseAddrPort("198.51.100.1:443")),
		[]byte{0xdd, 0xcc, 0xbb, 0xaa})
	raw := append(encodePacket(t, first), encodePacket(t, second)...)
	conn := N.NewBufferedConn(&readFromConn{r: bytes.NewReader(raw)})
	q := &quicStreamPacketConn{inputConn: conn}

	_, firstAddr, err := q.ReadFrom(make([]byte, 4))
	require.NoError(t, err)
	_, _, err = q.ReadFrom(make([]byte, 4))
	require.NoError(t, err)
	require.True(t, firstAddr.(*net.UDPAddr).IP.Equal(net.ParseIP("192.0.2.1")))
}

func encodePacket(t *testing.T, packet Packet) []byte {
	t.Helper()
	var buffer bytes.Buffer
	require.NoError(t, packet.WriteTo(&buffer))
	return buffer.Bytes()
}

func TestReadPacketIntoKeepsTheAddrPort(t *testing.T) {
	for _, target := range []netip.AddrPort{
		netip.MustParseAddrPort("203.0.113.7:443"),
		netip.MustParseAddrPort("[2001:db8::7]:443"),
	} {
		packet := NewPacket(1, 4, NewAddressAddrPort(target), []byte{0xaa, 0xbb, 0xcc, 0xdd})
		raw := encodePacket(t, packet)
		reader := bufio.NewReaderSize(bytes.NewReader(raw), 4096)

		got, err := readPacketInto(reader)
		require.NoError(t, err)
		require.Equal(t, target, got.ADDR.AddrPort())
		require.Equal(t, packet.DATA, got.DATA)
	}
}
