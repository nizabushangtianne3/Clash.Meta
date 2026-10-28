package inner_test

import (
	"io"
	"net"
	"net/netip"
	"testing"

	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/listener/inner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var answerFrom = netip.MustParseAddrPort("10.0.0.9:5353")

type stubTunnel struct {
	tcpReady chan struct{}

	tcpConn net.Conn
	tcpMeta *C.Metadata

	udpMeta *C.Metadata
	udpData []byte

	lastPacket C.UDPPacket
}

func newStubTunnel() *stubTunnel {
	return &stubTunnel{tcpReady: make(chan struct{})}
}

func (t *stubTunnel) HandleTCPConn(conn net.Conn, metadata *C.Metadata) {
	t.tcpConn = conn
	t.tcpMeta = metadata
	close(t.tcpReady)
	_, _ = io.Copy(conn, conn)
}

func (t *stubTunnel) HandleUDPPacket(packet C.UDPPacket, metadata *C.Metadata) {
	t.udpMeta = metadata
	t.udpData = append([]byte(nil), packet.Data()...)
	t.lastPacket = packet
	_, _ = packet.WriteBack(packet.Data(), answerFrom)
}

func (t *stubTunnel) NatTable() C.NatTable { return nil }

func TestTCPHandoffFailsWithoutATunnel(t *testing.T) {
	conn, err := inner.HandleTcp(nil, "1.2.3.4:80", "")

	require.Error(t, err)
	assert.Nil(t, conn)
	assert.Contains(t, err.Error(), "tunnel uninitialized")
}

func TestTCPHandoffCarriesTheInnerMetadata(t *testing.T) {
	tunnel := newStubTunnel()

	conn, err := inner.HandleTcp(tunnel, "1.2.3.4:80", "proxy-a")
	require.NoError(t, err)
	defer conn.Close()

	// The tunnelling goroutine is what fills the metadata in.
	<-tunnel.tcpReady

	// The half handed to the tunnel is the far end of the pipe, so what this
	// side writes is what the tunnel reads.
	_, err = conn.Write([]byte("ping"))
	require.NoError(t, err)

	metadata := tunnel.tcpMeta
	require.NotNil(t, metadata)
	assert.Equal(t, C.TCP, metadata.NetWork)
	assert.Equal(t, C.INNER, metadata.Type)
	assert.Equal(t, C.DNSNormal, metadata.DNSMode)
	assert.Equal(t, C.MihomoName, metadata.Process)
	assert.Equal(t, "proxy-a", metadata.SpecialProxy)
	assert.Equal(t, "1.2.3.4", metadata.DstIP.String())
	assert.EqualValues(t, 80, metadata.DstPort)
	assert.Equal(t, "1.2.3.4:80", metadata.RemoteAddress())
}

func TestTCPHandoffDefaultsTheProxyToEmpty(t *testing.T) {
	tunnel := newStubTunnel()

	conn, err := inner.HandleTcp(tunnel, "example.com:443", "")
	require.NoError(t, err)
	defer conn.Close()

	<-tunnel.tcpReady
	require.NotNil(t, tunnel.tcpMeta)
	assert.Empty(t, tunnel.tcpMeta.SpecialProxy)
}

// An address the metadata cannot parse must fail the handoff rather than
// reach the tunnel.
func TestTCPHandoffRejectsAnUnparseableAddress(t *testing.T) {
	for _, target := range []string{"no-port-here", "1.2.3.4"} {
		conn, err := inner.HandleTcp(newStubTunnel(), target, "")
		require.Error(t, err, target)
		assert.Nil(t, conn, target)
	}
}

func TestUDPHandoffFailsWithoutATunnel(t *testing.T) {
	conn, addr, err := inner.HandleUdp(nil, "udp", "1.2.3.4:80", "")

	require.Error(t, err)
	assert.Nil(t, conn)
	assert.Nil(t, addr)
	assert.Contains(t, err.Error(), "tunnel uninitialized")
}

func TestUDPHandoffOnlyAcceptsTheUDPNetworks(t *testing.T) {
	tunnel := newStubTunnel()
	for _, network := range []string{"tcp", "ip", ""} {
		conn, _, err := inner.HandleUdp(tunnel, network, "1.2.3.4:80", "")

		require.Error(t, err, network)
		assert.Nil(t, conn, network)
		assert.Contains(t, err.Error(), "unsupported network", network)
	}

	for _, network := range []string{"udp", "udp4", "udp6"} {
		conn, addr, err := inner.HandleUdp(tunnel, network, "1.2.3.4:80", "")
		require.NoError(t, err, network)
		require.NotNil(t, conn, network)
		assert.Equal(t, network, addr.Network())
		require.NoError(t, conn.Close())
	}
}

func TestUDPHandoffRejectsAnUnparseableTarget(t *testing.T) {
	tunnel := newStubTunnel()

	for _, target := range []string{"", "not an address", "1.2.3.4", "1.2.3.4:99999"} {
		conn, _, err := inner.HandleUdp(tunnel, "udp", target, "")

		require.Error(t, err, target)
		assert.Nil(t, conn, target)
		assert.Contains(t, err.Error(), "invalid target address", target)
	}
}

func TestUDPHandoffAnswersOverTheSameConn(t *testing.T) {
	tunnel := newStubTunnel()

	conn, addr, err := inner.HandleUdp(tunnel, "udp", "1.2.3.4:80", "proxy-b")
	require.NoError(t, err)
	defer conn.Close()

	assert.Equal(t, "udp", conn.LocalAddr().Network())
	assert.Equal(t, "1.2.3.4:80", addr.String())

	payload := []byte("udp payload")
	n, err := conn.WriteTo(payload, addr)
	require.NoError(t, err)
	require.Equal(t, len(payload), n)
	assert.Equal(t, payload, tunnel.udpData)
	require.NotNil(t, tunnel.udpMeta)
	assert.Equal(t, C.UDP, tunnel.udpMeta.NetWork)
	assert.Equal(t, C.INNER, tunnel.udpMeta.Type)
	assert.Equal(t, "proxy-b", tunnel.udpMeta.SpecialProxy)

	// The answer is produced by the tunnel writing back through the packet it
	// was handed.
	buffer := make([]byte, len(payload))
	read, from, err := conn.ReadFrom(buffer)
	require.NoError(t, err)
	assert.Equal(t, payload, buffer[:read])
	assert.Equal(t, answerFrom, netip.MustParseAddrPort(from.String()))
}

func TestUDPHandoffNeedsATargetToWrite(t *testing.T) {
	tunnel := newStubTunnel()
	conn, _, err := inner.HandleUdp(tunnel, "udp", "1.2.3.4:80", "")
	require.NoError(t, err)
	defer conn.Close()

	n, err := conn.WriteTo([]byte("payload"), nil)
	require.Error(t, err)
	assert.Equal(t, 0, n)
}

func TestUDPHandoffStopsAfterClose(t *testing.T) {
	tunnel := newStubTunnel()
	conn, addr, err := inner.HandleUdp(tunnel, "udp", "1.2.3.4:80", "")
	require.NoError(t, err)
	require.NoError(t, conn.Close())

	_, _, err = conn.ReadFrom(make([]byte, 16))
	assert.ErrorIs(t, err, net.ErrClosed)
	_, err = conn.WriteTo([]byte("payload"), addr)
	assert.ErrorIs(t, err, net.ErrClosed)
}

func TestUDPHandoffReportsAShortRead(t *testing.T) {
	tunnel := newStubTunnel()
	conn, addr, err := inner.HandleUdp(tunnel, "udp", "1.2.3.4:80", "")
	require.NoError(t, err)
	defer conn.Close()

	_, err = conn.WriteTo([]byte("a longer payload"), addr)
	require.NoError(t, err)

	read, _, err := conn.ReadFrom(make([]byte, 4))
	assert.ErrorIs(t, err, io.ErrShortBuffer)
	assert.Equal(t, 4, read)
}

func TestGetTunnelReturnsWhatWasInstalled(t *testing.T) {
	previous := inner.GetTunnel()
	installed := &stubTunnel{}
	inner.New(installed)
	t.Cleanup(func() { inner.New(previous) })

	assert.Same(t, installed, inner.GetTunnel())
}

// The packet the tunnel is handed stays the write-back channel after the
// conn closes, and must report the closure instead of queueing.
func TestPacketWriteBackFailsAfterTheConnCloses(t *testing.T) {
	tunnel := newStubTunnel()
	conn, addr, err := inner.HandleUdp(tunnel, "udp", "1.2.3.4:80", "")
	require.NoError(t, err)

	_, err = conn.WriteTo([]byte("payload"), addr)
	require.NoError(t, err)
	require.NotNil(t, tunnel.lastPacket)

	require.NoError(t, conn.Close())

	n, err := tunnel.lastPacket.WriteBack([]byte("answer"), answerFrom)
	assert.ErrorIs(t, err, net.ErrClosed)
	assert.Equal(t, 0, n)
}

// A datagram the queue cannot hold is released rather than blocking the
// caller, and the caller is told the full length was taken.
func TestPacketWriteBackDropsWhenTheQueueIsFull(t *testing.T) {
	tunnel := newStubTunnel()
	conn, addr, err := inner.HandleUdp(tunnel, "udp", "1.2.3.4:80", "")
	require.NoError(t, err)
	defer conn.Close()

	// One more than the conn's queue holds. Each write hands the tunnel a
	// packet that answers itself, so the queue is full before the last one and
	// that datagram has nowhere to go.
	const overCapacity = 129
	for range overCapacity {
		_, err = conn.WriteTo([]byte("payload"), addr)
		require.NoError(t, err)
	}

	require.NotNil(t, tunnel.lastPacket)
	n, err := tunnel.lastPacket.WriteBack([]byte("dropped"), answerFrom)
	require.NoError(t, err)
	assert.Equal(t, len("dropped"), n)
}
