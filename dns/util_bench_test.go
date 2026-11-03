package dns

import (
	"net"
	"testing"

	D "github.com/miekg/dns"
)

func benchmarkCachePutMessage() *D.Msg {
	msg := &D.Msg{}
	msg.SetQuestion(D.Fqdn("cache-put.example."), D.TypeA)
	msg.Rcode = D.RcodeSuccess
	msg.Answer = []D.RR{
		&D.A{Hdr: D.RR_Header{Name: D.Fqdn("cache-put.example."), Rrtype: D.TypeA, Class: D.ClassINET, Ttl: 120}, A: []byte{192, 0, 2, 10}},
		&D.A{Hdr: D.RR_Header{Name: D.Fqdn("cache-put.example."), Rrtype: D.TypeA, Class: D.ClassINET, Ttl: 240}, A: []byte{192, 0, 2, 11}},
	}
	msg.Extra = []D.RR{
		&D.OPT{Hdr: D.RR_Header{Name: ".", Rrtype: D.TypeOPT}},
	}
	return msg
}

func benchmarkCachePutAAAAMessage() *D.Msg {
	msg := &D.Msg{}
	msg.SetQuestion(D.Fqdn("cache-put-aaaa.example."), D.TypeAAAA)
	msg.Rcode = D.RcodeSuccess
	msg.Answer = []D.RR{
		&D.AAAA{Hdr: D.RR_Header{Name: D.Fqdn("cache-put-aaaa.example."), Rrtype: D.TypeAAAA, Class: D.ClassINET, Ttl: 120}, AAAA: net.ParseIP("2001:db8::10").To16()},
		&D.AAAA{Hdr: D.RR_Header{Name: D.Fqdn("cache-put-aaaa.example."), Rrtype: D.TypeAAAA, Class: D.ClassINET, Ttl: 240}, AAAA: net.ParseIP("2001:db8::11").To16()},
	}
	msg.Extra = []D.RR{
		&D.OPT{Hdr: D.RR_Header{Name: ".", Rrtype: D.TypeOPT}},
	}
	return msg
}

func benchmarkCachePutOneA() *D.Msg {
	msg := &D.Msg{}
	msg.SetQuestion(D.Fqdn("cache-put-one.example."), D.TypeA)
	msg.Rcode = D.RcodeSuccess
	msg.Answer = []D.RR{
		&D.A{Hdr: D.RR_Header{Name: D.Fqdn("cache-put-one.example."), Rrtype: D.TypeA, Class: D.ClassINET, Ttl: 180}, A: net.IP{192, 0, 2, 9}},
	}
	msg.Extra = []D.RR{
		&D.OPT{Hdr: D.RR_Header{Name: ".", Rrtype: D.TypeOPT}},
	}
	return msg
}

func BenchmarkPutMsgToCacheOneA(b *testing.B) {
	r := seededBenchResolver()
	msg := benchmarkCachePutOneA()
	key := questionKey(msg.Question[0])

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		putMsgToCache(r.cache, key, msg)
	}
}

func BenchmarkPutMsgToCacheTypicalAAAA(b *testing.B) {
	r := seededBenchResolver()
	msg := benchmarkCachePutAAAAMessage()
	key := questionKey(msg.Question[0])

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		putMsgToCache(r.cache, key, msg)
	}
}

func BenchmarkPutMsgToCacheTypical(b *testing.B) {
	r := seededBenchResolver()
	msg := benchmarkCachePutMessage()
	key := questionKey(msg.Question[0])

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		putMsgToCache(r.cache, key, msg)
	}
}

func benchmarkCacheMessage(qtype uint16, addresses []net.IP) *D.Msg {
	msg := &D.Msg{}
	msg.SetQuestion(D.Fqdn("cache-multi.example."), qtype)
	msg.RecursionAvailable = true
	for _, address := range addresses {
		if qtype == D.TypeA {
			msg.Answer = append(msg.Answer, &D.A{
				Hdr: D.RR_Header{Name: msg.Question[0].Name, Rrtype: qtype, Class: D.ClassINET, Ttl: 120},
				A:   address,
			})
		} else {
			msg.Answer = append(msg.Answer, &D.AAAA{
				Hdr:  D.RR_Header{Name: msg.Question[0].Name, Rrtype: qtype, Class: D.ClassINET, Ttl: 120},
				AAAA: address,
			})
		}
	}
	return msg
}

func BenchmarkCacheHitFourA(b *testing.B) {
	msg := benchmarkCacheMessage(D.TypeA, []net.IP{
		net.IPv4(192, 0, 2, 1), net.IPv4(192, 0, 2, 2),
		net.IPv4(192, 0, 2, 3), net.IPv4(192, 0, 2, 4),
	})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if copyCachedMsg(msg, false) == nil {
			b.Fatal("cache copy failed")
		}
	}
}

func BenchmarkCacheHitFourAAAA(b *testing.B) {
	msg := benchmarkCacheMessage(D.TypeAAAA, []net.IP{
		net.ParseIP("2001:db8::1"), net.ParseIP("2001:db8::2"),
		net.ParseIP("2001:db8::3"), net.ParseIP("2001:db8::4"),
	})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if copyCachedMsg(msg, false) == nil {
			b.Fatal("cache copy failed")
		}
	}
}
