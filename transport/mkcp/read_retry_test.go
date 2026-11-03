package mkcp

import (
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type transientReadConn struct {
	reads   atomic.Int32
	release chan struct{}
	close   sync.Once
}

func (c *transientReadConn) Read([]byte) (int, error) {
	if c.reads.Add(1) == 1 {
		return 0, errors.New("mkcp: transient read failure")
	}
	<-c.release
	return 0, net.ErrClosed
}

func (*transientReadConn) Write(b []byte) (int, error) { return len(b), nil }

func (c *transientReadConn) Close() error {
	c.close.Do(func() { close(c.release) })
	return nil
}

func (*transientReadConn) LocalAddr() net.Addr              { return &net.UDPAddr{} }
func (*transientReadConn) RemoteAddr() net.Addr             { return &net.UDPAddr{} }
func (*transientReadConn) SetDeadline(time.Time) error      { return nil }
func (*transientReadConn) SetReadDeadline(time.Time) error  { return nil }
func (*transientReadConn) SetWriteDeadline(time.Time) error { return nil }

func TestDialRetriesTransientReadErrors(t *testing.T) {
	raw := &transientReadConn{release: make(chan struct{})}
	conn, err := Dial(t.Context(), raw, Config{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	require.Eventually(t, func() bool { return raw.reads.Load() >= 2 }, time.Second, time.Millisecond)
}
