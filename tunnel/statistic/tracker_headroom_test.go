package statistic

import (
	"bytes"
	"errors"
	"testing"

	"github.com/metacubex/mihomo/common/buf"
	N "github.com/metacubex/mihomo/common/net"
	C "github.com/metacubex/mihomo/constant"
)

type headroomConn struct {
	N.ExtendedConn
	required int
	buffer   *buf.Buffer
}

func (c *headroomConn) FrontHeadroom() int { return c.required }

func (c *headroomConn) Upstream() any { return nil }

func (c *headroomConn) Chains() C.Chain { return nil }

func (c *headroomConn) ProviderChains() C.Chain { return nil }

func (c *headroomConn) AppendToChains(C.ProxyAdapter) {}

func (c *headroomConn) RemoteDestination() string { return "" }

func (c *headroomConn) WriteBuffer(buffer *buf.Buffer) error {
	if buffer.Start() < c.required {
		return errors.New("insufficient headroom")
	}
	c.buffer = buffer
	return nil
}

func TestTCPTrackerWriteBufferCopiesPooledBufferWithoutFrontHeadroom(t *testing.T) {
	conn := &headroomConn{required: 14}
	tracker := &tcpTracker{Conn: conn, writeOptions: N.NewReadWaitOptions(nil, conn)}
	payload := bytes.Repeat([]byte("websocket-headroom-regression-"), 1000)

	source := buf.NewSize(32768)
	if _, err := source.Write(payload); err != nil {
		t.Fatal(err)
	}

	if err := tracker.WriteBuffer(source); err != nil {
		t.Fatal(err)
	}
	if conn.buffer == source {
		t.Fatalf("source buffer was reused without headroom: start=%d", source.Start())
	}
	if conn.buffer.Start() < conn.required {
		t.Fatalf("destination headroom = %d, want >= %d", conn.buffer.Start(), conn.required)
	}
	if !bytes.Equal(conn.buffer.Bytes(), payload) {
		t.Fatal("buffer payload mismatch")
	}
	conn.buffer.Release()
}
