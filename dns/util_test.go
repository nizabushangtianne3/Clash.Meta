package dns

import (
	"net"
	"strconv"
	"testing"

	D "github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

func TestPutMsgToCacheClonesSourceAndDiscardsOPT(t *testing.T) {
	resolver := testResolver(nil)
	source := testAResponseMsg()
	source.Question = testQueryMsg().Question
	source.Extra = []D.RR{
		&D.OPT{Hdr: D.RR_Header{Name: ".", Rrtype: D.TypeOPT}},
	}
	key := questionKey(source.Question[0])

	putMsgToCache(resolver.cache, key, source)
	cached, _, hit := resolver.cache.GetWithExpire(key)
	require.True(t, hit)
	require.NotNil(t, cached)
	require.Empty(t, cached.Extra)

	source.Question[0].Name = "mutated.example.org."
	source.Answer[0].(*D.A).A = net.IP{127, 0, 0, 1}
	source.Extra[0].Header().Ttl = 99

	require.Equal(t, "example.org.", cached.Question[0].Name)
	require.Equal(t, net.IP{93, 184, 216, 34}, cached.Answer[0].(*D.A).A)
	require.Empty(t, cached.Extra)
}

func TestCacheHitThreeARecordsStayIndependent(t *testing.T) {
	source := &D.Msg{Answer: []D.RR{
		&D.A{Hdr: D.RR_Header{Name: "example.org.", Rrtype: D.TypeA}, A: net.IP{192, 0, 2, 1}},
		&D.A{Hdr: D.RR_Header{Name: "example.org.", Rrtype: D.TypeA}, A: net.IP{192, 0, 2, 2}},
		&D.A{Hdr: D.RR_Header{Name: "example.org.", Rrtype: D.TypeA}, A: net.IP{192, 0, 2, 3}},
	}}

	copied := copyCachedMsg(source, false)
	copied.Answer[1].(*D.A).A = net.IP{127, 0, 0, 1}

	require.Equal(t, net.IP{192, 0, 2, 2}, source.Answer[1].(*D.A).A)
	require.Equal(t, net.IP{127, 0, 0, 1}, copied.Answer[1].(*D.A).A)
}

func TestCacheHitTwoAAAARecordsStayIndependent(t *testing.T) {
	source := &D.Msg{Answer: []D.RR{
		&D.AAAA{Hdr: D.RR_Header{Name: "example.org.", Rrtype: D.TypeAAAA}, AAAA: net.ParseIP("2001:db8::1").To16()},
		&D.AAAA{Hdr: D.RR_Header{Name: "example.org.", Rrtype: D.TypeAAAA}, AAAA: net.ParseIP("2001:db8::2").To16()},
	}}

	copied := copyCachedMsg(source, false)
	copied.Answer[1].(*D.AAAA).AAAA = net.ParseIP("2001:db8::dead").To16()

	require.Equal(t, net.ParseIP("2001:db8::2").To16(), source.Answer[1].(*D.AAAA).AAAA)
	require.Equal(t, net.ParseIP("2001:db8::dead").To16(), copied.Answer[1].(*D.AAAA).AAAA)
}

func TestPutMsgToCacheClonesAAAAAndDiscardsOPT(t *testing.T) {
	resolver := testResolver(nil)
	source := &D.Msg{}
	source.SetQuestion("aaaa.example.org.", D.TypeAAAA)
	source.Answer = []D.RR{
		&D.AAAA{Hdr: D.RR_Header{Name: source.Question[0].Name, Rrtype: D.TypeAAAA, Class: D.ClassINET, Ttl: 120}, AAAA: net.ParseIP("2001:db8::10").To16()},
		&D.AAAA{Hdr: D.RR_Header{Name: source.Question[0].Name, Rrtype: D.TypeAAAA, Class: D.ClassINET, Ttl: 240}, AAAA: net.ParseIP("2001:db8::11").To16()},
	}
	source.Extra = []D.RR{&D.OPT{Hdr: D.RR_Header{Name: ".", Rrtype: D.TypeOPT}}}
	key := questionKey(source.Question[0])

	putMsgToCache(resolver.cache, key, source)
	cached, _, hit := resolver.cache.GetWithExpire(key)
	require.True(t, hit)
	require.Empty(t, cached.Extra)

	source.Question[0].Name = "mutated.example.org."
	source.Answer[0].(*D.AAAA).AAAA = net.ParseIP("2001:db8::dead").To16()
	source.Extra[0].Header().Ttl = 99

	require.Equal(t, "aaaa.example.org.", cached.Question[0].Name)
	require.Equal(t, net.ParseIP("2001:db8::10").To16(), cached.Answer[0].(*D.AAAA).AAAA)
	require.Equal(t, net.ParseIP("2001:db8::11").To16(), cached.Answer[1].(*D.AAAA).AAAA)
	require.Empty(t, cached.Extra)
}

func TestOneACacheWriteAndHitStayIndependent(t *testing.T) {
	resolver := testResolver(nil)
	source := &D.Msg{}
	source.SetQuestion("one.example.org.", D.TypeA)
	source.Answer = []D.RR{
		&D.A{Hdr: D.RR_Header{Name: source.Question[0].Name, Rrtype: D.TypeA, Class: D.ClassINET, Ttl: 180}, A: net.IP{192, 0, 2, 9}},
	}
	source.Extra = []D.RR{&D.OPT{Hdr: D.RR_Header{Name: ".", Rrtype: D.TypeOPT}}}
	key := questionKey(source.Question[0])

	putMsgToCache(resolver.cache, key, source)
	cached, _, hit := resolver.cache.GetWithExpire(key)
	require.True(t, hit)
	require.Empty(t, cached.Extra)

	source.Question[0].Name = "mutated.example.org."
	source.Answer[0].(*D.A).A = net.IP{127, 0, 0, 1}
	source.Extra[0].Header().Ttl = 99

	require.Equal(t, "one.example.org.", cached.Question[0].Name)
	require.Equal(t, net.IP{192, 0, 2, 9}, cached.Answer[0].(*D.A).A)
	require.Empty(t, cached.Extra)
}

func TestCacheHitFourRecordsStayIndependent(t *testing.T) {
	for _, tc := range []struct {
		name       string
		qtype      uint16
		discardOPT bool
	}{
		{name: "A", qtype: D.TypeA},
		{name: "AAAA", qtype: D.TypeAAAA},
		{name: "A/discard-OPT", qtype: D.TypeA, discardOPT: true},
		{name: "AAAA/discard-OPT", qtype: D.TypeAAAA, discardOPT: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := &D.Msg{}
			source.SetQuestion("four."+tc.name+".example.org.", tc.qtype)
			for i := range 4 {
				var address net.IP
				if tc.qtype == D.TypeA {
					address = net.ParseIP("192.0.2." + strconv.Itoa(i+1))
				} else {
					address = net.ParseIP("2001:db8::" + strconv.Itoa(i+1))
				}
				if tc.qtype == D.TypeA {
					source.Answer = append(source.Answer, &D.A{
						Hdr: D.RR_Header{Name: source.Question[0].Name, Rrtype: tc.qtype, Class: D.ClassINET, Ttl: 120},
						A:   address,
					})
				} else {
					source.Answer = append(source.Answer, &D.AAAA{
						Hdr:  D.RR_Header{Name: source.Question[0].Name, Rrtype: tc.qtype, Class: D.ClassINET, Ttl: 120},
						AAAA: address.To16(),
					})
				}
			}
			if tc.discardOPT {
				source.Question[0].Name = "write." + tc.name + ".example.org."
				source.Extra = []D.RR{&D.OPT{Hdr: D.RR_Header{Name: ".", Rrtype: D.TypeOPT}}}
			}

			copied := copyCachedMsg(source, tc.discardOPT)
			copied.Answer[1].Header().Ttl = 99
			if tc.qtype == D.TypeA {
				copied.Answer[1].(*D.A).A = net.IP{127, 0, 0, 1}
			} else {
				copied.Answer[1].(*D.AAAA).AAAA = net.ParseIP("2001:db8::dead")
			}

			require.Len(t, copied.Answer, 4)
			if tc.discardOPT {
				require.Empty(t, copied.Extra)
				require.Equal(t, "write."+tc.name+".example.org.", source.Question[0].Name)
				require.Equal(t, "write."+tc.name+".example.org.", copied.Question[0].Name)
				source.Question[0].Name = "mutated." + tc.name + ".example.org."
				require.Equal(t, "write."+tc.name+".example.org.", copied.Question[0].Name)
			}
			for i := range source.Answer {
				if i != 1 {
					require.Equal(t, source.Answer[i].Header().Ttl, copied.Answer[i].Header().Ttl)
				}
			}
			if tc.qtype == D.TypeA {
				require.Equal(t, net.IP{127, 0, 0, 1}, copied.Answer[1].(*D.A).A)
				require.Equal(t, net.ParseIP("192.0.2.2").String(), source.Answer[1].(*D.A).A.String())
			} else {
				require.Equal(t, net.ParseIP("2001:db8::dead"), copied.Answer[1].(*D.AAAA).AAAA)
				require.Equal(t, net.ParseIP("2001:db8::2").To16(), source.Answer[1].(*D.AAAA).AAAA)
			}
		})
	}
}
