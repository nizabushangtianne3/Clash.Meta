package tunnel

import (
	"net/netip"
	"testing"

	C "github.com/metacubex/mihomo/constant"
)

func TestPreHandleMetadataEmptyHostDoesNotAllocate(t *testing.T) {
	metadata := &C.Metadata{DstIP: netip.MustParseAddr("127.0.0.1")}

	allocations := testing.AllocsPerRun(100, func() {
		if err := preHandleMetadata(metadata); err != nil {
			t.Fatal(err)
		}
	})
	if allocations != 0 {
		t.Fatalf("empty-host prehandle allocations = %v, want 0", allocations)
	}
}

func BenchmarkPreHandleMetadataEmptyHost(b *testing.B) {
	metadata := &C.Metadata{DstIP: netip.MustParseAddr("127.0.0.1")}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := preHandleMetadata(metadata); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkNewUDPFlowMetadata(b *testing.B) {
	metadata := &C.Metadata{
		NetWork: C.UDP,
		Type:    C.SOCKS5,
		SrcIP:   netip.MustParseAddr("192.168.1.2"),
		DstIP:   netip.MustParseAddr("203.0.113.20"),
		DstPort: 443,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		flow, err := newUDPFlowMetadata(metadata)
		if err != nil {
			b.Fatal(err)
		}
		if !flow.originIP.IsValid() || !flow.dial.DstIP.IsValid() {
			b.Fatal("invalid flow metadata")
		}
	}
}
