package mkcp

import (
	"bytes"
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newTestConn() *Conn {
	conn := &Conn{
		state:       stateActive,
		cfg:         Config{},
		flushNotify: make(chan struct{}, 1),
		done:        make(chan struct{}),
	}
	conn.writer = packetWriter{writer: discardWriter{}}
	conn.readNotify = newWaitSignal(&conn.mu)
	conn.writeNotify = newWaitSignal(&conn.mu)
	return conn
}

func TestWriteSegmentReleasesItsPayloadAfterAck(t *testing.T) {
	conn := newTestConn()
	conn.mss = 1200
	conn.remoteRecvWin = 32
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

func TestReadWriteSignalsStayAllocationFree(t *testing.T) {
	conn := newTestConn()

	allocations := testing.AllocsPerRun(1000, func() {
		conn.signalReadLocked()
		conn.signalWriteLocked()
	})
	require.Zero(t, allocations)
}

func TestReadWakesEveryConcurrentWaiter(t *testing.T) {
	conn := newTestConn()
	results := make(chan byte, 2)

	for i := 0; i < 2; i++ {
		go func() {
			buffer := make([]byte, 1)
			_, err := conn.Read(buffer)
			if err != nil {
				return
			}
			results <- buffer[0]
		}()
	}

	time.Sleep(10 * time.Millisecond)
	conn.mu.Lock()
	conn.readBuf = append(conn.readBuf, 'a', 'b')
	conn.signalReadLocked()
	conn.mu.Unlock()

	require.Equal(t, []byte{'a', 'b'}, []byte{<-results, <-results})
}

func TestReadWriteDeadlinesExpire(t *testing.T) {
	readConn := newTestConn()
	require.NoError(t, readConn.SetReadDeadline(time.Now().Add(50*time.Microsecond)))
	_, err := readConn.Read(make([]byte, 1))
	require.ErrorIs(t, err, os.ErrDeadlineExceeded)

	writeConn := newTestConn()
	writeConn.cfg.WriteBuffer = 1350
	writeConn.sendWindow = append(writeConn.sendWindow, &dataSegment{})
	require.NoError(t, writeConn.SetWriteDeadline(time.Now().Add(50*time.Microsecond)))
	_, err = writeConn.Write([]byte{1})
	require.ErrorIs(t, err, os.ErrDeadlineExceeded)
}

func TestReadDeadlineCanBeCleared(t *testing.T) {
	conn := newTestConn()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(time.Second)))
	require.NoError(t, conn.SetReadDeadline(time.Time{}))

	conn.mu.Lock()
	require.True(t, conn.readNotify.deadline.IsZero())
	conn.mu.Unlock()
}

func TestOutOfOrderReceiveSegmentsStayOwnedUntilDelivery(t *testing.T) {
	conn := newTestConn()
	conn.conv = 7
	conn.recvNext = 2
	conn.recvCache = make(map[uint32]*dataSegment)
	reader := newTestPacketReader()
	thirdPacket := buildDataPacketWithNumber(3, "three")
	thirdSegments := reader.read(thirdPacket, nil)
	third := thirdSegments[0].(*dataSegment)
	conn.Input(thirdSegments)
	require.Same(t, third, conn.recvCache[3])
	require.True(t, third.recvOwned)
	require.False(t, third.recvReleased)

	secondPacket := buildDataPacketWithNumber(2, "two")
	secondSegments := reader.read(secondPacket, nil)
	second := secondSegments[0].(*dataSegment)
	conn.Input(secondSegments)
	require.Empty(t, conn.recvCache)
	require.Equal(t, []byte("twothree"), conn.readBuf)
	require.True(t, second.recvReleased)
	require.True(t, third.recvReleased)
}

func TestReadReusesTheReadBufferCapacity(t *testing.T) {
	conn := newTestConn()
	conn.readBuf = append(make([]byte, 0, 64), bytes.Repeat([]byte{7}, 64)...)
	capacity := cap(conn.readBuf)
	output := make([]byte, 64)

	n, err := conn.Read(output)
	require.NoError(t, err)
	require.Equal(t, 64, n)
	require.Equal(t, bytes.Repeat([]byte{7}, 64), output)
	require.Empty(t, conn.readBuf)
	require.Equal(t, capacity, cap(conn.readBuf))
}

func TestFlushControlSegmentsStayAllocationFree(t *testing.T) {
	if raceEnabled {
		t.Skip("allocation counts include race instrumentation")
	}

	conn := newTestConn()
	conn.ackList = []ackItem{
		{number: 1, timestamp: 1},
		{number: 2, timestamp: 2},
	}
	conn.ackDirty = true
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
	conn := newTestConn()
	conn.mss = 1200
	conn.remoteRecvWin = 32
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
