package outbound

import (
	"net/netip"
	"testing"

	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/transport/socks5"
	"github.com/stretchr/testify/require"
)

func TestSerializesSocksAddr(t *testing.T) {
	metadata := &C.Metadata{
		Host:    "example.com",
		DstIP:   netip.MustParseAddr("203.0.113.7"),
		DstPort: 443,
	}
	require.Equal(t, []byte{socks5.AtypDomainName, 11, 'e', 'x', 'a', 'm', 'p', 'l', 'e', '.', 'c', 'o', 'm', 0x01, 0xbb}, serializesSocksAddr(metadata))
}

func TestSerializesSocksAddrIP(t *testing.T) {
	metadata := &C.Metadata{
		DstIP:   netip.MustParseAddr("203.0.113.7"),
		DstPort: 443,
	}
	require.Equal(t, []byte{socks5.AtypIPv4, 203, 0, 113, 7, 0x01, 0xbb}, serializesSocksAddr(metadata))
}

func TestSerializesSocksAddrAllocatesOnce(t *testing.T) {
	metadata := &C.Metadata{
		Host:    "example.com",
		DstIP:   netip.MustParseAddr("203.0.113.7"),
		DstPort: 443,
	}
	allocations := testing.AllocsPerRun(1000, func() {
		if len(serializesSocksAddr(metadata)) == 0 {
			t.Fatal("empty address")
		}
	})
	require.LessOrEqual(t, allocations, 1.5)
}
