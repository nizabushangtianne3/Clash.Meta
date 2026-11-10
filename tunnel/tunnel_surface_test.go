package tunnel

import (
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/metacubex/mihomo/component/process"
	"github.com/metacubex/mihomo/component/sniffer"
	C "github.com/metacubex/mihomo/constant"
	"github.com/stretchr/testify/require"
)

// The tunnel answers the surface its host reads and writes. A test walks it end
// to end, so no accessor is left unread by the package that owns it.
func TestTunnelAccessorsAnswerWhatTheHostReadsAndWrites(t *testing.T) {
	previousMode := Mode()
	previousStatus := Status()
	previousFindProcess := FindProcessMode()
	t.Cleanup(func() {
		SetMode(previousMode)
		status.Store(previousStatus)
		SetFindProcessMode(previousFindProcess)
		UpdateProxies(nil, nil)
	})

	UpdateProxies(map[string]C.Proxy{"DIRECT": directTestProxy{}}, nil)
	require.Equal(t, "DIRECT", Proxies()["DIRECT"].Name())
	require.Equal(t, "DIRECT", Tunnel.Proxies()["DIRECT"].Name(),
		"the interface and the package name answer the same table")
	require.Empty(t, Providers())
	require.Empty(t, RuleProviders())
	require.Empty(t, Tunnel.Providers(), "the interface answers the same table")
	require.Empty(t, Tunnel.RuleProviders())
	require.Empty(t, Rules())

	SetMode(Global)
	require.Equal(t, Global, Mode())

	SetFindProcessMode(process.FindProcessAlways)
	require.Equal(t, process.FindProcessAlways, FindProcessMode())

	OnSuspend()
	require.Equal(t, Suspend, Status())
	OnInnerLoading()
	require.Equal(t, Inner, Status())
	OnRunning()
	require.Equal(t, Running, Status())

	// Sniffing is a property of the sniffer a config installs; without one
	// there is nothing to enable and nothing reports as enabled.
	require.False(t, IsSniffing())
	SetSniffing(true)
	require.False(t, IsSniffing())

	// A sniffer a config installs owns the flag, and the flag is what the
	// answer reads.
	dispatcher, err := sniffer.NewDispatcher(&sniffer.Config{Enable: true})
	require.NoError(t, err)
	UpdateSniffer(dispatcher)
	require.True(t, IsSniffing(), "an enabled sniffer turns sniffing on")
	SetSniffing(false)
	require.False(t, IsSniffing())
	UpdateSniffer(nil)

	require.NotNil(t, NatTable())
	require.NotNil(t, Tunnel.NatTable())
	require.NotNil(t, Tunnel.RuleUpdateCallback())
}

// A UDP packet whose flow has no key is dropped where it stands, so a worker
// never inherits a packet it cannot route.
func TestTunnelDropsUDPPacketWithoutAFlowKey(t *testing.T) {
	packet := &droppedTestPacket{}

	Tunnel.HandleUDPPacket(packet, &C.Metadata{})
	require.True(t, packet.dropped.Load(),
		"a packet with no flow key must be dropped, not queued")
}

type droppedTestPacket struct {
	// The nil embedded interface stands for the methods a dropped packet is
	// never asked for.
	C.UDPPacket
	dropped atomic.Bool
}

func (p *droppedTestPacket) ReadFrom(b []byte) (int, net.Addr, error)     { return 0, nil, nil }
func (p *droppedTestPacket) WriteTo(b []byte, addr net.Addr) (int, error) { return len(b), nil }
func (p *droppedTestPacket) Close() error                                 { return nil }

// An empty address is not a flow key, which is what makes this packet one the
// tunnel drops where it stands.
func (p *droppedTestPacket) LocalAddr() net.Addr                { return &net.UDPAddr{} }
func (p *droppedTestPacket) SetDeadline(t time.Time) error      { return nil }
func (p *droppedTestPacket) SetReadDeadline(t time.Time) error  { return nil }
func (p *droppedTestPacket) SetWriteDeadline(t time.Time) error { return nil }
func (p *droppedTestPacket) Data() []byte                       { return nil }
func (p *droppedTestPacket) Drop()                              { p.dropped.Store(true) }
