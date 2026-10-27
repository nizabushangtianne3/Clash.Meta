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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testMaxUoTPayload    = 1<<16 - 1
	testMaxUoTAddressLen = 1 + 1 + 255
)

// A UoT frame carries its destination twice: once as a length-prefixed address
// field and once as the payload length. Both have to survive a round trip for
// every destination the relay accepts, which is IP only because the client
// resolves the destination before it writes.
func TestDatagramRoundTripKeepsTheDestination(t *testing.T) {
	for _, target := range []netip.AddrPort{
		netip.MustParseAddrPort("8.8.8.8:53"),
		netip.MustParseAddrPort("[2001:4860:4860::8888]:443"),
	} {
		var buf bytes.Buffer
		payload := []byte{0x01, 0x02, 0x03, 0x04}
		require.NoError(t, WriteDatagram(&buf, target, payload))

		reader := NewUoTReader(&buf)
		addr, gotPayload, err := reader.ReadDatagram()
		require.NoError(t, err, target)
		assert.Equal(t, target.String(), addr.String(), target)
		assert.Equal(t, payload, gotPayload, target)
		require.Zero(t, buf.Len(), target)
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
		require.NoError(t, WriteDatagram(&buf, netip.MustParseAddrPort("8.8.8.8:53"), payload))
	})
	require.LessOrEqual(t, allocations, 1.0)
}

func TestWriteDatagramAtThePayloadLimit(t *testing.T) {
	payload := pool.Get(testMaxUoTPayload)
	payload[testMaxUoTPayload-1] = 0x7f

	var buf bytes.Buffer
	require.NoError(t, WriteDatagram(&buf, netip.MustParseAddrPort("8.8.8.8:53"), payload))

	reader := NewUoTReader(&buf)
	gotTarget, gotPayload, err := reader.ReadDatagram()
	require.NoError(t, err)
	require.Equal(t, "8.8.8.8:53", gotTarget.String())
	require.Equal(t, payload, gotPayload)
	_ = pool.Put(payload)
	_ = pool.Put(gotPayload)
}

func TestWriteDatagramRejectsPayloadAboveTheWireLimit(t *testing.T) {
	payload := make([]byte, testMaxUoTPayload+1)

	require.Error(t, WriteDatagram(&bytes.Buffer{}, netip.MustParseAddrPort("8.8.8.8:53"), payload))
}

// An address field wider than the widest encoded IP:port cannot name a UDP
// flow; the reader rejects the frame instead of trusting the length.
func TestReadDatagramRejectsOversizedAddressField(t *testing.T) {
	var header [4]byte
	binary.BigEndian.PutUint16(header[:2], testMaxUoTAddressLen+1)

	reader := NewUoTReader(bytes.NewReader(header[:]))
	_, _, err := reader.ReadDatagram()
	require.Error(t, err)
}

// A domain address field is dropped whole, together with its payload, and the
// frame after it stays on the stream boundary.
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

	reader := NewUoTReader(&buf)
	_, _, err := reader.ReadDatagram()
	require.ErrorIs(t, err, ErrAddrNotRepresentable)

	gotTarget, gotPayload, err := reader.ReadDatagram()
	require.NoError(t, err)
	require.Equal(t, target.String(), gotTarget.String())
	require.Equal(t, []byte{0xaa}, gotPayload)
	require.Zero(t, buf.Len())
	_ = pool.Put(gotPayload)
}

type discardConn struct{}

func (discardConn) Read([]byte) (int, error)    { return 0, io.EOF }
func (discardConn) Write(b []byte) (int, error) { return len(b), nil }
func (discardConn) Close() error                { return nil }
func (discardConn) LocalAddr() net.Addr         { return nil }
func (discardConn) RemoteAddr() net.Addr        { return nil }
func (discardConn) SetDeadline(time.Time) error { return nil }
func (discardConn) SetReadDeadline(time.Time) error {
	return nil
}
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

// BenchmarkUoTFrameRoundTrip measures one datagram's whole wire step: the
// frame is encoded by the writer and read back by the session's reader.
func BenchmarkUoTFrameRoundTrip(b *testing.B) {
	target := netip.MustParseAddrPort("1.2.3.4:53")
	payload := []byte{1, 2, 3, 4, 5, 6, 7, 8}

	b.Run("write", func(b *testing.B) {
		var frame bytes.Buffer
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			frame.Reset()
			if err := WriteDatagram(&frame, target, payload); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("read", func(b *testing.B) {
		const framesPerBatch = 512

		fill := func(frames *bytes.Buffer) *UoTReader {
			frames.Reset()
			for i := 0; i < framesPerBatch; i++ {
				if err := WriteDatagram(frames, target, payload); err != nil {
					b.Fatal(err)
				}
			}
			return NewUoTReader(frames)
		}

		var frames bytes.Buffer
		reader := fill(&frames)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, got, err := reader.ReadDatagram()
			if err != nil {
				reader = fill(&frames)
				_, got, err = reader.ReadDatagram()
			}
			if err != nil {
				b.Fatal(err)
			}
			_ = pool.Put(got)
		}
	})
}
