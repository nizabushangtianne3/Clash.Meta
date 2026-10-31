package constant

import (
	"net/netip"
	"testing"
)

var metadataSourceSink string

func BenchmarkMetadataSourceAddress(b *testing.B) {
	benchmarks := []struct {
		name     string
		metadata Metadata
	}{
		{name: "ipv4", metadata: Metadata{SrcIP: netip.MustParseAddr("192.168.1.2"), SrcPort: 54321}},
		{name: "ipv6", metadata: Metadata{SrcIP: netip.MustParseAddr("2001:db8::1"), SrcPort: 54321}},
		{name: "ipv6-zone", metadata: Metadata{SrcIP: netip.MustParseAddr("fe80::1%eth0"), SrcPort: 54321}},
	}
	for _, benchmark := range benchmarks {
		metadata := benchmark.metadata
		b.Run(benchmark.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				metadataSourceSink = metadata.SourceAddress()
			}
		})
	}
}

func BenchmarkMetadataSourceDetail(b *testing.B) {
	benchmarks := []struct {
		name     string
		metadata Metadata
	}{
		{name: "default", metadata: Metadata{SrcIP: netip.MustParseAddr("192.168.1.2"), SrcPort: 54321}},
		{
			name: "process",
			metadata: Metadata{
				SrcIP:   netip.MustParseAddr("192.168.1.2"),
				SrcPort: 54321,
				Process: "chrome.exe",
			},
		},
		{
			name: "process-uid",
			metadata: Metadata{
				SrcIP:   netip.MustParseAddr("192.168.1.2"),
				SrcPort: 54321,
				Process: "chrome.exe",
				Uid:     1001,
			},
		},
	}
	for _, benchmark := range benchmarks {
		metadata := benchmark.metadata
		b.Run(benchmark.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				metadataSourceSink = metadata.SourceDetail()
			}
		})
	}
}
