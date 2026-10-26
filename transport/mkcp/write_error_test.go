package mkcp

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type failWriter struct {
	attempts int
}

func (w *failWriter) Write([]byte) (int, error) {
	w.attempts++
	return 0, errors.New("mkcp: write failed")
}

// A write that fails must not be retried inside the same flush: the segment is
// still in the send window with its own timeout, and burning another syscall on
// a socket that just refused one only spends the flush's time budget.
func TestWriteSegmentsStopsAtTheFirstFailure(t *testing.T) {
	writer := &failWriter{}
	conn := &Conn{writer: packetWriter{writer: writer}}

	segments := []segment{&cmdOnlySegment{}, &cmdOnlySegment{}}
	start := time.Now()
	err := conn.writeSegments(segments)
	elapsed := time.Since(start)

	require.Error(t, err)
	require.Equal(t, 1, writer.attempts)
	require.Less(t, elapsed, 100*time.Millisecond)
}

// The failure has to end the connection. Without that the peer waits out its
// own deadline for segments that can never arrive, which is a sixty second
// stall under a client timeout rather than an error.
func TestSendSegmentsEndsTheConnectionOnWriteFailure(t *testing.T) {
	writer := &failWriter{}
	conn := &Conn{
		writer:      packetWriter{writer: writer},
		recvCache:   make(map[uint32]*dataSegment),
		flushNotify: make(chan struct{}, 1),
		done:        make(chan struct{}),
		state:       stateActive,
	}
	conn.readNotify = newWaitSignal(&conn.mu)
	conn.writeNotify = newWaitSignal(&conn.mu)

	conn.sendSegments([]segment{&cmdOnlySegment{}}, false)

	conn.mu.Lock()
	state := conn.state
	conn.mu.Unlock()
	require.Equal(t, stateTerminated, state)

	// A reader on the dead connection is released instead of blocking.
	released := make(chan struct{})
	go func() {
		_, _ = conn.Read(make([]byte, 16))
		close(released)
	}()
	select {
	case <-released:
	case <-time.After(5 * time.Second):
		t.Fatal("Read on a dead connection never returned")
	}
}

// An explicitly requested teardown still terminates, which is what the flush
// branch that sent the final segment relied on before.
func TestSendSegmentsTerminatesWhenAsked(t *testing.T) {
	conn := &Conn{
		writer:      packetWriter{writer: discardWriter{}},
		recvCache:   make(map[uint32]*dataSegment),
		flushNotify: make(chan struct{}, 1),
		done:        make(chan struct{}),
		state:       stateActive,
	}
	conn.readNotify = newWaitSignal(&conn.mu)
	conn.writeNotify = newWaitSignal(&conn.mu)

	conn.sendSegments([]segment{&cmdOnlySegment{}}, true)

	conn.mu.Lock()
	state := conn.state
	conn.mu.Unlock()
	require.Equal(t, stateTerminated, state)
}
