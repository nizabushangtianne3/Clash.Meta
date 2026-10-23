package mkcp

import (
	"net"
	"testing"
	"time"

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

func TestReadSignalDoesNotReplaceTheChannelWithoutWaiters(t *testing.T) {
	conn := &Conn{
		state:       stateActive,
		readNotify:  make(chan struct{}),
		writeNotify: make(chan struct{}),
		flushNotify: make(chan struct{}, 1),
		done:        make(chan struct{}),
	}
	notify := conn.readNotify

	allocations := testing.AllocsPerRun(1000, conn.signalReadLocked)
	conn.signalReadLocked()
	require.True(t, notify == conn.readNotify)
	require.Zero(t, allocations)
}

func TestWaitNotifyWithDeadlineStaysAllocationFree(t *testing.T) {
	notify := make(chan struct{})
	close(notify)
	deadline := time.Now().Add(time.Minute)

	allocations := testing.AllocsPerRun(1000, func() {
		require.NoError(t, waitNotify(notify, deadline))
	})
	require.Zero(t, allocations)
}

func TestOutOfOrderReceiveSegmentsStayOwnedUntilDelivery(t *testing.T) {
	conn := &Conn{
		state:       stateActive,
		cfg:         Config{},
		conv:        7,
		recvNext:    2,
		recvCache:   make(map[uint32]*dataSegment),
		readNotify:  make(chan struct{}),
		writeNotify: make(chan struct{}),
		flushNotify: make(chan struct{}, 1),
		done:        make(chan struct{}),
	}
	third := &dataSegment{conv: 7, number: 3, payload: []byte("three")}
	conn.Input([]segment{third})
	require.Same(t, third, conn.recvCache[3])
	require.True(t, third.recvOwned)
	require.False(t, third.recvReleased)

	second := &dataSegment{conv: 7, number: 2, payload: []byte("two")}
	conn.Input([]segment{second})
	require.Empty(t, conn.recvCache)
	require.Equal(t, []byte("twothree"), conn.readBuf)
	require.True(t, second.recvReleased)
	require.True(t, third.recvReleased)
}

func TestFlushControlSegmentsStayAllocationFree(t *testing.T) {
	if raceEnabled {
		t.Skip("allocation counts include race instrumentation")
	}

	conn := &Conn{
		state:       stateActive,
		cfg:         Config{},
		readNotify:  make(chan struct{}),
		writeNotify: make(chan struct{}),
		flushNotify: make(chan struct{}, 1),
		done:        make(chan struct{}),
		ackList: []ackItem{
			{number: 1, timestamp: 1},
			{number: 2, timestamp: 2},
		},
		ackDirty: true,
	}
	conn.writer = packetWriter{writer: discardWriter{}}
	var flushed []segment

	allocations := testing.AllocsPerRun(1000, func() {
		conn.ackList[0].nextFlush = 0
		conn.ackList[1].nextFlush = 0
		conn.ackDirty = true
		conn.mu.Lock()
		flushed = conn.flushAcksLocked(1, conn.flushBatch[:0])
		flushed = append(flushed, conn.pingSegmentLocked(commandPing, 1))
		flushed = conn.flushSendLocked(1, flushed)
		conn.mu.Unlock()
		conn.writeSegments(flushed)
		conn.releaseFlushSegments(flushed)
		clear(flushed)
		conn.flushBatch = flushed[:0]
	})
	require.Zero(t, allocations)
}

func TestUDPPacketWriteStaysAllocationFree(t *testing.T) {
	receiver, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer receiver.Close()
	go func() {
		buffer := make([]byte, 1500)
		for {
			if _, _, err := receiver.ReadFromUDP(buffer); err != nil {
				return
			}
		}
	}()

	sender, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer sender.Close()
	writer := &udpPacketConnWriter{
		udp:  sender,
		addr: receiver.LocalAddr().(*net.UDPAddr).AddrPort(),
	}
	payload := make([]byte, 128)

	allocations := testing.AllocsPerRun(1000, func() {
		if _, err := writer.Write(payload); err != nil {
			t.Fatal(err)
		}
	})
	require.Zero(t, allocations)
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
