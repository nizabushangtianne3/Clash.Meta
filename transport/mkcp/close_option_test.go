package mkcp

import (
	"sync"
	"testing"
)

func TestCloseOptionDoesNotDropAckedSendSegments(t *testing.T) {
	conn := &Conn{
		state:             stateReadyToClose,
		recvCache:         make(map[uint32]*dataSegment),
		firstUnacked:      0,
		sendNext:          1,
		remoteRecvWin:     32,
		totalInFlightSize: 1,
		flushNotify:       make(chan struct{}, 1),
		done:              make(chan struct{}),
	}
	conn.readNotify = newWaitSignal(&conn.mu)
	conn.writeNotify = newWaitSignal(&conn.mu)
	segmentPool := sync.Pool{New: func() any { return new(dataSegment) }}
	seg := segmentPool.Get().(*dataSegment)
	seg.number = 0
	conn.sendWindow = []*dataSegment{seg}

	ack := &ackSegment{
		conv:          conn.conv,
		option:        segmentOptionClose,
		receivingNext: 1,
		numberList:    []uint32{0},
	}
	conn.Input([]segment{ack})

	if len(conn.sendWindow) != 0 {
		t.Fatalf("close ACK was dropped: %d segments remain", len(conn.sendWindow))
	}
	if conn.state != stateTerminating {
		t.Fatalf("state = %d, want terminating", conn.state)
	}
}

func TestPeerTerminatingKeepsCachedOutOfOrderRecords(t *testing.T) {
	conn := &Conn{
		state:     stateActive,
		recvCache: make(map[uint32]*dataSegment),
	}
	bufferPool := sync.Pool{New: func() any { return new(receiveBuffer) }}
	dataPool := sync.Pool{New: func() any { return new(dataSegment) }}
	buffer := &receiveBuffer{data: make([]byte, 4), owner: &bufferPool}
	cached := &dataSegment{
		number:     1,
		recvOwned:  true,
		recvBuffer: buffer,
		recvPool:   &dataPool,
	}
	conn.recvCache[cached.number] = cached

	conn.setStateLocked(statePeerTerminating)
	if conn.recvCache[cached.number] != cached {
		t.Fatal("peer termination discarded a record pending delivery")
	}
	if !cached.recvOwned {
		t.Fatal("peer termination must leave the record owned for delivery")
	}

	conn.setStateLocked(stateTerminated)
	if _, remains := conn.recvCache[cached.number]; remains {
		t.Fatal("final termination must release the cache")
	}
	if cached.recvOwned || cached.recvBuffer.data != nil || cached.recvBuffer.owner != nil {
		t.Fatal("final termination must release the cached receive buffer")
	}
}

func TestControlSegmentPoolsDoNotRetainCloseOptions(t *testing.T) {
	conn := &Conn{
		state:         stateActive,
		recvCache:     make(map[uint32]*dataSegment),
		remoteRecvWin: 32,
	}

	staleAck := sendAckSegmentPool.Get().(*ackSegment)
	staleAck.option = segmentOptionClose
	sendAckSegmentPool.Put(staleAck)
	if ack := conn.newAckSegmentLocked(); ack.option == segmentOptionClose {
		t.Fatal("pooled ACK retained segmentOptionClose")
	}

	staleCmd := sendCmdSegmentPool.Get().(*cmdOnlySegment)
	staleCmd.option = segmentOptionClose
	sendCmdSegmentPool.Put(staleCmd)
	if ping := conn.pingSegmentLocked(commandPing, 0); ping.option == segmentOptionClose {
		t.Fatal("pooled ping retained segmentOptionClose")
	}
}
