package dns

import (
	"net"
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
