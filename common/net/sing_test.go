package net

import (
	"bytes"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/metacubex/sing/common/buf"
	"github.com/stretchr/testify/require"
)

// streamConn is one end of an in-memory stream. It fills the buffer it is
// handed without allocating, so an allocation seen while a relay runs belongs
// to the relay and not to the fixture. Its ReadBuffer appends, exactly like
// the reader wrappers the relay meets under a real connection.
type streamConn struct {
	in      *bytes.Reader
	out     *bytes.Buffer
	written atomic.Int64
	once    sync.Once
	done    chan struct{}
}

func newStreamConn(in *bytes.Reader, out *bytes.Buffer) *streamConn {
	return &streamConn{in: in, out: out, done: make(chan struct{})}
}

func (c *streamConn) Read(p []byte) (int, error)  { return c.in.Read(p) }
func (c *streamConn) Write(p []byte) (int, error) { return c.out.Write(p) }

func (c *streamConn) Close() error {
	c.once.Do(func() { close(c.done) })
	return nil
}

func (c *streamConn) LocalAddr() net.Addr              { return dummyAddr{} }
func (c *streamConn) RemoteAddr() net.Addr             { return dummyAddr{} }
func (c *streamConn) SetDeadline(time.Time) error      { return nil }
func (c *streamConn) SetReadDeadline(time.Time) error  { return nil }
func (c *streamConn) SetWriteDeadline(time.Time) error { return nil }

func (c *streamConn) ReadBuffer(buffer *buf.Buffer) error {
	_, err := buffer.ReadOnceFrom(c.in)
	return err
}

func (c *streamConn) WriteBuffer(buffer *buf.Buffer) error {
	_, err := c.out.Write(buffer.Bytes())
	c.written.Add(int64(buffer.Len()))
	return err
}

type dummyAddr struct{}

func (dummyAddr) Network() string { return "stream" }
func (dummyAddr) String() string  { return "stream" }

// A relay holds one buffer per direction, so a stream longer than that buffer
// must arrive whole and in order: a chunk that did not get reset would stack
// behind the previous one and show up here as duplicated bytes.
func TestRelayStreamsEveryChunkBothWays(t *testing.T) {
	leftPayload := bytes.Repeat([]byte("left-to-right:"), 20000)
	rightPayload := bytes.Repeat([]byte("right-to-left:"), 20000)

	left := newStreamConn(bytes.NewReader(rightPayload), &bytes.Buffer{})
	right := newStreamConn(bytes.NewReader(leftPayload), &bytes.Buffer{})

	go Relay(left, right)

	<-left.done
	<-right.done

	require.Equal(t, leftPayload, left.out.Bytes(), "the right payload crossed altered")
	require.Equal(t, rightPayload, right.out.Bytes(), "the left payload crossed altered")
}

// After the first chunk each direction's buffer is in place, so a steady
// stream costs the relay nothing per chunk.
func TestRelaySteadyStateCostsNothingPerChunk(t *testing.T) {
	// more than the chunks the run below asks for, so the stream never runs dry
	payload := bytes.Repeat([]byte("chunk"), 8<<20)
	left := newStreamConn(bytes.NewReader(payload), &bytes.Buffer{})
	right := newStreamConn(bytes.NewReader(nil), &bytes.Buffer{})

	go Relay(left, right)

	for right.written.Load() < buf.BufferSize {
		time.Sleep(0)
	}

	const chunks = 100
	const chunkSize = 64 << 10
	before := right.written.Load()
	allocs := testing.AllocsPerRun(chunks, func() {
		for right.written.Load() < before+chunkSize {
			time.Sleep(0)
		}
		before += chunkSize
	})
	require.Zero(t, allocs, "a steady chunk cost the relay an allocation")
}

// A conn that carries bytes read before the relay started hands them over
// first: its ReadBuffer skips them, and a fallback path depends on the
// recorded flight arriving at the far side before anything from the wire.
func TestRelayWritesTheCachedPrefixFirst(t *testing.T) {
	rest := bytes.Repeat([]byte("wire"), 1000)
	leftOut := &bytes.Buffer{}
	left := newStreamConn(bytes.NewReader(nil), leftOut)
	right := NewCachedConn(newStreamConn(bytes.NewReader(rest), &bytes.Buffer{}), []byte("recorded-flight"))

	go Relay(left, right)

	<-left.done
	require.Equal(t, append([]byte("recorded-flight"), rest...), leftOut.Bytes(),
		"the recorded flight must lead the stream")
}

func BenchmarkRelayStream(b *testing.B) {
	payload := bytes.Repeat([]byte("x"), 4<<20)
	// pre-sized and reused: a growing buffer would dwarf the relay's own cost
	out := &bytes.Buffer{}
	out.Grow(len(payload))
	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out.Reset()
		left := newStreamConn(bytes.NewReader(payload), &bytes.Buffer{})
		right := newStreamConn(bytes.NewReader(nil), out)
		go Relay(left, right)
		<-right.done
	}
}
