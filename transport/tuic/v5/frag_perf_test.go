//go:build !race

package v5

import (
	"testing"

	"github.com/metacubex/mihomo/common/pool"
)

func fragmentedFeedPackets() []Packet {
	payload := make([]byte, 4096)
	fragSize := 1024
	packets := make([]Packet, 0, len(payload)/fragSize)
	for off := 0; off < len(payload); off += fragSize {
		packets = append(packets, Packet{
			ASSOC_ID:   7,
			PKT_ID:     9,
			FRAG_TOTAL: uint8(len(payload) / fragSize),
			FRAG_ID:    uint8(len(packets)),
			SIZE:       uint16(fragSize),
			DATA:       payload[off : off+fragSize],
		})
	}
	return packets
}

func BenchmarkFeedFragmented(b *testing.B) {
	fragments := fragmentedFeedPackets()
	assembled := Packet{}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var d deFragger
		for j := range fragments {
			fragment := fragments[j]
			if packet, ok := d.Feed(&fragment); ok {
				assembled = packet
			}
		}
		_ = pool.Put(assembled.DATA)
		assembled.DATA = nil
	}
	if len(assembled.DATA) != 0 {
		b.Fatal("assembled payload was not released")
	}
}
