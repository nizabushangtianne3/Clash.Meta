package sudoku

import (
	"bytes"
	"io"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/metacubex/mihomo/common/pool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A UoT frame carries its destination twice: once as a length-prefixed SOCKS5
// field and once as the payload length. Both have to survive a round trip for
// every destination the address encoder accepts.
func TestDatagramRoundTripKeepsTheDestination(t *testing.T) {
	for _, target := range []string{
		"8.8.8.8:53",
		"[2001:4860:4860::8888]:443",
		"example.com:80",
	} {
		var buf bytes.Buffer
		payload := []byte{0x01, 0x02, 0x03, 0x04}
		require.NoError(t, WriteDatagram(&buf, target, payload), target)

		reader := NewUoTReader(&buf)
		addr, gotPayload, err := reader.ReadDatagram()
		require.NoError(t, err, target)
		assert.Equal(t, target, addr.String(), target)
		assert.Equal(t, payload, gotPayload, target)
		_ = pool.Put(gotPayload)
	}
}

// The frame is built in a pooled buffer and sealed as one write, so a warm
// path allocates nothing beyond what the address encoder itself needs.
func TestWriteDatagramAllocatesAtMostOnce(t *testing.T) {
	var buf bytes.Buffer
	payload := []byte{0x01, 0x02, 0x03}

	allocations := testing.AllocsPerRun(200, func() {
		buf.Reset()
		require.NoError(t, WriteDatagram(&buf, "8.8.8.8:53", payload))
	})
	require.LessOrEqual(t, allocations, 1.0)
}

// A domain address is longer than an IP one, so the reader's scratch has to
// cover it; the frame after it must still be found on the stream boundary.
func TestReadDatagramKeepsFramingAcrossADomainField(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, WriteDatagram(&buf, "a.very.long.domain.name.example.com:80", []byte{0xaa}))
	require.NoError(t, WriteDatagram(&buf, "1.1.1.1:53", []byte{0xbb, 0xcc}))

	reader := NewUoTReader(&buf)

	addr, payload, err := reader.ReadDatagram()
	require.NoError(t, err)
	require.Equal(t, "a.very.long.domain.name.example.com:80", addr.String())
	require.Equal(t, []byte{0xaa}, payload)
	_ = pool.Put(payload)

	addr, payload, err = reader.ReadDatagram()
	require.NoError(t, err)
	require.Equal(t, "1.1.1.1:53", addr.String())
	require.Equal(t, []byte{0xbb, 0xcc}, payload)
	_ = pool.Put(payload)
}

type discardConn struct{}

func (discardConn) Read([]byte) (int, error)        { return 0, io.EOF }
func (discardConn) Write(b []byte) (int, error)     { return len(b), nil }
func (discardConn) Close() error                    { return nil }
func (discardConn) LocalAddr() net.Addr             { return nil }
func (discardConn) RemoteAddr() net.Addr            { return nil }
func (discardConn) SetDeadline(time.Time) error     { return nil }
func (discardConn) SetReadDeadline(time.Time) error { return nil }
func (discardConn) SetWriteDeadline(time.Time) error {
	return nil
}

// The reply path used to materialise a net.UDPAddr for the net.PacketConn entry
// point on every datagram; the AddrPort entry point takes the address the
// tunnel already holds.
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
