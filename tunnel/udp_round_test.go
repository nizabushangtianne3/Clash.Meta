package tunnel

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	N "github.com/metacubex/mihomo/common/net"
	"github.com/metacubex/mihomo/component/resolver"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/tunnel/statistic"
	"github.com/stretchr/testify/require"
)

// Host lookups map nothing: a destination that is not a fake IP stays as it is.
type directHostMapper struct{}

func (directHostMapper) FakeIPEnabled() bool               { return false }
func (directHostMapper) MappingEnabled() bool              { return false }
func (directHostMapper) IsFakeIP(netip.Addr) bool          { return false }
func (directHostMapper) IsFakeBroadcastIP(netip.Addr) bool { return false }
func (directHostMapper) IsExistFakeIP(netip.Addr) bool     { return false }
func (directHostMapper) FindHostByIP(netip.Addr) (string, bool) {
	return "", false
}
func (directHostMapper) FlushFakeIP() error                { return nil }
func (directHostMapper) InsertHostByIP(netip.Addr, string) {}
func (directHostMapper) StoreFakePoolState()               {}

// The proxy a datagram is addressed to: it names the destination the metadata
// carries, which is what the direct adapter does for UDP. The socket is
// unconnected, because the flow writes each datagram with WriteTo.
type udpDirectProxy struct {
	C.Proxy
}

func (udpDirectProxy) Name() string { return "DIRECT" }

func (udpDirectProxy) ListenPacketContext(
	ctx context.Context,
	metadata *C.Metadata,
) (C.PacketConn, error) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	return &udpTestPacketConn{EnhancePacketConn: N.NewEnhancePacketConn(pc)}, nil
}

type udpTestPacketConn struct {
	N.EnhancePacketConn
}

func (c *udpTestPacketConn) Chains() C.Chain               { return nil }
func (c *udpTestPacketConn) ProviderChains() C.Chain       { return nil }
func (c *udpTestPacketConn) AppendToChains(C.ProxyAdapter) {}
func (c *udpTestPacketConn) RemoteDestination() string     { return "" }

func (c *udpTestPacketConn) ResolveUDP(context.Context, *C.Metadata) error {
	return nil
}

// The packet a datagram arrives on: the write-back is what the flow answers
// through, so it is what this records.
type echoPacket struct {
	data     []byte
	answered chan string
	laddr    net.Addr
}

func (p *echoPacket) Data() []byte { return p.data }

func (p *echoPacket) WriteBack(b []byte, _ netip.AddrPort) (int, error) {
	select {
	case p.answered <- string(b):
	default:
	}
	return len(b), nil
}

func (p *echoPacket) Drop() {}

func (p *echoPacket) LocalAddr() net.Addr { return p.laddr }

// A packet whose drop can be counted: a flow that never started leaves the
// packet where it was. The count is atomic because the flow goroutine drops it.
type dropCountingPacket struct {
	data    []byte
	dropped atomic.Int32
	laddr   net.Addr
}

func (p *dropCountingPacket) Data() []byte { return p.data }

func (p *dropCountingPacket) WriteBack(b []byte, _ netip.AddrPort) (int, error) {
	return len(b), nil
}

func (p *dropCountingPacket) Drop() { p.dropped.Add(1) }

func (p *dropCountingPacket) LocalAddr() net.Addr { return p.laddr }

// A proxy that cannot carry the datagram.
type refusedProxy struct {
	C.Proxy
}

func (refusedProxy) Name() string { return "DIRECT" }

func (refusedProxy) ListenPacketContext(
	ctx context.Context,
	metadata *C.Metadata,
) (C.PacketConn, error) {
	return nil, errors.New("udp refused")
}

// A datagram a proxy answers: the packet is admitted to a queue, the flow is
// started, the proxy is dialed for the destination, and the answer is written
// back to the packet the datagram arrived on. Together these are the whole UDP
// data path, and the functions it walks had no coverage before.
func TestHandleUDPPacketRoundsThroughTheProxy(t *testing.T) {
	restoreStatus := status.Load()
	status.Store(Running)
	defer status.Store(restoreStatus)

	restoreMapper := resolver.DefaultHostMapper
	resolver.DefaultHostMapper = directHostMapper{}
	defer func() { resolver.DefaultHostMapper = restoreMapper }()

	server, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	serverPort := server.LocalAddr().(*net.UDPAddr).Port
	t.Cleanup(func() { _ = server.Close() })
	go func() {
		buf := make([]byte, 2048)
		for {
			n, addr, err := server.ReadFrom(buf)
			if err != nil {
				return
			}
			_, _ = server.WriteTo(append([]byte("echo:"), buf[:n]...), addr)
		}
	}()

	UpdateProxies(map[string]C.Proxy{"DIRECT": udpDirectProxy{}}, nil)
	SetMode(Direct)
	t.Cleanup(func() {
		SetMode(Direct)
		UpdateProxies(nil, nil)
		invalidateAllProxies()
	})

	answered := make(chan string, 1)
	const srcPort = 53531
	packet := &echoPacket{
		data:     []byte("query"),
		answered: answered,
		laddr:    &net.UDPAddr{IP: net.ParseIP("192.168.1.100"), Port: srcPort},
	}
	metadata := &C.Metadata{
		NetWork: C.UDP,
		Type:    C.INNER,
		SrcIP:   netip.AddrFrom4([4]byte{192, 168, 1, 100}),
		SrcPort: srcPort,
		DstIP:   netip.MustParseAddr("127.0.0.1"),
		DstPort: uint16(serverPort),
	}

	// The nat table is walked by the flow key, so an entry a previous test left
	// for the same source must not answer this datagram with a sender that has
	// no read loop behind it, and this flow must not outlive the test.
	flowKey, _ := packetFlowKey(C.NewPacketAdapter(packet, metadata))
	natTable.DeleteForFlow(flowKey)
	t.Cleanup(func() { natTable.DeleteForFlow(flowKey) })

	tunnel{}.HandleUDPPacket(packet, metadata)

	select {
	case reply := <-answered:
		require.Equal(t, "echo:query", reply)
	case <-time.After(5 * time.Second):
		t.Fatal("the flow never wrote the answer back to the packet")
	}
}

// A proxy that cannot carry the datagram leaves nothing behind: the flow it
// would have started is not in the table for the next datagram to be admitted
// to, and no connection is tracked for it.
func TestHandleUDPPacketDropsAFlowTheProxyCannotDial(t *testing.T) {
	restoreStatus := status.Load()
	status.Store(Running)
	defer status.Store(restoreStatus)

	restoreMapper := resolver.DefaultHostMapper
	resolver.DefaultHostMapper = directHostMapper{}
	defer func() { resolver.DefaultHostMapper = restoreMapper }()

	UpdateProxies(map[string]C.Proxy{"DIRECT": refusedProxy{}}, nil)
	SetMode(Direct)
	t.Cleanup(func() {
		SetMode(Direct)
		UpdateProxies(nil, nil)
		invalidateAllProxies()
	})

	const srcPort = 53532
	packet := &dropCountingPacket{
		data:  []byte("query"),
		laddr: &net.UDPAddr{IP: net.ParseIP("192.168.1.100"), Port: srcPort},
	}
	metadata := &C.Metadata{
		NetWork: C.UDP,
		Type:    C.INNER,
		SrcIP:   netip.AddrFrom4([4]byte{192, 168, 1, 100}),
		SrcPort: srcPort,
		DstIP:   netip.MustParseAddr("127.0.0.1"),
		DstPort: 1,
	}

	flowKey, _ := packetFlowKey(C.NewPacketAdapter(packet, metadata))
	natTable.DeleteForFlow(flowKey)
	t.Cleanup(func() { natTable.DeleteForFlow(flowKey) })
	trackedBefore := statistic.DefaultManager.Size()

	tunnel{}.HandleUDPPacket(packet, metadata)

	// The error is not one the retry loop classifies as terminal, so the dial is
	// attempted across its budget before the flow is given up on.
	require.Eventually(t, func() bool {
		return packet.dropped.Load() > 0
	}, 8*time.Second, 5*time.Millisecond, "the datagram is dropped with the flow that never started")
	require.Eventually(t, func() bool {
		_, exist := natTable.LoadForFlow(flowKey)
		return !exist
	}, 8*time.Second, 5*time.Millisecond, "the refused flow leaves the table")
	require.Equal(t, trackedBefore, statistic.DefaultManager.Size(),
		"a flow that never started tracks no connection")
}
