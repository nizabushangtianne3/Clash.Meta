//go:build !race

package v5

import (
	"bytes"
	"net/netip"
	"testing"

	"github.com/metacubex/mihomo/common/pool"
)

func BenchmarkTUICNativeDatagramRoute(b *testing.B) {
	payload := make([]byte, 1350)
	packet := NewPacket(1, 2, 1, 0, 1350,
		NewAddressAddrPort(netip.MustParseAddrPort("203.0.113.7:443")), payload)
	var wire bytes.Buffer
	if err := packet.WriteTo(&wire); err != nil {
		b.Fatal(err)
	}
	raw := wire.Bytes()
	conn := &quicStreamPacketConn{
		packetCh:     make(chan Packet, 64),
		nativeClosed: make(chan struct{}),
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case received := <-conn.packetCh:
				pool.Put(received.DATA)
			case <-conn.nativeClosed:
				return
			}
		}
	}()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		conn.receiveDatagram(raw)
	}
	b.StopTimer()
	conn.packetOnce.Do(func() { close(conn.nativeClosed) })
	<-done
	conn.drainNative()
}
