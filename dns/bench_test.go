package dns

import (
	"context"
	"net"
	"net/netip"
	"testing"
	"time"

	D "github.com/miekg/dns"
)

func seededBenchResolver() *Resolver {
	return &Resolver{
		ipv6:    true,
		cache:   Config{}.newCache(),
		ipCache: Config{}.newIPCache(),
	}
}

func seedAAAAAnswerMessage(name string, ttl uint32) *D.Msg {
	query := &D.Msg{}
	query.SetQuestion(D.Fqdn(name), D.TypeAAAA)
	query.Answer = []D.RR{
		&D.AAAA{Hdr: D.RR_Header{Name: D.Fqdn(name), Rrtype: D.TypeAAAA, Class: D.ClassINET, Ttl: ttl}, AAAA: net.ParseIP("2001:db8::4").To16()},
		&D.AAAA{Hdr: D.RR_Header{Name: D.Fqdn(name), Rrtype: D.TypeAAAA, Class: D.ClassINET, Ttl: ttl}, AAAA: net.ParseIP("2001:db8::5").To16()},
	}
	return query
}

func seedAnswerMessage(name string, ttl uint32) *D.Msg {
	query := &D.Msg{}
	query.SetQuestion(D.Fqdn(name), D.TypeA)
	query.Answer = []D.RR{
		&D.A{Hdr: D.RR_Header{Name: D.Fqdn(name), Rrtype: D.TypeA, Class: D.ClassINET, Ttl: ttl}, A: netip.MustParseAddr("1.2.3.4").AsSlice()},
		&D.A{Hdr: D.RR_Header{Name: D.Fqdn(name), Rrtype: D.TypeA, Class: D.ClassINET, Ttl: ttl}, A: netip.MustParseAddr("1.2.3.5").AsSlice()},
	}
	return query
}

// The message-cache hit path deep-copies the stored answer so callers can
// mutate their copy; this measures that copy on a typical two-answer reply.
func BenchmarkExchangeContextCacheHit(b *testing.B) {
	r := seededBenchResolver()
	query := seedAnswerMessage("cached.example.", 300)
	putMsgToCache(r.cache, questionKey(query.Question[0]), query)
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := r.ExchangeContext(ctx, query); err != nil {
			b.Fatal(err)
		}
	}
}

// The tunnel lookups ride the parsed-ip cache; this measures the full
// lookup path including the answer slice clone on a hit.
func BenchmarkExchangeContextCacheHitAAAA(b *testing.B) {
	r := seededBenchResolver()
	query := seedAAAAAnswerMessage("cached-aaaa.example.", 300)
	putMsgToCache(r.cache, questionKey(query.Question[0]), query)
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := r.ExchangeContext(ctx, query); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkExchangeContextCacheHitOneA(b *testing.B) {
	r := seededBenchResolver()
	query := &D.Msg{}
	query.SetQuestion(D.Fqdn("cached-one-a.example."), D.TypeA)
	query.Answer = []D.RR{
		&D.A{Hdr: D.RR_Header{Name: D.Fqdn("cached-one-a.example."), Rrtype: D.TypeA, Class: D.ClassINET, Ttl: 300}, A: net.ParseIP("192.0.2.4").To4()},
	}
	putMsgToCache(r.cache, questionKey(query.Question[0]), query)
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := r.ExchangeContext(ctx, query); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLookupIPCached(b *testing.B) {
	r := seededBenchResolver()
	ips := []netip.Addr{netip.MustParseAddr("1.2.3.4"), netip.MustParseAddr("1.2.3.5")}
	r.ipCache.SetWithExpire(
		ipCacheEntryKey("cached.example", D.TypeA),
		cachedIPs{ips: ips},
		time.Now().Add(time.Hour),
	)
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := r.lookupIPCached(ctx, "cached.example", D.TypeA); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkServeMsgCacheHitWithPack is one local DNS query the server answers
// end to end: the wire bytes the client sent, the parse the server needs, the
// cache-hit resolution through the handler chain, and the response pack the
// writer performs.
func BenchmarkServeMsgCacheHitWithPack(b *testing.B) {
	resolver := seededBenchResolver()
	service := NewService(resolver, NewEnhancer(EnhancerConfig{IPv6: true}))
	query := seedAnswerMessage("server-cached.example.", 300)
	putMsgToCache(resolver.cache, questionKey(query.Question[0]), query)
	ctx := context.Background()
	wire, err := query.Pack()
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q := new(D.Msg)
		if err := q.Unpack(wire); err != nil {
			b.Fatal(err)
		}
		reply, err := service.ServeMsg(ctx, q)
		if err != nil || reply == nil {
			b.Fatal(err)
		}
		if _, err := reply.Pack(); err != nil {
			b.Fatal(err)
		}
	}
}
