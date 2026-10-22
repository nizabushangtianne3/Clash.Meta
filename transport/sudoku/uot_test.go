package sudoku

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/metacubex/mihomo/common/pool"
	"github.com/stretchr/testify/require"
)

func TestDatagramRoundTripKeepsTheAddrPort(t *testing.T) {
	for _, target := range []netip.AddrPort{
		netip.MustParseAddrPort("8.8.8.8:53"),
		netip.MustParseAddrPort("[2001:4860:4860::8888]:443"),
	} {
		var buf bytes.Buffer
		payload := []byte{0x01, 0x02, 0x03, 0x04}
		require.NoError(t, WriteDatagram(&buf, target, payload))

		gotTarget, gotPayload, err := ReadDatagram(&buf)
		require.NoError(t, err)
		require.Equal(t, target, gotTarget)
		require.Equal(t, payload, gotPayload)
		require.Zero(t, buf.Len())
		_ = pool.Put(gotPayload)
	}
}

func TestWriteDatagramAllocatesAtMostOnce(t *testing.T) {
	target := netip.MustParseAddrPort("8.8.8.8:53")
	payload := []byte{0x01, 0x02, 0x03}
	var buf bytes.Buffer

	allocations := testing.AllocsPerRun(100, func() {
		buf.Reset()
		if err := WriteDatagram(&buf, target, payload); err != nil {
			t.Fatalf("write datagram: %v", err)
		}
	})
	require.LessOrEqual(t, allocations, 1.0)
}

func TestReadDatagramDropsADomainFieldWithoutBreakingFraming(t *testing.T) {
	target := netip.MustParseAddrPort("1.1.1.1:53")

	var buf bytes.Buffer
	domain := []byte{atypDomain, 11}
	domain = append(domain, []byte("example.com")...)
	domain = append(domain, 0, 53)
	var header [4]byte
	binary.BigEndian.PutUint16(header[:2], uint16(len(domain)))
	binary.BigEndian.PutUint16(header[2:], 1)
	buf.Write(header[:])
	buf.Write(domain)
	buf.WriteByte(0xff)
	require.NoError(t, WriteDatagram(&buf, target, []byte{0xaa}))

	_, _, err := ReadDatagram(&buf)
	require.ErrorIs(t, err, ErrAddrNotRepresentable)

	gotTarget, gotPayload, err := ReadDatagram(&buf)
	require.NoError(t, err)
	require.Equal(t, target, gotTarget)
	require.Equal(t, []byte{0xaa}, gotPayload)
	_ = pool.Put(gotPayload)
}

type discardConn struct{}

func (discardConn) Read([]byte) (int, error)         { return 0, io.EOF }
func (discardConn) Write(b []byte) (int, error)      { return len(b), nil }
func (discardConn) Close() error                     { return nil }
func (discardConn) LocalAddr() net.Addr              { return nil }
func (discardConn) RemoteAddr() net.Addr             { return nil }
func (discardConn) SetDeadline(time.Time) error      { return nil }
func (discardConn) SetReadDeadline(time.Time) error  { return nil }
func (discardConn) SetWriteDeadline(time.Time) error { return nil }

// The reply path held an AddrPort and materialised a net.UDPAddr for the
// net.PacketConn entry point on every datagram.
func BenchmarkUoTWriteBack(b *testing.B) {
	conn := NewUoTPacketConn(discardConn{})
	addr := netip.MustParseAddrPort("1.2.3.4:53")
	payload := []byte("reply")

	b.Run("addrport", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := conn.WriteToAddrPort(payload, addr); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("net-addr", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := conn.WriteTo(payload, net.UDPAddrFromAddrPort(addr)); err != nil {
				b.Fatal(err)
			}
		}
	})
}
