package sudoku

import (
	"net"
	"net/netip"
	"testing"

	"github.com/metacubex/mihomo/common/pool"
	"github.com/metacubex/mihomo/component/nat"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/transport/sudoku"
	"github.com/stretchr/testify/require"
)

type uotCaptureTunnel struct {
	C.Tunnel
	metadatas []*C.Metadata
	wrappers  []*uotPacket
}

func (c *uotCaptureTunnel) HandleUDPPacket(udpPacket C.UDPPacket, metadata *C.Metadata) {
	c.metadatas = append(c.metadatas, metadata)
	if pkt, ok := udpPacket.(*uotPacket); ok {
		c.wrappers = append(c.wrappers, pkt)
	}
	udpPacket.Drop()
}

type uotDropTunnel struct {
	C.Tunnel
}

func (uotDropTunnel) HandleUDPPacket(udpPacket C.UDPPacket, _ *C.Metadata) {
	udpPacket.Drop()
}

// uotAdapterTunnel keeps the adapter a flow's write-back proxy holds.
type uotAdapterTunnel struct {
	C.Tunnel
	adapter C.PacketAdapter
}

func (c *uotAdapterTunnel) HandleUDPPacket(udpPacket C.UDPPacket, metadata *C.Metadata) {
	c.adapter = C.NewPacketAdapter(udpPacket, metadata)
}

func newTestUoTSession(t *testing.T, tunnel C.Tunnel) (uotSession, net.Conn) {
	t.Helper()
	server, client := net.Pipe()
	t.Cleanup(func() {
		_ = server.Close()
		_ = client.Close()
	})
	return uotSession{
		writer: sudoku.NewUoTPacketConn(server),
		source: netip.MustParseAddrPort("192.168.1.2:54321"),
		tunnel: tunnel,
	}, client
}

func TestUoTSessionReusesMetadataAndReturnsThePayload(t *testing.T) {
	tunnel := &uotCaptureTunnel{}
	session, _ := newTestUoTSession(t, tunnel)
	target := netip.MustParseAddrPort("1.2.3.4:53")

	session.handle(target, pool.Get(8))
	session.handle(target, pool.Get(8))
	session.handle(target, pool.Get(8))

	require.Len(t, tunnel.metadatas, 3)
	require.Same(t, tunnel.metadatas[0], tunnel.metadatas[1])
	require.Same(t, tunnel.metadatas[0], tunnel.metadatas[2])
	require.Equal(t, "1.2.3.4", tunnel.metadatas[0].DstIP.String())
	require.EqualValues(t, 53, tunnel.metadatas[0].DstPort)
	require.EqualValues(t, 54321, tunnel.metadatas[0].SrcPort)
	require.Equal(t, C.SUDOKU, tunnel.metadatas[0].Type)

	// Drop releases the payload but leaves the wrapper whole.
	for _, wrapper := range tunnel.wrappers {
		require.Nil(t, wrapper.payload)
		require.NotNil(t, wrapper.writer)
		require.EqualValues(t, 54321, wrapper.source.Port())
	}
}

// A flow answers its replies through the last packet the sender handed it, so
// the wrapper must still write back to its own client after Drop.
func TestUoTPacketWriteBackSurvivesDrop(t *testing.T) {
	tunnel := &uotAdapterTunnel{}
	session, client := newTestUoTSession(t, tunnel)
	session.handle(netip.MustParseAddrPort("1.2.3.4:53"), pool.Get(8))
	require.NotNil(t, tunnel.adapter)

	replyTarget := netip.MustParseAddrPort("5.6.7.8:1234")
	received := make(chan netip.AddrPort, 1)
	go func() {
		if target, _, err := sudoku.ReadDatagram(client); err == nil {
			received <- target
		}
	}()

	proxy := nat.NewWriteBackProxy(tunnel.adapter)
	tunnel.adapter.Drop()
	_, err := proxy.WriteBack([]byte("reply"), replyTarget)
	require.NoError(t, err)
	require.Equal(t, replyTarget, <-received)
}

// The flow cache and the payload pool are the whole warm-path cost: a repeated
// datagram to a known destination allocates its wrapper and nothing else.
func TestUoTWarmHandleAllocatesTheWrapperOnly(t *testing.T) {
	session, _ := newTestUoTSession(t, uotDropTunnel{})
	target := netip.MustParseAddrPort("1.2.3.4:53")

	session.handle(target, pool.Get(8))
	payload := pool.Get(8)

	allocations := testing.AllocsPerRun(100, func() {
		session.handle(target, payload)
	})
	require.EqualValues(t, 1, allocations)
}
