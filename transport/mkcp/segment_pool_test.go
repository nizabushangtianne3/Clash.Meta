package mkcp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWriteSegmentReleasesItsPayloadAfterAck(t *testing.T) {
	conn := &Conn{
		state:         stateActive,
		mss:           1200,
		remoteRecvWin: 32,
		readNotify:    make(chan struct{}),
		writeNotify:   make(chan struct{}),
		flushNotify:   make(chan struct{}, 1),
		done:          make(chan struct{}),
	}
	conn.writer = packetWriter{writer: discardWriter{}}
	payload := make([]byte, conn.mss)

	var flushed []segment
	allocations := testing.AllocsPerRun(100, func() {
		_, err := conn.Write(payload)
		require.NoError(t, err)

		conn.mu.Lock()
		flushed = conn.flushSendLocked(0)
		conn.mu.Unlock()
		conn.writeSegments(flushed)

		ack := conn.sendNext - 1
		conn.mu.Lock()
		require.True(t, conn.processAckLocked(ack))
		conn.mu.Unlock()
		conn.releaseFlushSegments(flushed)
	})
	require.Empty(t, conn.sendWindow)
	last, ok := flushed[0].(*dataSegment)
	require.True(t, ok)
	require.Nil(t, last.payload)
	require.LessOrEqual(t, int(allocations), 3)
}

func TestWriteDoesNotSignalWriteWaitersWhenQueueing(t *testing.T) {
	conn := &Conn{
		state:         stateActive,
		mss:           1200,
		remoteRecvWin: 32,
		readNotify:    make(chan struct{}),
		writeNotify:   make(chan struct{}),
		flushNotify:   make(chan struct{}, 1),
		done:          make(chan struct{}),
	}
	conn.writer = packetWriter{writer: discardWriter{}}
	notify := conn.writeNotify

	_, err := conn.Write(make([]byte, conn.mss))
	require.NoError(t, err)
	require.True(t, notify == conn.writeNotify)
}

func BenchmarkWriteSegmentPayloadLifecycle(b *testing.B) {
	conn := &Conn{
		state:         stateActive,
		mss:           1200,
		remoteRecvWin: 32,
		readNotify:    make(chan struct{}),
		writeNotify:   make(chan struct{}),
		flushNotify:   make(chan struct{}, 1),
		done:          make(chan struct{}),
	}
	conn.writer = packetWriter{writer: discardWriter{}}
	payload := make([]byte, conn.mss)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := conn.Write(payload); err != nil {
			b.Fatal(err)
		}
		conn.mu.Lock()
		flushed := conn.flushSendLocked(0)
		conn.mu.Unlock()
		conn.writeSegments(flushed)
		ack := conn.sendNext - 1
		conn.mu.Lock()
		if !conn.processAckLocked(ack) {
			conn.mu.Unlock()
			b.Fatal("ack missed")
		}
		conn.mu.Unlock()
		conn.releaseFlushSegments(flushed)
	}
}
