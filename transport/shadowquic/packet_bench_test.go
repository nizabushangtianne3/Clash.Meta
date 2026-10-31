package shadowquic

import (
	"net/netip"
	"testing"

	"github.com/metacubex/mihomo/common/net/packet"
	"github.com/metacubex/mihomo/common/pool"
	"github.com/metacubex/mihomo/transport/socks5"
)

func BenchmarkAssociationWaitReadFrom(b *testing.B) {
	const payloadSize = 1200
	ap := netip.MustParseAddrPort("203.0.113.7:443")
	a := &association{input: make(chan receivedPacket, 1)}
	wrapper := packet.NewEnhancePacketConn(a)
	payload := pool.Get(payloadSize)
	copy(payload, make([]byte, payloadSize))
	packetData := receivedPacket{
		data: payload,
		addr: socksNetAddr{addr: socks5.AddrFromStdAddrPort(ap)},
	}
	testAddr, testAddrErr := AddrPortFromNetAddr(packetData.addr)
	if testAddrErr != nil || testAddr != ap {
		b.Fatalf("address conversion: %v %v", testAddr, testAddrErr)
	}

	b.ReportAllocs()
	b.SetBytes(payloadSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		a.input <- packetData
		data, _, err := wrapper.WaitReadFrom()
		if err != nil {
			b.Fatal(err)
		}
		if len(data) != payloadSize {
			b.Fatalf("invalid datagram: len=%d", len(data))
		}
		pool.Put(data)
	}
}
