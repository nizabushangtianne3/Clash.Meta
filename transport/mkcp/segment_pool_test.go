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
		flushed = conn.flushSendLocked(0, conn.flushBatch[:0])
		conn.mu.Unlock()
		conn.writeSegments(flushed)

		ack := conn.sendNext - 1
		conn.mu.Lock()
		require.True(t, conn.processAckLocked(ack))
		conn.mu.Unlock()
		conn.releaseFlushSegments(flushed)
		clear(flushed)
		conn.flushBatch = flushed[:0]
	})
	require.Empty(t, conn.sendWindow)
	require.Zero(t, allocations)
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

func TestAckDoesNotSignalWriteWaitersWhenNoneExist(t *testing.T) {
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
	conn.mu.Lock()
	require.True(t, conn.processAckLocked(conn.sendNext-1))
	conn.mu.Unlock()
	require.True(t, notify == conn.writeNotify)
}

func TestWriteReusesRetiredSegments(t *testing.T) {
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

	_, err := conn.Write(payload)
	require.NoError(t, err)
	first := conn.sendWindow[0]
	conn.mu.Lock()
	flushed := conn.flushSendLocked(0, nil)
	conn.mu.Unlock()
	conn.writeSegments(flushed)
	conn.mu.Lock()
	require.True(t, conn.processAckLocked(conn.sendNext-1))
	conn.mu.Unlock()
	conn.releaseFlushSegments(flushed)

	_, err = conn.Write(payload)
	require.NoError(t, err)
	require.True(t, first == conn.sendWindow[0])
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
		flushed := conn.flushSendLocked(0, conn.flushBatch[:0])
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
		clear(flushed)
		conn.flushBatch = flushed[:0]
	}
}
