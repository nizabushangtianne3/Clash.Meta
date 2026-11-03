package tunnel

import (
	"fmt"
	"net"
	"net/netip"
	"testing"

	C "github.com/metacubex/mihomo/constant"
	rulesCommon "github.com/metacubex/mihomo/rules/common"
	"github.com/metacubex/mihomo/rules/wrapper"
	"github.com/stretchr/testify/require"
)

type benchmarkFlowPacket struct {
	C.UDPPacket
	localAddr net.Addr
}

func (packet benchmarkFlowPacket) LocalAddr() net.Addr { return packet.localAddr }
func (benchmarkFlowPacket) Drop()                      {}

func TestHandleUDPConnExistingFlowAdmitsPacket(t *testing.T) {
	previous := status.Load()
	status.Store(Running)
	t.Cleanup(func() { status.Store(previous) })

	key := netip.MustParseAddrPort("192.168.1.2:54321")
	sender, _ := natTable.GetOrCreateForFlow(key, newPacketSender)
	t.Cleanup(func() {
		sender.Close()
		natTable.DeleteForFlow(key)
	})

	packet := C.NewPacketAdapter(
		benchmarkFlowPacket{localAddr: net.UDPAddrFromAddrPort(key)},
		&C.Metadata{
			NetWork: C.UDP,
			Type:    C.SOCKS5,
			DstIP:   netip.MustParseAddr("1.2.3.4"),
			DstPort: 53,
		},
	)

	handleUDPConn(udpPacket{packet: packet, key: key})

	select {
	case queued := <-sender.(*packetSender).ch:
		require.Equal(t, packet, queued)
	default:
		t.Fatal("packet was not admitted to the existing flow")
	}
}

func TestHandleUDPConnExistingFlowDoesNotAllocate(t *testing.T) {
	previous := status.Load()
	status.Store(Running)
	t.Cleanup(func() { status.Store(previous) })

	key := netip.MustParseAddrPort("192.168.1.2:54321")
	sender, _ := natTable.GetOrCreateForFlow(key, newPacketSender)
	t.Cleanup(func() {
		sender.Close()
		natTable.DeleteForFlow(key)
	})

	packet := C.NewPacketAdapter(
		benchmarkFlowPacket{localAddr: net.UDPAddrFromAddrPort(key)},
		&C.Metadata{
			NetWork: C.UDP,
			Type:    C.SOCKS5,
			DstIP:   netip.MustParseAddr("1.2.3.4"),
			DstPort: 53,
		},
	)

	allocations := testing.AllocsPerRun(100, func() {
		handleUDPConn(udpPacket{packet: packet, key: key})
	})
	// go 1.20 adds one toolchain-side allocation here that later toolchains
	// elide; the pre-optimization path allocated five per packet.
	require.LessOrEqual(t, int(allocations), 1)
}

func BenchmarkHandleUDPConnExistingFlow(b *testing.B) {
	previous := status.Load()
	status.Store(Running)
	b.Cleanup(func() { status.Store(previous) })

	key := netip.MustParseAddrPort("192.168.1.2:54321")
	sender, _ := natTable.GetOrCreateForFlow(key, newPacketSender)
	b.Cleanup(func() {
		sender.Close()
		natTable.DeleteForFlow(key)
	})

	packet := C.NewPacketAdapter(
		benchmarkFlowPacket{localAddr: net.UDPAddrFromAddrPort(key)},
		&C.Metadata{
			NetWork: C.UDP,
			Type:    C.SOCKS5,
			DstIP:   netip.MustParseAddr("1.2.3.4"),
			DstPort: 53,
		},
	)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		handleUDPConn(udpPacket{packet: packet, key: key})
	}
}

type nativeQueuedPacket struct {
	C.UDPPacket
	metadata *C.Metadata
	source   netip.AddrPort
	data     []byte
}

func (packet *nativeQueuedPacket) Data() []byte                                  { return packet.data }
func (packet *nativeQueuedPacket) Metadata() *C.Metadata                         { return packet.metadata }
func (packet *nativeQueuedPacket) FlowKey() netip.AddrPort                       { return packet.source }
func (packet *nativeQueuedPacket) WriteBack([]byte, netip.AddrPort) (int, error) { return 0, nil }
func (packet *nativeQueuedPacket) LocalAddr() net.Addr                           { return net.UDPAddrFromAddrPort(packet.source) }
func (*nativeQueuedPacket) Drop()                                                {}

func BenchmarkHandleUDPConnExistingFlowAdapter(b *testing.B) {
	previous := status.Load()
	status.Store(Running)
	b.Cleanup(func() { status.Store(previous) })

	key := netip.MustParseAddrPort("192.168.1.2:54321")
	metadata := &C.Metadata{
		NetWork: C.UDP,
		Type:    C.SOCKS5,
		DstIP:   netip.MustParseAddr("1.2.3.4"),
		DstPort: 53,
	}
	sender, _ := natTable.GetOrCreateForFlow(key, newPacketSender)
	pc := &destinationRecordingPacketConn{}
	go sender.Process(pc, nil)
	b.Cleanup(func() {
		sender.Close()
		natTable.DeleteForFlow(key)
	})
	sender.AddMapping(&C.Metadata{DstIP: netip.MustParseAddr("192.168.1.2")}, &C.Metadata{DstIP: netip.MustParseAddr("203.0.113.3")})

	b.Run("wrapped", func(b *testing.B) {
		packet := deadlineTestPacket{localAddr: net.UDPAddrFromAddrPort(key), data: []byte("ping")}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			adapter := C.NewPacketAdapter(packet, metadata)
			handleUDPConn(udpPacket{packet: adapter, key: key})
		}
	})
	b.Run("native", func(b *testing.B) {
		packet := &nativeQueuedPacket{metadata: metadata, source: key, data: []byte("ping")}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			handleUDPConn(udpPacket{packet: packet, key: key})
		}
	})
}

func benchmarkMetadata() C.Metadata {
	return C.Metadata{
		NetWork: C.TCP,
		Type:    C.HTTP,
		Host:    "benchmark.example.com",
		DstPort: 443,
	}
}

func benchmarkResolveRuntime(tb testing.TB) {
	tb.Helper()

	oldRuntime := loadRuntime()

	configMux.Lock()
	currentRuntime.Store(
		&runtimeSnapshot{
			proxies: map[string]C.Proxy{"DIRECT": directTestProxy{}},
			rules: []C.Rule{
				wrapper.NewRuleWrapper(
					rulesCommon.NewDomain("benchmark.example.com", "DIRECT"),
				),
			},
			mode: Rule,
		},
	)
	configMux.Unlock()

	tb.Cleanup(func() {
		configMux.Lock()
		currentRuntime.Store(oldRuntime)
		invalidateAllProxies()
		configMux.Unlock()
	})
}

func TestResolveMetadataPureDomainRuleStaysAllocationLight(t *testing.T) {
	benchmarkResolveRuntime(t)

	var proxy C.Proxy
	var err error
	allocations := testing.AllocsPerRun(100, func() {
		metadata := benchmarkMetadata()
		proxy, _, err = resolveMetadata(&metadata)
	})
	require.NoError(t, err)
	require.NotNil(t, proxy)
	require.Equal(t, "DIRECT", proxy.Name())
	require.LessOrEqual(t, allocations, float64(1))
}

func BenchmarkResolveMetadata(b *testing.B) {
	benchmarkResolveRuntime(b)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		metadata := benchmarkMetadata()
		proxy, _, err := resolveMetadata(&metadata)
		if err != nil {
			b.Fatal(err)
		}
		if proxy == nil || proxy.Name() != "DIRECT" {
			b.Fatalf("proxy = %v, want DIRECT", proxy)
		}
	}
}

func BenchmarkResolveMetadataFallback(b *testing.B) {
	benchmarkResolveRuntime(b)
	configMux.Lock()
	next := cloneRuntime(loadRuntime())
	next.rules = nil
	currentRuntime.Store(next)
	configMux.Unlock()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		metadata := benchmarkMetadata()
		metadata.Host = fmt.Sprintf("miss-%d.example.com", i)
		proxy, _, err := resolveMetadata(&metadata)
		if err != nil {
			b.Fatal(err)
		}
		if proxy == nil || proxy.Name() != "DIRECT" {
			b.Fatalf("proxy = %v, want DIRECT", proxy)
		}
	}
}
