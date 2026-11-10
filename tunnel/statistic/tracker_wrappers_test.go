package statistic

import (
	"net"
	"net/netip"
	"testing"

	"github.com/metacubex/mihomo/common/buf"
	N "github.com/metacubex/mihomo/common/net"
	C "github.com/metacubex/mihomo/constant"
	"github.com/stretchr/testify/require"
)

// The counter the tracker hands out is what a reader-replace mechanism sees,
// and it must keep accounting while refusing to be replaced.
func TestTCPTrackerCounterKeepsAccountingAndRefusesReplacement(t *testing.T) {
	manager := newStripedManager()
	tracker, remote := newTestTCPTracker(t, manager, true)

	reader, _ := tracker.UnwrapReader()
	counter, ok := reader.(*tcpCounterConn)
	require.True(t, ok, "the tracker hands out its counter, not the raw conn")
	require.False(t, counter.ReaderReplaceable())
	require.False(t, counter.WriterReplaceable())

	go func() { _, _ = remote.Write([]byte("counted-bytes")) }()
	readBuf := make([]byte, 64)
	n, err := counter.Read(readBuf)
	require.NoError(t, err)
	require.Equal(t, len("counted-bytes"), n)
	require.Equal(t, int64(n), tracker.Info().DownloadTotal.Load(),
		"a read through the counter still lands in the tracker")

	readDone := make(chan struct{})
	go func() {
		readBuf := make([]byte, 64)
		_, _ = remote.Read(readBuf)
		close(readDone)
	}()
	n, err = counter.Write([]byte("counted-bytes"))
	require.NoError(t, err)
	<-readDone
	require.Equal(t, len("counted-bytes"), n)
	require.Equal(t, int64(n), tracker.Info().UploadTotal.Load())

	go func() { _, _ = remote.Write([]byte("buffered-bytes")) }()
	readBuffer := buf.New()
	require.NoError(t, counter.ReadBuffer(readBuffer))
	require.Equal(t, int64(len("counted-bytes"))+int64(len("buffered-bytes")),
		tracker.Info().DownloadTotal.Load())
	readBuffer.Release()

	writeBuffer := buf.New()
	writeBuffer.Write([]byte("buffered-bytes"))
	before := tracker.Info().UploadTotal.Load()
	go func() {
		drain := make([]byte, 64)
		_, _ = remote.Read(drain)
	}()
	require.NoError(t, counter.WriteBuffer(writeBuffer))
	require.Equal(t, before+int64(len("buffered-bytes")), tracker.Info().UploadTotal.Load())
	writeBuffer.Release()
}

// A UDP tracker answers the id it carries, the conn it wraps, and a packet it
// was asked to wait for.
func TestUDPTrackerAnswersItsIdItsConnAndAWait(t *testing.T) {
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

	require.Equal(t, tracker.UUID.String(), tracker.ID())
	require.Equal(t, client.LocalAddr(), tracker.Upstream().(C.PacketConn).LocalAddr())

	go func() { _, _ = server.WriteTo([]byte("waited-bytes"), client.LocalAddr()) }()
	data, _, err := tracker.WaitReadFrom()
	require.NoError(t, err)
	require.Equal(t, []byte("waited-bytes"), data)
	require.Equal(t, int64(len(data)), tracker.Info().DownloadTotal.Load())
}
