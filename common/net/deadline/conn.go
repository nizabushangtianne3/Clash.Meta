package deadline

import (
	"net"
	"os"
	"sync"
	"time"

	"github.com/metacubex/mihomo/common/atomic"
	"github.com/metacubex/mihomo/common/pool"

	"github.com/metacubex/sing/common/buf"
	"github.com/metacubex/sing/common/bufio"
	"github.com/metacubex/sing/common/network"
)

type connReadResult struct {
	data   []byte
	unread []byte
	err    error
}

var connReadResultPool = sync.Pool{New: func() any { return new(connReadResult) }}

func (r *connReadResult) release() {
	if r.data != nil {
		pool.Put(r.data)
	}
	r.data = nil
	r.unread = nil
	r.err = nil
	connReadResultPool.Put(r)
}

type Conn struct {
	network.ExtendedConn
	deadline     atomic.TypedValue[time.Time]
	pipeDeadline PipeDeadline
	disablePipe  atomic.Bool
	inRead       atomic.Bool
	resultCh     chan *connReadResult
}

func IsConn(conn any) bool {
	_, ok := conn.(*Conn)
	return ok
}

func NewConn(conn net.Conn) *Conn {
	c := &Conn{
		ExtendedConn: bufio.NewExtendedConn(conn),
		pipeDeadline: MakePipeDeadline(),
		resultCh:     make(chan *connReadResult, 1),
	}
	c.resultCh <- nil
	return c
}

func (c *Conn) Read(p []byte) (n int, err error) {
	select {
	case result := <-c.resultCh:
		if result != nil {
			n = copy(p, result.unread)
			err = result.err
			result.unread = result.unread[n:]
			if len(result.unread) == 0 {
				result.release()
				c.resultCh <- nil
			} else {
				c.resultCh <- result
			}
			return
		}

		c.resultCh <- nil
	case <-c.pipeDeadline.Wait():
		return 0, os.ErrDeadlineExceeded
	}

	if c.disablePipe.Load() {
		return c.ExtendedConn.Read(p)
	}
	if c.deadline.Load().IsZero() {
		c.inRead.Store(true)
		defer c.inRead.Store(false)
		return c.ExtendedConn.Read(p)
	}

	<-c.resultCh
	go pipeRead(c, len(p))
	return c.Read(p)
}

func pipeRead(c *Conn, size int) {
	data := pool.Get(size)
	n, err := c.ExtendedConn.Read(data)

	result := connReadResultPool.Get().(*connReadResult)
	result.data = data
	result.unread = data[:n]
	result.err = err
	c.resultCh <- result
}

func (c *Conn) ReadBuffer(buffer *buf.Buffer) (err error) {
	select {
	case result := <-c.resultCh:
		if result != nil {
			n, _ := buffer.Write(result.unread)
			err = result.err
			result.unread = result.unread[n:]
			if len(result.unread) == 0 {
				result.release()
				c.resultCh <- nil
			} else {
				c.resultCh <- result
			}
			return
		}

		c.resultCh <- nil
	case <-c.pipeDeadline.Wait():
		return os.ErrDeadlineExceeded
	}

	if c.disablePipe.Load() {
		return c.ExtendedConn.ReadBuffer(buffer)
	}
	if c.deadline.Load().IsZero() {
		c.inRead.Store(true)
		defer c.inRead.Store(false)
		return c.ExtendedConn.ReadBuffer(buffer)
	}

	<-c.resultCh
	go pipeRead(c, buffer.FreeLen())
	return c.ReadBuffer(buffer)
}

func (c *Conn) SetReadDeadline(t time.Time) error {
	if c.disablePipe.Load() {
		return c.ExtendedConn.SetReadDeadline(t)
	}
	if c.inRead.Load() {
		c.disablePipe.Store(true)
		return c.ExtendedConn.SetReadDeadline(t)
	}
	c.deadline.Store(t)
	c.pipeDeadline.Set(t)
	return nil
}

func (c *Conn) ReaderReplaceable() bool {
	select {
	case result := <-c.resultCh:
		c.resultCh <- result
		if result != nil {
			return false
		}
	default:
		return false
	}
	return c.disablePipe.Load() || c.deadline.Load().IsZero()
}

func (c *Conn) WriterReplaceable() bool {
	return true
}

func (c *Conn) Upstream() any {
	return c.ExtendedConn
}
