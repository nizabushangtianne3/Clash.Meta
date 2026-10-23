package sniffer

import (
	"io"
	"net"
	"testing"
	"time"

	N "github.com/metacubex/mihomo/common/net"
	C "github.com/metacubex/mihomo/constant"
)

type benchSniffConn struct {
	net.Conn
	data []byte
}

func (c *benchSniffConn) Read(p []byte) (int, error) {
	if len(c.data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, c.data)
	c.data = c.data[n:]
	return n, nil
}

func (c *benchSniffConn) SetReadDeadline(time.Time) error { return nil }
func (c *benchSniffConn) Close() error                    { return nil }

type benchSniffer struct{}

func (benchSniffer) SupportNetwork() C.NetWork        { return C.TCP }
func (benchSniffer) Protocol() string                 { return "bench" }
func (benchSniffer) SupportPort(uint16) bool          { return true }
func (benchSniffer) SniffData([]byte) (string, error) { return "example.com", nil }

// BenchmarkSniffDomain covers the per-connection work of the dispatcher: it
// filters the configured sniffers by network and port, then feeds the buffered
// data until one of them names the destination.
func BenchmarkSniffDomain(b *testing.B) {
	sd := &Dispatcher{
		sniffers: []configuredSniffer{{Sniffer: benchSniffer{}}, {Sniffer: benchSniffer{}}},
	}
	segment := []byte("GET / HTTP/1.1\r\nHost: example.com\r\nUser-Agent: bench\r\n\r\n")

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		conn := N.NewBufferedConn(&benchSniffConn{data: segment})
		if _, _, err := sd.sniffDomain(conn, &C.Metadata{NetWork: C.TCP, DstPort: 80}); err != nil {
			b.Fatal(err)
		}
	}
}
