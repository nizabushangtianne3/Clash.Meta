package sudoku

import (
	"net"
	"net/netip"
	"testing"

	"github.com/metacubex/mihomo/common/pool"
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

func TestUoTSessionReusesMetadataAndKeepsWrappersWhole(t *testing.T) {
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

	require.NotSame(t, tunnel.wrappers[0], tunnel.wrappers[1])
	require.NotSame(t, tunnel.wrappers[0], tunnel.wrappers[2])
	for _, wrapper := range tunnel.wrappers {
		require.Nil(t, wrapper.payload)
		// A flow keeps the packet it last handed the sender so replies can be
		// written back through it after Drop.
		require.NotNil(t, wrapper.writer)
	}
}

func TestUoTWarmHandleAllocatesOnlyTheWrapper(t *testing.T) {
	session, _ := newTestUoTSession(t, uotDropTunnel{})
	target := netip.MustParseAddrPort("1.2.3.4:53")

	session.handle(target, pool.Get(8))
	payload := pool.Get(8)

	allocations := testing.AllocsPerRun(100, func() {
		session.handle(target, payload)
	})
	require.EqualValues(t, 1, allocations)
}

func TestUoTWriteBackKeepsItsWriterAfterDrop(t *testing.T) {
	tunnel := &uotCaptureTunnel{}
	session, client := newTestUoTSession(t, tunnel)
	session.handle(netip.MustParseAddrPort("1.2.3.4:53"), pool.Get(8))

	replyTarget := netip.MustParseAddrPort("8.8.8.8:53")
	reply := []byte{0xde, 0xad}
	writeErr := make(chan error, 1)
	go func() {
		_, err := tunnel.wrappers[0].WriteBack(reply, replyTarget)
		writeErr <- err
	}()

	gotTarget, gotPayload, err := sudoku.ReadDatagram(client)
	require.NoError(t, err)
	require.Equal(t, replyTarget, gotTarget)
	require.Equal(t, reply, gotPayload)
	require.NoError(t, <-writeErr)
	_ = pool.Put(gotPayload)
}
