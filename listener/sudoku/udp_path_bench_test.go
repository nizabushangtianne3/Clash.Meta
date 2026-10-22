package sudoku

import (
	"bytes"
	"net/netip"
	"testing"

	"github.com/metacubex/mihomo/transport/sudoku"
)

// BenchmarkUoTPacketPath measures one datagram's whole inbound step: the wire
// frame is re-encoded, read back, and routed through the session.
func BenchmarkUoTPacketPath(b *testing.B) {
	target := netip.MustParseAddrPort("1.2.3.4:53")
	payload := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	session := uotSession{
		writer: sudoku.NewUoTPacketConn(nil),
		source: netip.MustParseAddrPort("192.168.1.2:54321"),
		tunnel: uotDropTunnel{},
	}
	var frame bytes.Buffer
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		frame.Reset()
		if err := sudoku.WriteDatagram(&frame, target, payload); err != nil {
			b.Fatal(err)
		}
		gotTarget, gotPayload, err := sudoku.ReadDatagram(&frame)
		if err != nil {
			b.Fatal(err)
		}
		session.handle(gotTarget, gotPayload)
	}
}
