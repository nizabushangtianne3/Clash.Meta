package packet

import (
	"errors"
	"net"
	"testing"
	"time"

	"github.com/metacubex/mihomo/common/pool"
	"github.com/stretchr/testify/require"
)

func newUDPTestPair(tb testing.TB, network string, ip net.IP) (*net.UDPConn, EnhancePacketConn) {
	tb.Helper()

	server, err := net.ListenUDP(network, &net.UDPAddr{IP: ip})
	if err != nil && network == "udp6" {
		tb.Skipf("no loopback for %s: %v", network, err)
	}
	require.NoError(tb, err)
	client, err := net.DialUDP(network, nil, server.LocalAddr().(*net.UDPAddr))
	require.NoError(tb, err)
	tb.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
	})

	return client, NewEnhancePacketConn(server)
}

func TestWaitReadFromReturnsAddrPort(t *testing.T) {
	for _, family := range []struct {
		network string
		ip      net.IP
	}{
		{network: "udp4", ip: net.IPv4(127, 0, 0, 1)},
		{network: "udp6", ip: net.IPv6loopback},
	} {
		t.Run(family.network, func(t *testing.T) {
			client, conn := newUDPTestPair(t, family.network, family.ip)
			payload := []byte("ping")

			_, err := client.Write(payload)
			require.NoError(t, err)
			data, addr, err := conn.WaitReadFrom()
			require.NoError(t, err)
			defer pool.Put(data)

			require.Equal(t, payload, data)
			require.Equal(t, client.LocalAddr().(*net.UDPAddr).AddrPort(), addr)
		})
	}
}

func TestWaitReadFromReturnsIPv6AddrPort(t *testing.T) {
	server, err := net.ListenUDP("udp6", &net.UDPAddr{IP: net.IPv6loopback})
	require.NoError(t, err)
	client, err := net.DialUDP("udp6", nil, server.LocalAddr().(*net.UDPAddr))
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
	})

	conn := NewEnhancePacketConn(server)
	payload := []byte("ping")
	_, err = client.Write(payload)
	require.NoError(t, err)
	data, addr, err := conn.WaitReadFrom()
	require.NoError(t, err)
	defer pool.Put(data)

	require.Equal(t, payload, data)
	require.Equal(t, client.LocalAddr().(*net.UDPAddr).AddrPort(), addr)
}

func TestWaitReadFromHandlesZeroLengthDatagram(t *testing.T) {
	client, conn := newUDPTestPair(t, "udp4", net.IPv4(127, 0, 0, 1))
	writeZeroLengthDatagram(t, client)
	data, addr, err := conn.WaitReadFrom()
	require.NoError(t, err)
	require.Nil(t, data)
	require.Equal(t, client.LocalAddr().(*net.UDPAddr).AddrPort(), addr)
}

func BenchmarkWaitReadFrom(b *testing.B) {
	for _, size := range []int{64, 1500} {
		b.Run(testPayloadName(size), func(b *testing.B) {
			client, conn := newUDPTestPair(b, "udp4", net.IPv4(127, 0, 0, 1))
			payload := make([]byte, size)

			b.ReportAllocs()
			b.SetBytes(int64(size))
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				if _, err := client.Write(payload); err != nil {
					b.Fatal(err)
				}
				data, _, err := conn.WaitReadFrom()
				if err != nil {
					b.Fatal(err)
				}
				pool.Put(data)
			}
		})
	}
}

func testPayloadName(size int) string {
	if size == 1500 {
		return "1500B"
	}
	return "small"
}

type bufferReadConn struct {
	closed   bool
	payload  []byte
	read     bool
	err      error
	peerAddr net.Addr
}

func (c *bufferReadConn) Close() error {
	c.closed = true
	return nil
}

func (c *bufferReadConn) LocalAddr() net.Addr {
	return &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 53}
}

func (c *bufferReadConn) ReadFrom(p []byte) (int, net.Addr, error) {
	return 0, nil, errors.New("ReadFrom is unused")
}

func (c *bufferReadConn) SetDeadline(time.Time) error      { return nil }
func (c *bufferReadConn) SetReadDeadline(time.Time) error  { return nil }
func (c *bufferReadConn) SetWriteDeadline(time.Time) error { return nil }

func (c *bufferReadConn) WriteTo(p []byte, _ net.Addr) (int, error) {
	return len(p), nil
}

func (c *bufferReadConn) ReadFromWithBuffer(getBuffer func(sizeHint int) []byte) (int, net.Addr, error) {
	if c.closed {
		return 0, nil, errors.New("closed")
	}
	if c.read {
		return 0, nil, errors.New("already read")
	}
	c.read = true
	buf := getBuffer(len(c.payload))
	copy(buf, c.payload)
	return len(c.payload), c.peerAddr, nil
}

func TestWaitReadFromWithBufferReturnsPayload(t *testing.T) {
	payload := []byte("with-buffer")
	conn := newEnhanceWithBufferPacketConn(&bufferReadConn{
		payload:  payload,
		peerAddr: &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 53},
	})

	data, addr, err := conn.WaitReadFrom()
	require.NoError(t, err)
	defer pool.Put(data)
	require.Equal(t, payload, data)
	require.True(t, addr.Addr().Is4In6())
	require.EqualValues(t, 53, addr.Port())
}

func BenchmarkWaitReadFromWithBuffer(b *testing.B) {
	payload := make([]byte, 1500)
	conn := newEnhanceWithBufferPacketConn(&bufferReadConn{
		payload:  payload,
		peerAddr: &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 53},
	})
	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		data, _, err := conn.WaitReadFrom()
		if err != nil {
			b.Fatal(err)
		}
		pool.Put(data)
		conn.withBufferPacketConn.(*bufferReadConn).read = false
	}
}
