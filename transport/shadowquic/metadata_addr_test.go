package shadowquic

import (
	"net/netip"
	"testing"

	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/transport/socks5"
	"github.com/stretchr/testify/require"
)

func TestMetadataAddrMatchesTheSocksEncoder(t *testing.T) {
	for _, test := range []struct {
		metadata *C.Metadata
		want     string
	}{
		{&C.Metadata{DstIP: netip.MustParseAddr("1.2.3.4"), DstPort: 53}, "1.2.3.4:53"},
		{&C.Metadata{DstIP: netip.MustParseAddr("2001:db8::1"), DstPort: 443}, "[2001:db8::1]:443"},
		{&C.Metadata{DstIP: netip.MustParseAddr("::ffff:1.2.3.4"), DstPort: 53}, "1.2.3.4:53"},
		{&C.Metadata{Host: "example.com", DstPort: 443}, "example.com:443"},
	} {
		got, err := MetadataAddr(test.metadata)
		require.NoError(t, err)
		require.Equal(t, socks5.ParseAddr(test.want), got, "destination %s", test.want)
	}
}

func TestMetadataAddrRejectsAnEmptyDestination(t *testing.T) {
	_, err := MetadataAddr(&C.Metadata{DstPort: 53})
	require.ErrorIs(t, err, errInvalidAddress)
}

// An IP target used to be formatted into a host:port string and parsed back
// through net.ParseIP, which cost a handful of allocations on every request.
func TestMetadataAddrAllocatesOnceForAnIPDestination(t *testing.T) {
	metadata := &C.Metadata{DstIP: netip.MustParseAddr("203.0.113.7"), DstPort: 53}

	allocations := testing.AllocsPerRun(100, func() {
		if _, err := MetadataAddr(metadata); err != nil {
			t.Fatal(err)
		}
	})
	require.EqualValues(t, 1, allocations)
}

func BenchmarkMetadataAddr(b *testing.B) {
	metadata := &C.Metadata{DstIP: netip.MustParseAddr("203.0.113.7"), DstPort: 53}

	b.Run("addrport", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := MetadataAddr(metadata); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("addr-string", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = socks5.ParseAddr(metadata.RemoteAddress())
		}
	})
}
