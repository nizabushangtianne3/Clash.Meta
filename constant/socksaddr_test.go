package constant

import (
	"net/netip"
	"testing"

	M "github.com/metacubex/sing/common/metadata"
	"github.com/stretchr/testify/require"
)

func TestMetadataSocksaddrKeepsTheHostAndTheAddrPort(t *testing.T) {
	byHost := (&Metadata{Host: "example.com", DstPort: 443}).Socksaddr()
	require.Equal(t, "example.com", byHost.Fqdn)
	require.EqualValues(t, 443, byHost.Port)
	require.False(t, byHost.Addr.IsValid())

	byIP := (&Metadata{DstIP: netip.MustParseAddr("203.0.113.7"), DstPort: 53}).Socksaddr()
	require.Equal(t, netip.MustParseAddr("203.0.113.7"), byIP.Addr)
	require.EqualValues(t, 53, byIP.Port)
}

// An IP destination used to be formatted into a host:port string and parsed
// straight back, which allocated that string on every connection. The host
// branch is unchanged: Metadata.String already returns the host itself, so the
// old call passed the same string this one does.
func TestMetadataSocksaddrAllocatesNothingForAnIPDestination(t *testing.T) {
	metadata := &Metadata{DstIP: netip.MustParseAddr("203.0.113.7"), DstPort: 53}

	allocations := testing.AllocsPerRun(100, func() {
		_ = metadata.Socksaddr()
	})
	require.Zero(t, allocations)
}

func BenchmarkMetadataSocksaddr(b *testing.B) {
	metadata := &Metadata{DstIP: netip.MustParseAddr("203.0.113.7"), DstPort: 53}

	b.Run("addrport", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = metadata.Socksaddr()
		}
	})

	b.Run("host-port-string", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = M.ParseSocksaddrHostPort(metadata.String(), metadata.DstPort)
		}
	})
}
