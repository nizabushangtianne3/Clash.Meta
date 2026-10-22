package gost

import (
	"bytes"
	"net"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeRelayConn struct{ net.Conn }

func (fakeRelayConn) Write(b []byte) (int, error) { return len(b), nil }

type fakeRelayReadConn struct {
	net.Conn
	r *bytes.Reader
}

func (c fakeRelayReadConn) Read(b []byte) (int, error) { return c.r.Read(b) }

func TestRelayPacketConnWriteToAssociationGuard(t *testing.T) {
	bound := net.UDPAddrFromAddrPort(netip.MustParseAddrPort("203.0.113.7:443"))
	conn := &relayPacketConn{conn: fakeRelayConn{}, raddr: bound}

	_, err := conn.WriteTo([]byte("ping"), bound)
	require.NoError(t, err)

	otherPort := net.UDPAddrFromAddrPort(netip.MustParseAddrPort("203.0.113.7:80"))
	_, err = conn.WriteTo([]byte("ping"), otherPort)
	require.Error(t, err)

	otherHost := net.UDPAddrFromAddrPort(netip.MustParseAddrPort("203.0.113.8:443"))
	_, err = conn.WriteTo([]byte("ping"), otherHost)
	require.Error(t, err)

	// the same destination given as a 16-byte v4-in-v6 still matches
	mapped := &net.UDPAddr{IP: net.IP(netip.MustParseAddr("::ffff:203.0.113.7").AsSlice()), Port: 443}
	_, err = conn.WriteTo([]byte("ping"), mapped)
	require.NoError(t, err)
}

func TestRelayPacketConnReadFromShortBufferDropsTheRest(t *testing.T) {
	raddr := net.UDPAddrFromAddrPort(netip.MustParseAddrPort("203.0.113.7:443"))
	raw := []byte{0x00, 0x04, 0xaa, 0xbb, 0xcc, 0xdd}
	conn := &relayPacketConn{
		conn:  fakeRelayReadConn{r: bytes.NewReader(raw)},
		raddr: raddr,
	}

	buf := make([]byte, 2)
	n, gotAddr, err := conn.ReadFrom(buf)
	require.NoError(t, err)
	require.Equal(t, 2, n)
	require.Equal(t, raddr, gotAddr)
	require.Equal(t, []byte{0xaa, 0xbb}, buf)
}
