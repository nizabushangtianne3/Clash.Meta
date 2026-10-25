package dns

import (
	"context"
	"testing"

	"github.com/metacubex/mihomo/component/fakeip"
	C "github.com/metacubex/mihomo/constant"
	D "github.com/miekg/dns"
)

// The device's DNS queries ride the fake-ip path in the default setup, so one
// answer here is what every lookup the stack makes pays for.
func BenchmarkServeMsgFakeIP(b *testing.B) {
	pool := newTestFakeIPPool(b, "198.18.0.0/16")
	mapper := NewEnhancer(EnhancerConfig{
		EnhancedMode:  C.DNSFakeIP,
		FakeIPPool:    pool,
		FakeIPSkipper: &fakeip.Skipper{},
	})
	service := NewService(nil, mapper)
	ctx := context.Background()

	query := &D.Msg{}
	query.SetQuestion(D.Fqdn("www.example.com"), D.TypeA)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := service.ServeMsg(ctx, query); err != nil {
			b.Fatal(err)
		}
	}
}

// A query the pool has nothing for answers empty, which is what a skipped or
// unsupported type pays for.
func BenchmarkServeMsgFakeIPEmpty(b *testing.B) {
	mapper := NewEnhancer(EnhancerConfig{
		EnhancedMode:  C.DNSFakeIP,
		FakeIPSkipper: &fakeip.Skipper{},
	})
	service := NewService(nil, mapper)
	ctx := context.Background()

	query := &D.Msg{}
	query.SetQuestion(D.Fqdn("www.example.com"), D.TypeA)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := service.ServeMsg(ctx, query); err != nil {
			b.Fatal(err)
		}
	}
}
