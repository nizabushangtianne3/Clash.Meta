package sing

import (
	"context"
	"net"
	"net/netip"
	"testing"
	"time"

	C "github.com/metacubex/mihomo/constant"

	M "github.com/metacubex/sing/common/metadata"
)

type acceptTunnel struct {
	C.Tunnel
}

func (acceptTunnel) HandleTCPConn(conn net.Conn, _ *C.Metadata) {
	_ = conn.Close()
}

type fakeTCPConn struct{}

var benchAddr = &net.TCPAddr{IP: net.IPv4(10, 0, 0, 1), Port: 7890}

func (fakeTCPConn) Read([]byte) (int, error)         { return 0, nil }
func (fakeTCPConn) Write(b []byte) (int, error)      { return len(b), nil }
func (fakeTCPConn) Close() error                     { return nil }
func (fakeTCPConn) LocalAddr() net.Addr              { return benchAddr }
func (fakeTCPConn) RemoteAddr() net.Addr             { return &net.TCPAddr{IP: net.IPv4(10, 0, 0, 1), Port: 7891} }
func (fakeTCPConn) SetDeadline(time.Time) error      { return nil }
func (fakeTCPConn) SetReadDeadline(time.Time) error  { return nil }
func (fakeTCPConn) SetWriteDeadline(time.Time) error { return nil }

// One run accepts one TCP connection, which is what every connection costs
// before any proxy work starts.
func BenchmarkNewConnectionTCPHandoff(b *testing.B) {
	handler := &ListenerHandler{
		ListenerConfig: ListenerConfig{Tunnel: acceptTunnel{}, Type: C.TUN},
	}
	metadata := M.Metadata{
		Source:      M.SocksaddrFrom(netip.MustParseAddr("192.168.1.2"), 54321),
		Destination: M.SocksaddrFrom(netip.MustParseAddr("1.2.3.4"), 443),
	}
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = handler.NewConnection(ctx, fakeTCPConn{}, metadata)
	}
}
