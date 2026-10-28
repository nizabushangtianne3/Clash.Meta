package mekya

import (
	"context"
	"testing"

	"github.com/metacubex/mihomo/common/pool"
	"github.com/stretchr/testify/require"
)

func TestWrappedPacketConnForwardsAnOwnedPacketWithoutAllocating(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn := newWrappedPacketConn(ctx, nil)
	session := &serverSession{ctx: ctx, readerChan: make(chan []byte, 1)}
	require.NoError(t, conn.addSession(session))

	payload := pool.Get(len("datagram!"))
	copy(payload, "datagram!")
	out := make([]byte, len(payload))
	session.readerChan <- payload
	n, addr, err := conn.ReadFrom(out)
	require.NoError(t, err)
	require.Equal(t, len(payload), n)
	require.Same(t, session, addr)

	allocations := testing.AllocsPerRun(100, func() {
		ownedPacket := pool.Get(len(payload))
		session.readerChan <- ownedPacket
		n, _, err = conn.ReadFrom(out)
	})
	require.NoError(t, err)
	require.Equal(t, len(payload), n)
	require.Zero(t, allocations)
}

func TestWrappedPacketConnKeepsTheReadLimit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn := newWrappedPacketConn(ctx, nil)
	session := &serverSession{ctx: ctx, readerChan: make(chan []byte, 1)}
	require.NoError(t, conn.addSession(session))

	payload := make([]byte, 2001)
	out := make([]byte, len(payload))
	session.readerChan <- payload
	n, _, err := conn.ReadFrom(out)
	require.NoError(t, err)
	require.Equal(t, 2000, n)
}

func TestServerSessionWriteQueuesAnIndependentPacketWithoutAllocating(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	session := &serverSession{ctx: ctx, writerChan: make(chan []byte, 1)}
	payload := []byte("response")
	n, err := session.Write(payload)
	require.NoError(t, err)
	require.Equal(t, len(payload), n)

	queued := <-session.writerChan
	require.Equal(t, payload, queued)
	payload[0] = 'R'
	require.Equal(t, []byte("response"), queued)

	queuedLen := 0
	allocations := testing.AllocsPerRun(100, func() {
		n, err = session.Write(payload)
		queued = <-session.writerChan
		queuedLen = len(queued)
		_ = pool.Put(queued)
	})
	require.NoError(t, err)
	require.Equal(t, len(payload), n)
	require.Equal(t, len(payload), queuedLen)
	require.Zero(t, allocations)
}

func BenchmarkWrappedPacketConnForward(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn := newWrappedPacketConn(ctx, nil)
	session := &serverSession{ctx: ctx, readerChan: make(chan []byte, 1)}
	if err := conn.addSession(session); err != nil {
		b.Fatal(err)
	}
	payload := make([]byte, 1200)
	out := make([]byte, len(payload))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		session.readerChan <- payload
		if _, _, err := conn.ReadFrom(out); err != nil {
			b.Fatal(err)
		}
	}
}

func TestRequestClientSessionWriteQueuesAnIndependentPacketWithoutAllocating(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	session := &requestClientSession{ctx: ctx, writerChan: make(chan []byte, 1)}
	payload := []byte("request")
	n, err := session.Write(payload)
	require.NoError(t, err)
	require.Equal(t, len(payload), n)

	queued := <-session.writerChan
	require.Equal(t, payload, queued)
	payload[0] = 'R'
	require.Equal(t, []byte("request"), queued)

	queuedLen := 0
	allocations := testing.AllocsPerRun(100, func() {
		n, err = session.Write(payload)
		queued = <-session.writerChan
		queuedLen = len(queued)
		if cap(queued)&(cap(queued)-1) == 0 {
			_ = pool.Put(queued)
		}
	})
	require.NoError(t, err)
	require.Equal(t, len(payload), n)
	require.Equal(t, len(payload), queuedLen)
	require.Zero(t, allocations)
}

func BenchmarkServerSessionWrite(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	session := &serverSession{ctx: ctx, writerChan: make(chan []byte, 1)}
	payload := make([]byte, 1200)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := session.Write(payload); err != nil {
			b.Fatal(err)
		}
		queued := <-session.writerChan
		if cap(queued)&(cap(queued)-1) == 0 {
			_ = pool.Put(queued)
		}
	}
}

func BenchmarkRequestClientSessionWrite(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	session := &requestClientSession{ctx: ctx, writerChan: make(chan []byte, 1)}
	payload := make([]byte, 1200)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := session.Write(payload); err != nil {
			b.Fatal(err)
		}
		queued := <-session.writerChan
		if cap(queued)&(cap(queued)-1) == 0 {
			_ = pool.Put(queued)
		}
	}
}
