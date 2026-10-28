package tunnel

import (
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/metacubex/mihomo/common/lru"
	"github.com/metacubex/mihomo/common/pool"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/transport/socks5"
	"github.com/stretchr/testify/require"
)

type tunnelUDPRead struct {
	data []byte
	addr netip.AddrPort
	err  error
}

type tunnelDropPacketConn struct {
	reads       []tunnelUDPRead
	readIndex   int
	wroteToAddr []netip.AddrPort
}

func (c *tunnelDropPacketConn) WaitReadFrom() ([]byte, netip.AddrPort, error) {
	if c.readIndex >= len(c.reads) {
		return nil, netip.AddrPort{}, net.ErrClosed
	}
	read := c.reads[c.readIndex]
	c.readIndex++
	return read.data, read.addr, read.err
}

func (c *tunnelDropPacketConn) WriteToUDPAddrPort(_ []byte, addr netip.AddrPort) (int, error) {
	c.wroteToAddr = append(c.wroteToAddr, addr)
	return 0, nil
}

func (tunnelDropPacketConn) WriteTo([]byte, net.Addr) (int, error) {
	return 0, net.ErrClosed
}

func (tunnelDropPacketConn) ReadFrom([]byte) (int, net.Addr, error) {
	return 0, nil, net.ErrClosed
}

func (tunnelDropPacketConn) Close() error        { return nil }
func (tunnelDropPacketConn) LocalAddr() net.Addr { return nil }
func (tunnelDropPacketConn) SetDeadline(time.Time) error {
	return nil
}

func (tunnelDropPacketConn) SetReadDeadline(time.Time) error {
	return nil
}

func (tunnelDropPacketConn) SetWriteDeadline(time.Time) error {
	return nil
}

type tunnelDropTunnel struct {
	C.Tunnel
	payloads [][]byte
	wrappers []*tunnelUDPPacket
}

func (c *tunnelDropTunnel) HandleUDPPacket(udpPacket C.UDPPacket, _ *C.Metadata) {
	if tunnelPacket, ok := udpPacket.(*tunnelUDPPacket); ok {
		c.wrappers = append(c.wrappers, tunnelPacket)
	}
	c.payloads = append(c.payloads, append([]byte(nil), udpPacket.Data()...))
	udpPacket.Drop()
}

func TestTunnelUDPPacketWrappersReleaseTheirBuffers(t *testing.T) {
	source := netip.MustParseAddrPort("192.168.1.2:54321")
	conn := &tunnelDropPacketConn{
		reads: []tunnelUDPRead{
			{data: tunnelPooledData("first"), addr: source},
			{addr: source},
			{err: net.ErrClosed},
		},
	}
	tunnel := &tunnelDropTunnel{}
	listener := newTunnelTestListener()

	listener.serveUDP(conn, tunnel)

	require.Len(t, tunnel.payloads, 2)
	require.Equal(t, []byte("first"), tunnel.payloads[0])
	require.Empty(t, tunnel.payloads[1])
	for _, wrapper := range tunnel.wrappers {
		require.Nil(t, wrapper.payload)
		require.Same(t, conn, wrapper.pc)
		require.Equal(t, source, wrapper.FlowKey())
		require.Equal(t, source, wrapper.LocalAddr().(*net.UDPAddr).AddrPort())
	}
}

func TestTunnelUDPPacketWriteBackUsesAddrPort(t *testing.T) {
	conn := &tunnelDropPacketConn{}
	source := netip.MustParseAddrPort("192.168.1.2:54321")
	packet := newTunnelUDPPacket(conn, source, nil)

	_, err := packet.WriteBack([]byte("reply"), netip.MustParseAddrPort("8.8.8.8:53"))

	require.NoError(t, err)
	require.Equal(t, []netip.AddrPort{source}, conn.wroteToAddr)
}

func tunnelPooledData(value string) []byte {
	buf := pool.Get(len(value))
	copy(buf, value)
	return buf
}

type tunnelCaptureTunnel struct {
	C.Tunnel
	payloads  [][]byte
	wrappers  []*tunnelUDPPacket
	metadatas []*C.Metadata
}

func (c *tunnelCaptureTunnel) HandleUDPPacket(udpPacket C.UDPPacket, metadata *C.Metadata) {
	if tunnelPacket, ok := udpPacket.(*tunnelUDPPacket); ok {
		c.wrappers = append(c.wrappers, tunnelPacket)
	}
	c.payloads = append(c.payloads, append([]byte(nil), udpPacket.Data()...))
	c.metadatas = append(c.metadatas, metadata)
	udpPacket.Drop()
}

func newTunnelTestListener() *PacketConn {
	return &PacketConn{
		target: socks5.AddrFromStdAddrPort(netip.MustParseAddrPort("8.8.8.8:53")),
		flowCache: lru.New(
			lru.WithSize[netip.AddrPort, *tunUDPFlow](1024),
			lru.WithAge[netip.AddrPort, *tunUDPFlow](300),
		),
	}
}

func TestTunnelUDPFlowCacheReusesMetadata(t *testing.T) {
	tunnel := &tunnelCaptureTunnel{}
	listener := newTunnelTestListener()
	source := netip.MustParseAddrPort("192.168.1.2:54321")

	listener.handleUDP(tunnelDropPacketConn{}, tunnel, tunnelPooledData("first"), source)
	listener.handleUDP(tunnelDropPacketConn{}, tunnel, tunnelPooledData("second"), source)

	require.Len(t, tunnel.metadatas, 2)
	require.Same(t, tunnel.metadatas[0], tunnel.metadatas[1])
	require.Equal(t, "8.8.8.8", tunnel.metadatas[0].DstIP.String())
	require.EqualValues(t, 53, tunnel.metadatas[0].DstPort)
	require.Equal(t, "192.168.1.2", tunnel.metadatas[0].SrcIP.String())
	require.EqualValues(t, 54321, tunnel.metadatas[0].SrcPort)
}

func TestTunnelUDPFlowCacheSplitsSources(t *testing.T) {
	tunnel := &tunnelCaptureTunnel{}
	listener := newTunnelTestListener()

	listener.handleUDP(tunnelDropPacketConn{}, tunnel, tunnelPooledData("first"), netip.MustParseAddrPort("192.168.1.2:54321"))
	listener.handleUDP(tunnelDropPacketConn{}, tunnel, tunnelPooledData("second"), netip.MustParseAddrPort("192.168.1.3:54322"))

	require.Len(t, tunnel.metadatas, 2)
	require.NotSame(t, tunnel.metadatas[0], tunnel.metadatas[1])
	require.Equal(t, "192.168.1.3", tunnel.metadatas[1].SrcIP.String())
}

type tunnelDropOnlyTunnel struct {
	C.Tunnel
}

func (tunnelDropOnlyTunnel) HandleUDPPacket(udpPacket C.UDPPacket, _ *C.Metadata) {
	udpPacket.Drop()
}

func BenchmarkTunnelUDPHandle(b *testing.B) {
	tunnel := tunnelDropOnlyTunnel{}
	listener := newTunnelTestListener()
	source := netip.MustParseAddrPort("192.168.1.2:54321")
	buf := pool.Get(pool.UDPBufferSize)
	copy(buf, "payload")
	listener.handleUDP(tunnelDropPacketConn{}, tunnel, buf[:7], source)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		listener.handleUDP(tunnelDropPacketConn{}, tunnel, buf[:7], source)
	}
}
