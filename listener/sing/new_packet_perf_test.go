package sing

import (
	"context"
	"net/netip"
	"testing"

	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/sing/common/buf"
	M "github.com/metacubex/sing/common/metadata"
	"github.com/metacubex/sing/common/network"
	"github.com/stretchr/testify/require"
)

type fakeNetPacketWriter struct{}

func (fakeNetPacketWriter) WritePacket(*buf.Buffer, M.Socksaddr) error { return nil }

type singleCaptureTunnel struct {
	C.Tunnel
	packet   C.UDPPacket
	metadata *C.Metadata
}

func (t *singleCaptureTunnel) HandleUDPPacket(packet C.UDPPacket, metadata *C.Metadata) {
	packet.Drop()
	t.packet = packet
	t.metadata = metadata
}

func BenchmarkNewPacketSingleDatagram(b *testing.B) {
	source := M.Metadata{
		Source:      M.SocksaddrFrom(netip.MustParseAddr("192.168.1.2"), 54321),
		Destination: M.SocksaddrFrom(netip.MustParseAddr("1.2.3.4"), 443),
	}
	tunnel := dropTunnel{}
	handler := &ListenerHandler{ListenerConfig: ListenerConfig{Tunnel: tunnel}}
	init := func(network.PacketConn) network.PacketWriter { return fakeNetPacketWriter{} }

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buffer := buf.NewPacket()
		handler.NewPacket(context.Background(), netip.MustParseAddrPort("192.168.1.2:54321"), buffer, source, init)
	}
}

func TestNewPacketSetsSourceAndDestination(t *testing.T) {
	tunnel := &singleCaptureTunnel{}
	handler := &ListenerHandler{ListenerConfig: ListenerConfig{Tunnel: tunnel}}
	init := func(network.PacketConn) network.PacketWriter { return fakeNetPacketWriter{} }

	metadata := M.Metadata{
		Source:      M.SocksaddrFrom(netip.MustParseAddr("192.168.1.2"), 54321),
		Destination: M.SocksaddrFrom(netip.MustParseAddr("1.2.3.4"), 443),
	}
	buffer := buf.NewPacket()
	_, _ = buffer.Write([]byte("payload"))
	handler.NewPacket(context.Background(), netip.MustParseAddrPort("192.168.1.2:54321"), buffer, metadata, init)

	require.NotNil(t, tunnel.packet)
	require.NotNil(t, tunnel.metadata)
	require.NotNil(t, tunnel.metadata)
	require.EqualValues(t, "192.168.1.2", tunnel.metadata.SrcIP.String())
	require.EqualValues(t, 54321, tunnel.metadata.SrcPort)
	require.EqualValues(t, "1.2.3.4", tunnel.metadata.DstIP.String())
	require.EqualValues(t, 443, tunnel.metadata.DstPort)
	require.Zero(t, tunnel.packet.(*packet).buff)
}
