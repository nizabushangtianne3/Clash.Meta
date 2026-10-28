package tproxy

import (
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/metacubex/mihomo/common/lru"
	"github.com/metacubex/mihomo/common/pool"
	C "github.com/metacubex/mihomo/constant"
	"github.com/stretchr/testify/require"
)

type tproxyDropPacketConn struct{}

func (tproxyDropPacketConn) ReadFrom([]byte) (int, net.Addr, error) {
	return 0, nil, net.ErrClosed
}

func (tproxyDropPacketConn) WriteTo([]byte, net.Addr) (int, error) {
	return 0, net.ErrClosed
}

func (tproxyDropPacketConn) Close() error        { return nil }
func (tproxyDropPacketConn) LocalAddr() net.Addr { return nil }
func (tproxyDropPacketConn) SetDeadline(time.Time) error {
	return nil
}

func (tproxyDropPacketConn) SetReadDeadline(time.Time) error {
	return nil
}

func (tproxyDropPacketConn) SetWriteDeadline(time.Time) error {
	return nil
}

type tproxyDropTunnel struct {
	C.Tunnel
	buffers   [][]byte
	wrappers  []*packet
	metadatas []*C.Metadata
}

func (c *tproxyDropTunnel) HandleUDPPacket(udpPacket C.UDPPacket, metadata *C.Metadata) {
	c.metadatas = append(c.metadatas, metadata)
	if tproxyPacket, ok := udpPacket.(*packet); ok {
		c.wrappers = append(c.wrappers, tproxyPacket)
	}
	c.buffers = append(c.buffers, append([]byte(nil), udpPacket.Data()...))
	udpPacket.Drop()
}

type tproxyDropOnlyTunnel struct {
	C.Tunnel
}

func (tproxyDropOnlyTunnel) HandleUDPPacket(udpPacket C.UDPPacket, _ *C.Metadata) {
	udpPacket.Drop()
}

func newTproxyTestListener() *UDPListener {
	return &UDPListener{
		flowCache: lru.New(
			lru.WithSize[netip.AddrPort, *tproxyUDPFlow](tproxyUDPFlowCacheSize),
			lru.WithAge[netip.AddrPort, *tproxyUDPFlow](tproxyUDPFlowCacheAge),
		),
	}
}

func TestTproxyUDPFlowCacheReusesMetadata(t *testing.T) {
	tunnel := &tproxyDropTunnel{}
	listener := newTproxyTestListener()
	conn := tproxyDropPacketConn{}
	lAddr := netip.MustParseAddrPort("192.168.1.2:54321")
	rAddr := netip.MustParseAddrPort("8.8.8.8:53")

	listener.handlePacketConn(conn, tunnel, []byte("first"), lAddr, rAddr, 46)
	listener.handlePacketConn(conn, tunnel, []byte("second"), lAddr, rAddr, 0)
	listener.handlePacketConn(conn, tunnel, []byte("third"), lAddr, rAddr, 46)

	require.Len(t, tunnel.metadatas, 3)
	require.Same(t, tunnel.metadatas[0], tunnel.metadatas[1])
	require.Same(t, tunnel.metadatas[0], tunnel.metadatas[2])
	require.Equal(t, "8.8.8.8", tunnel.metadatas[0].DstIP.String())
	require.EqualValues(t, 53, tunnel.metadatas[0].DstPort)
	require.EqualValues(t, 54321, tunnel.metadatas[0].SrcPort)
	// the dscp of the flow's first datagram stays on the shared metadata
	require.EqualValues(t, 46, tunnel.metadatas[0].DSCP)
	require.EqualValues(t, 46, tunnel.metadatas[2].DSCP)
}

func TestTproxyUDPFlowCacheSplitsTargets(t *testing.T) {
	tunnel := &tproxyDropTunnel{}
	listener := newTproxyTestListener()
	conn := tproxyDropPacketConn{}
	lAddr := netip.MustParseAddrPort("192.168.1.2:54321")

	listener.handlePacketConn(conn, tunnel, []byte("payload"), lAddr, netip.MustParseAddrPort("8.8.8.8:53"), 0)
	listener.handlePacketConn(conn, tunnel, []byte("payload"), lAddr, netip.MustParseAddrPort("1.1.1.1:53"), 0)

	require.Len(t, tunnel.metadatas, 2)
	require.NotSame(t, tunnel.metadatas[0], tunnel.metadatas[1])
	require.Equal(t, "1.1.1.1", tunnel.metadatas[1].DstIP.String())
	require.EqualValues(t, 53, tunnel.metadatas[1].DstPort)
}

func TestTproxyUDPFlowCacheUnmapsFourInSixSources(t *testing.T) {
	tunnel := &tproxyDropTunnel{}
	listener := newTproxyTestListener()
	conn := tproxyDropPacketConn{}
	lAddr := netip.MustParseAddrPort("[::ffff:192.168.1.2]:54321")
	rAddr := netip.MustParseAddrPort("8.8.8.8:53")

	listener.handlePacketConn(conn, tunnel, []byte("payload"), lAddr, rAddr, 0)
	listener.handlePacketConn(conn, tunnel, []byte("payload"), lAddr, rAddr, 0)

	require.Len(t, tunnel.metadatas, 2)
	require.Same(t, tunnel.metadatas[0], tunnel.metadatas[1])
	require.Equal(t, "192.168.1.2", tunnel.metadatas[0].SrcIP.String())
}

func TestTproxyUDPPacketWrappersReleaseTheirBuffers(t *testing.T) {
	tunnel := &tproxyDropTunnel{}
	listener := newTproxyTestListener()
	conn := &tproxyDropPacketConn{}
	lAddr := netip.MustParseAddrPort("192.168.1.2:54321")
	rAddr := netip.MustParseAddrPort("8.8.8.8:53")

	listener.handlePacketConn(conn, tunnel, []byte("first"), lAddr, rAddr, 46)
	listener.handlePacketConn(conn, tunnel, []byte("second"), lAddr, rAddr, 0)

	require.Len(t, tunnel.buffers, 2)
	require.Equal(t, []byte("first"), tunnel.buffers[0])
	require.Equal(t, []byte("second"), tunnel.buffers[1])
	require.Equal(t, "8.8.8.8", tunnel.metadatas[0].DstIP.String())
	require.EqualValues(t, 53, tunnel.metadatas[0].DstPort)
	require.EqualValues(t, 54321, tunnel.metadatas[0].SrcPort)
	require.EqualValues(t, 46, tunnel.metadatas[0].DSCP)
	for _, wrapper := range tunnel.wrappers {
		require.Nil(t, wrapper.buf)
		require.Equal(t, lAddr, wrapper.lAddr)
		require.Same(t, conn, wrapper.pc)
		require.Same(t, listener, wrapper.l)
	}
}

// One run forwards one datagram through a warm flow and drops it, so the
// reported allocs are the steady-state cost with tunnel ownership.
func BenchmarkTproxyUDPHandle(b *testing.B) {
	tunnel := tproxyDropOnlyTunnel{}
	listener := newTproxyTestListener()
	conn := &tproxyDropPacketConn{}
	lAddr := netip.MustParseAddrPort("192.168.1.2:54321")
	rAddr := netip.MustParseAddrPort("8.8.8.8:53")
	// the read loop's bufs come from the pool, so the warm path puts one back
	buf := pool.Get(pool.UDPBufferSize)
	copy(buf, "payload")
	listener.handlePacketConn(conn, tunnel, buf[:7], lAddr, rAddr, 0)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		listener.handlePacketConn(conn, tunnel, buf[:7], lAddr, rAddr, 0)
	}
}
