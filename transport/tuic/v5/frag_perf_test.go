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

func BenchmarkFeedSingleFragment(b *testing.B) {
	payload := make([]byte, 1200)
	packets := make([]Packet, 64)
	for i := range packets {
		packets[i] = fragmentPacket(0, 1, uint16(len(payload)), payload)
		packets[i].PKT_ID = uint16(i)
	}
	var d deFragger
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		packet, ok := d.Feed(&packets[i&63])
		if !ok || len(packet.DATA) != len(payload) {
			b.Fatal("single fragment was not forwarded")
		}
	}
}

func BenchmarkFeedUniqueFragmentedMessages(b *testing.B) {
	const fragments = 4
	const fragmentSize = 512
	payload := make([]byte, fragmentSize)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var d deFragger
		messageID := uint16(i)
		for fragmentID := 0; fragmentID < fragments; fragmentID++ {
			data := pool.Get(fragmentSize)
			copy(data, payload)
			packet := fragmentPacket(uint8(fragmentID), fragments, fragmentSize, data)
			packet.PKT_ID = messageID
			assembled, ok := d.Feed(&packet)
			if fragmentID != fragments-1 {
				if ok {
					b.Fatal("partial message completed")
				}
				continue
			}
			if !ok {
				b.Fatal("final fragment did not complete")
			}
			pool.Put(assembled.DATA)
		}
	}
}
