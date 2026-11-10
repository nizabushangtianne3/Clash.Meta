package statistic

import (
	"context"
	"net"
	"net/netip"
	"testing"

	N "github.com/metacubex/mihomo/common/net"
	C "github.com/metacubex/mihomo/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type statisticTestPacketConn struct {
	N.EnhancePacketConn
}

func (c *statisticTestPacketConn) Chains() C.Chain               { return nil }
func (c *statisticTestPacketConn) ProviderChains() C.Chain       { return nil }
func (c *statisticTestPacketConn) AppendToChains(C.ProxyAdapter) {}
func (c *statisticTestPacketConn) RemoteDestination() string     { return "" }
func (c *statisticTestPacketConn) ResolveUDP(context.Context, *C.Metadata) error {
	return nil
}

func newTestTCPTracker(t *testing.T, manager *Manager, pushToManager bool) (*tcpTracker, net.Conn) {
	t.Helper()
	conn, remote := net.Pipe()
	t.Cleanup(func() {
		_ = conn.Close()
		_ = remote.Close()
	})
	tracker := NewTCPTracker(
		newStatisticTestConn(conn),
		manager,
		&C.Metadata{
			NetWork: C.TCP,
			SrcIP:   netip.MustParseAddr("127.0.0.1"),
			DstIP:   netip.MustParseAddr("127.0.0.1"),
			DstPort: 443,
		},
		nil,
		0,
		0,
		pushToManager,
	)
	return tracker, remote
}

func TestTCPTrackerAccountsBothDirections(t *testing.T) {
	manager := newStripedManager()
	tracker, remote := newTestTCPTracker(t, manager, true)

	// the remote writes, the tracker reads: download
	go func() {
		_, _ = remote.Write([]byte("response-bytes"))
	}()
	buf := make([]byte, 64)
	n, err := tracker.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, len("response-bytes"), n)
	assert.Equal(t, int64(n), tracker.Info().DownloadTotal.Load())

	// the tracker writes, the remote reads: upload
	done := make(chan struct{})
	go func() {
		readBuf := make([]byte, 64)
		_, _ = remote.Read(readBuf)
		close(done)
	}()
	n, err = tracker.Write([]byte("request-bytes"))
	require.NoError(t, err)
	<-done
	assert.Equal(t, len("request-bytes"), n)
	assert.Equal(t, int64(n), tracker.Info().UploadTotal.Load())

	// both reach the manager's totals
	up, down := manager.TotalTraffic(false)
	assert.Equal(t, int64(len("request-bytes")), up)
	assert.Equal(t, int64(len("response-bytes")), down)
}

func TestTCPTrackerWithoutManagerPushKeepsItsOwnTotals(t *testing.T) {
	manager := newStripedManager()
	tracker, remote := newTestTCPTracker(t, manager, false)

	go func() {
		_, _ = remote.Write([]byte("response-bytes"))
	}()
	buf := make([]byte, 64)
	_, err := tracker.Read(buf)
	require.NoError(t, err)

	assert.Equal(t, int64(len("response-bytes")), tracker.Info().DownloadTotal.Load())
	up, down := manager.TotalTraffic(false)
	assert.Zero(t, up)
	assert.Zero(t, down)
}

func TestTCPTrackerSnapshotLifecycle(t *testing.T) {
	manager := newStripedManager()
	tracker, _ := newTestTCPTracker(t, manager, true)

	// the constructor joins, the close leaves
	snapshot := manager.Snapshot()
	assert.Len(t, snapshot.Connections, 1)
	assert.Equal(t, tracker.ID(), snapshot.Connections[0].UUID.String())
	assert.NotNil(t, tracker.Upstream())
	assert.Same(t, tracker.Info(), snapshot.Connections[0])

	require.NoError(t, tracker.Close())
	assert.Empty(t, manager.Snapshot().Connections)
}

func TestTCPTrackerUnwrapReturnsPrebuiltCounter(t *testing.T) {
	tracker, _ := newTestTCPTracker(t, newStripedManager(), true)

	firstReader, readerCounts := tracker.UnwrapReader()
	secondReader, readerCountsAgain := tracker.UnwrapReader()
	firstWriter, writerCounts := tracker.UnwrapWriter()
	secondWriter, writerCountsAgain := tracker.UnwrapWriter()
	assert.Same(t, firstReader, secondReader)
	assert.Same(t, firstWriter, secondWriter)
	assert.Same(t, firstReader, &tracker.counter)
	assert.Same(t, firstWriter, &tracker.counter)
	assert.Empty(t, readerCounts)
	assert.Empty(t, readerCountsAgain)
	assert.Empty(t, writerCounts)
	assert.Empty(t, writerCountsAgain)
}

func TestTrackerClonesMetadataIntoCreationSnapshot(t *testing.T) {
	manager := newStripedManager()
	metadata := &C.Metadata{
		NetWork: C.TCP,
		Host:    "original.example",
		DstPort: 443,
	}
	conn, remote := net.Pipe()
	t.Cleanup(func() {
		_ = conn.Close()
		_ = remote.Close()
	})
	tracker := NewTCPTracker(newStatisticTestConn(conn), manager, metadata, nil, 0, 0, false)

	metadata.Host = "mutated.example"
	metadata.SpecialRules = "mutated"

	snapshot := tracker.Info()
	assert.Equal(t, "original.example", snapshot.Metadata.Host)
	assert.Empty(t, snapshot.Metadata.SpecialRules)

	var output []byte
	output = snapshot.AppendJSON(output)
	assert.Contains(t, string(output), `"host":"original.example"`)
	assert.NotContains(t, string(output), `"host":"mutated.example"`)

	// Traffic through a live tracker never rewrites the snapshot it answers
	// from, which is what lets a reader hold one row per connection and skip
	// re-reading a connection whose snapshot pointer has not moved.
	echo := make(chan int, 1)
	go func() {
		buf := make([]byte, 64)
		n, _ := remote.Read(buf)
		echo <- n
	}()
	go func() {
		_, _ = remote.Write([]byte("client-hello"))
	}()
	buf := make([]byte, 64)
	_, err := tracker.Read(buf)
	require.NoError(t, err)
	n, err := tracker.Write([]byte("server-hello"))
	require.NoError(t, err)
	assert.Equal(t, len("server-hello"), n)
	assert.Equal(t, len("server-hello"), <-echo)

	later := tracker.Info().Metadata
	assert.Equal(t, "original.example", later.Host)
	assert.Empty(t, later.SpecialRules)
	assert.Equal(t, metadata.NetWork, later.NetWork)
	assert.Equal(t, metadata.DstPort, later.DstPort)
	assert.Same(t, snapshot.Metadata, tracker.Info().Metadata)
}
func TestUDPTrackerAccountsPackets(t *testing.T) {
	manager := newStripedManager()
	server, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = server.Close() })
	client, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	tracker := NewUDPTracker(
		&statisticTestPacketConn{EnhancePacketConn: N.NewEnhancePacketConn(client)},
		manager,
		&C.Metadata{NetWork: C.UDP, DstIP: netip.MustParseAddr("127.0.0.1"), DstPort: 53},
		nil,
		0,
		0,
		true,
	)

	// upload: the tracker writes to the server
	readDone := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 64)
		n, _, _ := server.ReadFrom(buf)
		readDone <- buf[:n]
	}()
	n, err := tracker.WriteTo([]byte("dns-query"), server.LocalAddr())
	require.NoError(t, err)
	assert.Equal(t, len("dns-query"), n)
	assert.Equal(t, int64(n), tracker.Info().UploadTotal.Load())
	assert.Equal(t, []byte("dns-query"), <-readDone)

	// download: the server answers and the tracker reads it back
	go func() {
		_, _ = server.WriteTo([]byte("dns-answer"), client.LocalAddr())
	}()
	buf := make([]byte, 64)
	n, _, err = tracker.ReadFrom(buf)
	require.NoError(t, err)
	assert.Equal(t, len("dns-answer"), n)
	assert.Equal(t, int64(len("dns-answer")), tracker.Info().DownloadTotal.Load())

	// both land in the manager totals and the snapshot
	up, down := manager.TotalTraffic(false)
	assert.Equal(t, int64(len("dns-query")), up)
	assert.Equal(t, int64(len("dns-answer")), down)
	assert.Len(t, manager.Snapshot().Connections, 1)

	require.NoError(t, tracker.Close())
	assert.Empty(t, manager.Snapshot().Connections)
}

func TestNowTrafficReportsTheBlips(t *testing.T) {
	manager := newStripedManager()
	up, down := manager.NowTraffic(false)
	assert.Zero(t, up)
	assert.Zero(t, down)

	tracker, remote := newTestTCPTracker(t, manager, true)
	go func() {
		_, _ = remote.Write([]byte("response-bytes"))
	}()
	buf := make([]byte, 64)
	_, err := tracker.Read(buf)
	require.NoError(t, err)

	// the blips move on the manager's one-second tick, so the totals carry the
	// traffic immediately while Now stays at zero until the tick
	up, down = manager.NowTraffic(false)
	assert.Zero(t, up)
	assert.Zero(t, down)
	totalUp, totalDown := manager.TotalTraffic(false)
	assert.Zero(t, totalUp)
	assert.Equal(t, int64(len("response-bytes")), totalDown)
}

func TestManagerMemoryReadsThePlatformModule(t *testing.T) {
	manager := newStripedManager()
	memory := manager.Memory()
	assert.GreaterOrEqual(t, memory, uint64(0))
	// a second read is stable enough to be the same process
	assert.Equal(t, memory, manager.Memory())
}

type mutableChainPacketConn struct {
	C.PacketConn
	chain C.Chain
}

func (c *mutableChainPacketConn) Chains() C.Chain           { return c.chain }
func (c *mutableChainPacketConn) ProviderChains() C.Chain   { return nil }
func (c *mutableChainPacketConn) RemoteDestination() string { return "" }

func TestUDPTrackerChainsStayAppendIsolated(t *testing.T) {
	conn := &mutableChainPacketConn{chain: C.Chain{"DIRECT"}}
	tracker := NewUDPTracker(
		conn,
		newStripedManager(),
		&C.Metadata{NetWork: C.UDP, DstPort: 53},
		nil,
		0,
		0,
		false,
	)

	conn.chain = append(conn.chain, "mutated")
	assert.Equal(t, C.Chain{"DIRECT"}, tracker.TrackerInfo.Chain)
	assert.Equal(t, len(tracker.TrackerInfo.Chain), cap(tracker.TrackerInfo.Chain))
}
