package sudoku

import (
	"bytes"
	"errors"
	"io"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/metacubex/mihomo/common/net/packet"
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

// BenchmarkUoTFrameRoundTrip measures one datagram's whole wire step: the
// frame is encoded by the writer and read back by the session's reader.
type uotFrameConn struct {
	r *bytes.Reader
}

func (c *uotFrameConn) Read(p []byte) (int, error) { return c.r.Read(p) }
func (c *uotFrameConn) Write(p []byte) (int, error) {
	return 0, errors.New("write is unused")
}
func (c *uotFrameConn) Close() error                    { return nil }
func (c *uotFrameConn) LocalAddr() net.Addr             { return nil }
func (c *uotFrameConn) RemoteAddr() net.Addr            { return nil }
func (c *uotFrameConn) SetDeadline(time.Time) error     { return nil }
func (c *uotFrameConn) SetReadDeadline(time.Time) error { return nil }
func (c *uotFrameConn) SetWriteDeadline(time.Time) error {
	return nil
}
func (c *uotFrameConn) ReadFrom(p []byte) (int, net.Addr, error) {
	n, addr, err := NewUoTPacketConn(c).ReadFrom(p)
	return n, addr, err
}
func (c *uotFrameConn) WriteTo(p []byte, _ net.Addr) (int, error) {
	return c.Write(p)
}

type uotReadConn struct {
	r *bytes.Reader
}

func (c *uotReadConn) Read(p []byte) (int, error) { return c.r.Read(p) }
func (*uotReadConn) Write(p []byte) (int, error) {
	return 0, errors.New("write is unused")
}
func (*uotReadConn) Close() error                     { return nil }
func (*uotReadConn) LocalAddr() net.Addr              { return nil }
func (*uotReadConn) RemoteAddr() net.Addr             { return nil }
func (*uotReadConn) SetDeadline(time.Time) error      { return nil }
func (*uotReadConn) SetReadDeadline(time.Time) error  { return nil }
func (*uotReadConn) SetWriteDeadline(time.Time) error { return nil }

func TestUoTWaitReadFromSkipsDomainsAndUnmapsIPv4(t *testing.T) {
	var frames bytes.Buffer
	require.NoError(t, WriteDatagram(&frames, "example.com:53", []byte("domain")))
	require.NoError(t, WriteDatagram(&frames, "[::ffff:1.2.3.4]:5353", []byte("okay")))

	reader := bytes.NewReader(frames.Bytes())
	pc := NewUoTPacketConn(&uotReadConn{r: reader})

	data, addr, err := pc.WaitReadFrom()
	require.NoError(t, err)
	require.Equal(t, []byte("okay"), data)
	require.Equal(t, "1.2.3.4:5353", addr.String())
	require.True(t, addr.Addr().Is4())
	pool.Put(data)

	_, _, err = pc.WaitReadFrom()
	require.ErrorIs(t, err, io.EOF)
}

func BenchmarkUoTWaitReadFrom(b *testing.B) {
	const payloadByte = 0xaa
	var frames bytes.Buffer
	for i := 0; i < 128; i++ {
		if err := WriteDatagram(&frames, "8.8.8.8:53", []byte{payloadByte, payloadByte, payloadByte, payloadByte}); err != nil {
			b.Fatal(err)
		}
	}
	raw := frames.Bytes()
	reader := bytes.NewReader(raw)
	wrapper := packet.NewEnhancePacketConn(NewUoTPacketConn(&uotFrameConn{r: reader}))

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader.Reset(raw)
		for consumed := 0; consumed < 128; {
			data, _, err := wrapper.WaitReadFrom()
			if err != nil {
				b.Fatal(err)
			}
			if len(data) != 4 || data[0] != payloadByte {
				b.Fatalf("bad datagram: %d bytes", len(data))
			}
			pool.Put(data)
			consumed++
		}
	}
}
func BenchmarkUoTFrameRoundTrip(b *testing.B) {
	target := "1.2.3.4:53"
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
