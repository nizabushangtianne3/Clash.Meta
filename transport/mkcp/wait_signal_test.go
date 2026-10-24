package mkcp

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newWaitSignalTestConn() *Conn {
	conn := &Conn{
		state:       stateActive,
		cfg:         Config{},
		flushNotify: make(chan struct{}, 1),
		done:        make(chan struct{}),
	}
	conn.readNotify = newWaitSignal(&conn.mu)
	conn.writeNotify = newWaitSignal(&conn.mu)
	return conn
}

func TestReadWriteSignalsStayAllocationFree(t *testing.T) {
	if raceEnabled {
		t.Skip("allocation counts include race instrumentation")
	}

	conn := newWaitSignalTestConn()
	conn.mu.Lock()
	conn.readWaiters = 1
	conn.writeWaiters = 1

	allocations := testing.AllocsPerRun(1000, func() {
		conn.signalReadLocked()
		conn.signalWriteLocked()
	})

	conn.mu.Unlock()
	require.Zero(t, allocations)
}

func TestReadSignalWakesEveryConcurrentWaiter(t *testing.T) {
	conn := newWaitSignalTestConn()
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
	for {
		conn.mu.Lock()
		waiting := conn.readWaiters
		conn.mu.Unlock()
		if waiting == 2 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	conn.mu.Lock()
	conn.readBuf = append(conn.readBuf, 'a', 'b')
	conn.signalReadLocked()
	conn.mu.Unlock()

	require.ElementsMatch(t, []byte{'a', 'b'}, []byte{<-results, <-results})
}

func TestReadWriteDeadlinesExpire(t *testing.T) {
	readConn := newWaitSignalTestConn()
	require.NoError(t, readConn.SetReadDeadline(time.Now().Add(50*time.Microsecond)))
	_, err := readConn.Read(make([]byte, 1))
	require.ErrorIs(t, err, os.ErrDeadlineExceeded)

	writeConn := newWaitSignalTestConn()
	writeConn.mss = 1
	writeConn.cfg.WriteBuffer = 1350
	writeConn.sendWindow = append(writeConn.sendWindow, &dataSegment{}, &dataSegment{})
	require.NoError(t, writeConn.SetWriteDeadline(time.Now().Add(50*time.Microsecond)))
	_, err = writeConn.Write([]byte{1})
	require.ErrorIs(t, err, os.ErrDeadlineExceeded)
}

func TestReadDeadlineCanBeCleared(t *testing.T) {
	conn := newWaitSignalTestConn()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(time.Second)))
	require.NoError(t, conn.SetReadDeadline(time.Time{}))

	conn.mu.Lock()
	require.True(t, conn.readNotify.deadline.IsZero())
	conn.mu.Unlock()
}
