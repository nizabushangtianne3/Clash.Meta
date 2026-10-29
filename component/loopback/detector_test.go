package loopback

import (
	"io"
	"net"
	"net/netip"
	"time"

	"github.com/metacubex/sing/common/buf"
	"testing"

	N "github.com/metacubex/mihomo/common/net"
	C "github.com/metacubex/mihomo/constant"
)

type stubConn struct {
	N.ExtendedConn
	local net.Addr
}

func (c *stubConn) Read([]byte) (int, error)       { return 0, io.EOF }
func (c *stubConn) Write(p []byte) (int, error)    { return len(p), nil }
func (c *stubConn) ReadBuffer(*buf.Buffer) error   { return io.EOF }
func (c *stubConn) WriteBuffer(*buf.Buffer) error  { return nil }
func (c *stubConn) Close() error                   { return nil }
func (c *stubConn) LocalAddr() net.Addr            { return c.local }
func (c *stubConn) RemoteAddr() net.Addr           { return nil }
func (*stubConn) SetDeadline(time.Time) error      { return nil }
func (*stubConn) SetReadDeadline(time.Time) error  { return nil }
func (*stubConn) SetWriteDeadline(time.Time) error { return nil }
func (*stubConn) Chains() C.Chain                  { return nil }
func (*stubConn) ProviderChains() C.Chain          { return nil }
func (*stubConn) AppendToChains(C.ProxyAdapter)    {}
func (*stubConn) RemoteDestination() string        { return "" }

func newLoopbackConn() *stubConn {
	return &stubConn{local: net.UDPAddrFromAddrPort(netip.MustParseAddrPort("127.0.0.1:40001"))}
}

func TestLoopbackConnIsRemovedOnClose(t *testing.T) {
	detector := NewDetector()
	conn := detector.NewConn(newLoopbackConn())
	source := netip.MustParseAddrPort("127.0.0.1:40001")

	if _, ok := detector.connMap.Load(source); !ok {
		t.Fatal("loopback connection was not registered")
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	if _, ok := detector.connMap.Load(source); ok {
		t.Fatal("loopback connection was not removed")
	}
}

func BenchmarkLoopbackNewConn(b *testing.B) {
	detector := NewDetector()
	conn := newLoopbackConn()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := detector.NewConn(conn).Close(); err != nil {
			b.Fatal(err)
		}
	}
}
