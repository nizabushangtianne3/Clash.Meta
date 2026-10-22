//go:build !race

package trusttunnel

import (
	"bytes"
	"net/netip"
	"testing"

	"github.com/metacubex/sing/common/buf"
	M "github.com/metacubex/sing/common/metadata"
	"github.com/stretchr/testify/require"
)

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

type packetReader struct {
	r *bytes.Reader
}

func (p *packetReader) Read(b []byte) (int, error) { return p.r.Read(b) }

func (p *packetReader) Close() error { return nil }

func BenchmarkClientWritePacketToServer(b *testing.B) {
	client := &clientPacketConn{
		packetConn: packetConn{
			httpConn: httpConn{writer: discardWriter{}},
		},
	}
	destination := M.SocksaddrFrom(netip.MustParseAddr("203.0.113.7"), 443)
	payload := make([]byte, 1350)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := client.writePacketToServer(buf.As(payload), destination); err != nil {
			b.Fatal(err)
		}
	}
}

func TestClientWritePacketToServerAllocatesOnlyTheBuffer(t *testing.T) {
	client := &clientPacketConn{
		packetConn: packetConn{
			httpConn: httpConn{writer: discardWriter{}},
		},
	}
	destination := M.SocksaddrFrom(netip.MustParseAddr("203.0.113.7"), 443)
	payload := make([]byte, 1350)

	allocations := testing.AllocsPerRun(1000, func() {
		if err := client.writePacketToServer(buf.As(payload), destination); err != nil {
			t.Fatal(err)
		}
	})
	require.LessOrEqual(t, allocations, 1.5)
}

func BenchmarkServerReadPacketFromClient(b *testing.B) {
	var wire bytes.Buffer
	client := &clientPacketConn{
		packetConn: packetConn{
			httpConn: httpConn{writer: &wire},
		},
	}
	destination := M.SocksaddrFrom(netip.MustParseAddr("203.0.113.7"), 443)
	if err := client.writePacketToServer(buf.As(make([]byte, 1350)), destination); err != nil {
		b.Fatal(err)
	}
	raw := wire.Bytes()

	body := &packetReader{r: bytes.NewReader(raw)}
	server := &serverPacketConn{
		packetConn: packetConn{
			httpConn: httpConn{body: body},
		},
	}
	buffer := buf.NewSize(1350)
	defer buffer.Release()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		body.r.Reset(raw)
		buffer.Reset()
		if _, err := server.readPacketFromClient(buffer); err != nil {
			b.Fatal(err)
		}
	}
}

func TestServerReadPacketFromClientAllocatesBufferAndHeader(t *testing.T) {
	var wire bytes.Buffer
	client := &clientPacketConn{
		packetConn: packetConn{
			httpConn: httpConn{writer: &wire},
		},
	}
	destination := M.SocksaddrFrom(netip.MustParseAddr("203.0.113.7"), 443)
	require.NoError(t, client.writePacketToServer(buf.As(make([]byte, 1350)), destination))
	raw := wire.Bytes()

	body := &packetReader{r: bytes.NewReader(raw)}
	server := &serverPacketConn{
		packetConn: packetConn{
			httpConn: httpConn{body: body},
		},
	}
	buffer := buf.NewSize(1350)
	defer buffer.Release()

	allocations := testing.AllocsPerRun(1000, func() {
		body.r.Reset(raw)
		buffer.Reset()
		if _, err := server.readPacketFromClient(buffer); err != nil {
			t.Fatal(err)
		}
	})
	require.LessOrEqual(t, allocations, 2.5)
}
