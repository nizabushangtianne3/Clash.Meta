//go:build !race

package gost

import (
	"bytes"
	"net"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

// 0.5 leaves room for the one buffer a sync.Pool refills after a GC while still
// failing the per-call allocation this replaced.
func TestRelayPacketConnWriteToAllocatesNothing(t *testing.T) {
	raddr := net.UDPAddrFromAddrPort(netip.MustParseAddrPort("203.0.113.7:443"))
	conn := &relayPacketConn{conn: fakeRelayConn{}, raddr: raddr}
	payload := []byte("ping")

	allocations := testing.AllocsPerRun(1000, func() {
		if _, err := conn.WriteTo(payload, raddr); err != nil {
			t.Fatal(err)
		}
	})
	require.LessOrEqual(t, allocations, 0.5)
}

func BenchmarkRelayPacketConnWriteTo(b *testing.B) {
	raddr := net.UDPAddrFromAddrPort(netip.MustParseAddrPort("203.0.113.7:443"))
	conn := &relayPacketConn{conn: fakeRelayConn{}, raddr: raddr}
	payload := make([]byte, 1350)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := conn.WriteTo(payload, raddr); err != nil {
			b.Fatal(err)
		}
	}
}

func TestRelayPacketConnReadFromAllocatesNothing(t *testing.T) {
	raddr := net.UDPAddrFromAddrPort(netip.MustParseAddrPort("203.0.113.7:443"))
	raw := append([]byte{0x05, 0x46}, make([]byte, 1350)...)
	reader := bytes.NewReader(raw)
	conn := &relayPacketConn{
		conn:  fakeRelayReadConn{r: reader},
		raddr: raddr,
	}
	buf := make([]byte, 2048)

	allocations := testing.AllocsPerRun(1000, func() {
		reader.Reset(raw)
		if _, _, err := conn.ReadFrom(buf); err != nil {
			t.Fatal(err)
		}
	})
	require.LessOrEqual(t, allocations, 0.5)
}

func BenchmarkRelayPacketConnReadFrom(b *testing.B) {
	raddr := net.UDPAddrFromAddrPort(netip.MustParseAddrPort("203.0.113.7:443"))
	raw := append([]byte{0x05, 0x46}, make([]byte, 1350)...)
	reader := bytes.NewReader(raw)
	conn := &relayPacketConn{
		conn:  fakeRelayReadConn{r: reader},
		raddr: raddr,
	}
	buf := make([]byte, 2048)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader.Reset(raw)
		if _, _, err := conn.ReadFrom(buf); err != nil {
			b.Fatal(err)
		}
	}
}

func TestRelayPacketConnReadFromOversizedAllocatesNothing(t *testing.T) {
	raddr := net.UDPAddrFromAddrPort(netip.MustParseAddrPort("203.0.113.7:443"))
	raw := append([]byte{0x00, 0x40}, make([]byte, 64)...)
	reader := bytes.NewReader(raw)
	conn := &relayPacketConn{
		conn:  fakeRelayReadConn{r: reader},
		raddr: raddr,
	}
	buf := make([]byte, 32)

	allocations := testing.AllocsPerRun(1000, func() {
		reader.Reset(raw)
		if _, _, err := conn.ReadFrom(buf); err != nil {
			t.Fatal(err)
		}
	})
	require.LessOrEqual(t, allocations, 0.5)
}

func BenchmarkRelayPacketConnReadFromOversized(b *testing.B) {
	raddr := net.UDPAddrFromAddrPort(netip.MustParseAddrPort("203.0.113.7:443"))
	raw := append([]byte{0x00, 0x40}, make([]byte, 64)...)
	reader := bytes.NewReader(raw)
	conn := &relayPacketConn{
		conn:  fakeRelayReadConn{r: reader},
		raddr: raddr,
	}
	buf := make([]byte, 32)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader.Reset(raw)
		if _, _, err := conn.ReadFrom(buf); err != nil {
			b.Fatal(err)
		}
	}
}
