package constant

import (
	"net/netip"
	"testing"
)

var metadataAddressSink string

func BenchmarkMetadataRemoteAddress(b *testing.B) {
	benchmarks := []struct {
		name     string
		metadata Metadata
	}{
		{
			name: "domain",
			metadata: Metadata{
				Host:    "benchmark.example.com",
				DstPort: 443,
			},
		},
		{
			name: "ipv4",
			metadata: Metadata{
				DstIP:   netip.MustParseAddr("192.0.2.1"),
				DstPort: 443,
			},
		},
		{
			name: "ipv6",
			metadata: Metadata{
				DstIP:   netip.MustParseAddr("2001:db8::1"),
				DstPort: 443,
			},
		},
	}
	for _, benchmark := range benchmarks {
		metadata := benchmark.metadata
		b.Run(benchmark.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				metadataAddressSink = metadata.RemoteAddress()
			}
		})
	}
}
