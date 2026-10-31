package loopback

import (
	"fmt"
	"io"
	"net"
	"net/netip"
	"sync"
	"testing"
	"time"

	N "github.com/metacubex/mihomo/common/net"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/sing/common/buf"
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
	return newLoopbackConnPort(40001)
}

func newLoopbackConnPort(port uint16) *stubConn {
	source := netip.AddrPortFrom(netip.AddrFrom4([4]byte{127, 0, 0, 1}), port)
	return &stubConn{local: net.UDPAddrFromAddrPort(source)}
}

func TestLoopbackConnIsRemovedOnClose(t *testing.T) {
	detector := NewDetector()
	conn := detector.NewConn(newLoopbackConn())
	source := netip.MustParseAddrPort("127.0.0.1:40001")

	if _, ok := detector.connMap[source]; !ok {
		t.Fatal("loopback connection was not registered")
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	if _, ok := detector.connMap[source]; ok {
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

func TestLoopbackConcurrentLifecycle(t *testing.T) {
	detector := NewDetector()
	const count = 64
	conns := make([]C.Conn, count)
	var ready sync.WaitGroup
	ready.Add(count)
	var start sync.WaitGroup
	start.Add(1)
	for index := range count {
		go func(index int) {
			defer ready.Done()
			start.Wait()
			conns[index] = detector.NewConn(newLoopbackConnPort(uint16(40000 + index)))
		}(index)
	}
	start.Done()
	ready.Wait()

	for index, conn := range conns {
		if conn == nil {
			t.Fatal("loopback connection was not wrapped")
		}
		source := netip.MustParseAddrPort(fmt.Sprintf("127.0.0.1:%d", 40000+index))
		if _, ok := detector.connMap[source]; !ok {
			t.Fatal("loopback connection was not registered")
		}
		if err := conn.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if len(detector.connMap) != 0 {
		t.Fatal("loopback connections were not removed")
	}
}
