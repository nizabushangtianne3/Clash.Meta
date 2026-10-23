package mkcp

import (
	"net"
	"net/netip"
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

func TestPacketConnWriteStaysAllocationFree(t *testing.T) {
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
	writer := &packetConnWriter{pc: sender, addr: receiver.LocalAddr()}
	payload := make([]byte, 128)

	allocations := testing.AllocsPerRun(1000, func() {
		if _, err := writer.Write(payload); err != nil {
			t.Fatal(err)
		}
	})
	require.Zero(t, allocations)
}

func TestSessionAddressCanonicalizesUDPWithoutString(t *testing.T) {
	first := &net.UDPAddr{IP: net.IPv4(1, 2, 3, 4), Port: 53}
	second := &net.UDPAddr{IP: net.IPv4(1, 2, 3, 4), Port: 53}
	var address sessionAddressKey

	allocations := testing.AllocsPerRun(1000, func() {
		address = sessionAddress(first)
	})
	require.Equal(t, sessionAddress(second), address)
	require.Equal(t, netip.MustParseAddrPort("1.2.3.4:53"), address.udp)
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
