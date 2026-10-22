//go:build !race

package sing_vless

import (
	"bytes"
	"net/netip"
	"testing"

	"github.com/metacubex/sing/common/buf"
	M "github.com/metacubex/sing/common/metadata"
	"github.com/stretchr/testify/require"
)

// 0.5 leaves room for the one buffer a sync.Pool refills after a GC while still
// failing the per-call allocation this replaced.
func TestServerPacketConnWriteToAllocatesNothing(t *testing.T) {
	pc := &serverPacketConn{ExtendedConn: &fakeExtendedConn{}}
	payload := []byte("ping")

	allocations := testing.AllocsPerRun(1000, func() {
		if _, err := pc.WriteTo(payload, nil); err != nil {
			t.Fatal(err)
		}
	})
	require.LessOrEqual(t, allocations, 0.5)
}

func BenchmarkServerPacketConnWriteTo(b *testing.B) {
	pc := &serverPacketConn{ExtendedConn: &fakeExtendedConn{}}
	payload := make([]byte, 1350)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := pc.WriteTo(payload, nil); err != nil {
			b.Fatal(err)
		}
	}
}

func TestServerPacketConnReadPacketAllocatesNothing(t *testing.T) {
	raw := append([]byte{0x05, 0x46}, make([]byte, 1350)...)
	reader := bytes.NewReader(raw)
	pc := &serverPacketConn{
		ExtendedConn: &fakeExtendedConn{r: reader},
		destination:  M.SocksaddrFrom(netip.MustParseAddr("203.0.113.7"), 443),
	}
	buffer := buf.NewSize(1350)
	defer buffer.Release()

	allocations := testing.AllocsPerRun(1000, func() {
		reader.Reset(raw)
		buffer.Reset()
		if _, err := pc.ReadPacket(buffer); err != nil {
			t.Fatal(err)
		}
	})
	require.LessOrEqual(t, allocations, 0.5)
}

func BenchmarkServerPacketConnReadPacket(b *testing.B) {
	raw := append([]byte{0x05, 0x46}, make([]byte, 1350)...)
	reader := bytes.NewReader(raw)
	pc := &serverPacketConn{
		ExtendedConn: &fakeExtendedConn{r: reader},
		destination:  M.SocksaddrFrom(netip.MustParseAddr("203.0.113.7"), 443),
	}
	buffer := buf.NewSize(1350)
	defer buffer.Release()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader.Reset(raw)
		buffer.Reset()
		if _, err := pc.ReadPacket(buffer); err != nil {
			b.Fatal(err)
		}
	}
}
