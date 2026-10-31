package outbound

import (
	"net"
	"net/netip"
	"testing"
	"time"

	N "github.com/metacubex/mihomo/common/net"
)

type noopPacketConn struct{}

func (noopPacketConn) ReadFrom([]byte) (int, net.Addr, error) {
	return 0, nil, net.ErrClosed
}

func (noopPacketConn) WriteTo([]byte, net.Addr) (int, error) { return 0, nil }
func (noopPacketConn) Close() error                          { return nil }
func (noopPacketConn) LocalAddr() net.Addr                   { return nil }
func (noopPacketConn) SetDeadline(time.Time) error           { return nil }

func (noopPacketConn) SetReadDeadline(time.Time) error  { return nil }
func (noopPacketConn) SetWriteDeadline(time.Time) error { return nil }

func BenchmarkSSRPacketConnWriteTo(b *testing.B) {
	conn := &ssrPacketConn{
		EnhancePacketConn: N.NewEnhancePacketConn(noopPacketConn{}),
		rAddr:             net.UDPAddrFromAddrPort(netip.MustParseAddrPort("203.0.113.7:443")),
	}
	payload := make([]byte, 1200)
	addr := net.UDPAddrFromAddrPort(netip.MustParseAddrPort("192.168.1.2:54321"))

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := conn.WriteTo(payload, addr); err != nil {
			b.Fatal(err)
		}
	}
}
