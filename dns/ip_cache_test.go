package dns

import (
	"context"
	"net/netip"
	"testing"
	"time"

	D "github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

func TestLookupIPUsesParsedCache(t *testing.T) {
	client := &countingDNSClient{}
	r := NewResolverFromClient(client)
	ctx := context.Background()

	ips, err := r.LookupIP(ctx, "www.example.com")
	require.NoError(t, err)
	require.NotEmpty(t, ips)
	require.EqualValues(t, 2, client.queries.Load(), "the first A and AAAA queries must both be exchanged")

	repeated, err := r.LookupIP(ctx, "www.example.com")
	require.NoError(t, err)
	require.Equal(t, ips, repeated)
	require.EqualValues(t, 2, client.queries.Load(), "the parsed A/AAAA result must not dial upstream again")
	_, _, hit := r.ipCache.GetWithExpire(ipCacheEntryKey("www.example.com", D.TypeA))
	require.True(t, hit)
	_, _, hit = r.ipCache.GetWithExpire(ipCacheEntryKey("www.example.com", D.TypeAAAA))
	require.True(t, hit)

	r.ClearCache()
	_, _, hit = r.ipCache.GetWithExpire(ipCacheEntryKey("www.example.com", D.TypeA))
	require.False(t, hit)
	_, _, hit = r.ipCache.GetWithExpire(ipCacheEntryKey("www.example.com", D.TypeAAAA))
	require.False(t, hit)
}

func TestLookupIPCachePairKeepsAddressOrder(t *testing.T) {
	client := &countingDNSClient{}
	r := NewResolverFromClient(client)
	ctx := context.Background()

	_, err := r.LookupIP(ctx, "www.example.com")
	require.NoError(t, err)

	ips, hit := r.lookupIPFromCachePair(ctx, "www.example.com")
	require.True(t, hit)
	require.Equal(t, []netip.Addr{
		netip.MustParseAddr("192.0.2.10"),
		netip.MustParseAddr("192.0.2.11"),
		netip.MustParseAddr("2001:db8::1"),
	}, ips)
}

func TestLookupIPStoresPreaggregatedPair(t *testing.T) {
	client := &countingDNSClient{}
	r := NewResolverFromClient(client)
	ctx := context.Background()

	_, err := r.LookupIP(ctx, "www.example.com")
	require.NoError(t, err)

	pair, expireTime, hit := r.ipCache.GetWithExpire(ipCacheEntryKey("www.example.com", ipCachePairQtype))
	require.True(t, hit)
	require.NotNil(t, pair.msg)
	require.NotNil(t, pair.pairMsg)
	require.Equal(t, D.TypeA, pair.msg.Question[0].Qtype)
	require.Equal(t, D.TypeAAAA, pair.pairMsg.Question[0].Qtype)
	require.Equal(t, []netip.Addr{
		netip.MustParseAddr("192.0.2.10"),
		netip.MustParseAddr("192.0.2.11"),
		netip.MustParseAddr("2001:db8::1"),
	}, pair.ips)
	require.False(t, expireTime.Before(time.Now()))
}

func TestCacheIPPairUsesTheEarlierExpiry(t *testing.T) {
	r := NewResolverFromClient(&countingDNSClient{})
	host := "www.example.com"
	aMsg := &D.Msg{
		Question: []D.Question{{Name: D.Fqdn(host), Qtype: D.TypeA, Qclass: D.ClassINET}},
		Answer:   []D.RR{&D.A{Hdr: D.RR_Header{Name: D.Fqdn(host), Rrtype: D.TypeA, Class: D.ClassINET, Ttl: 100}}},
	}
	aaaaMsg := &D.Msg{
		Question: []D.Question{{Name: D.Fqdn(host), Qtype: D.TypeAAAA, Qclass: D.ClassINET}},
		Answer:   []D.RR{&D.AAAA{Hdr: D.RR_Header{Name: D.Fqdn(host), Rrtype: D.TypeAAAA, Class: D.ClassINET, Ttl: 50}}},
	}

	r.cacheLookupIP(host, D.TypeA, aMsg, []netip.Addr{netip.MustParseAddr("192.0.2.10")})
	r.cacheLookupIP(host, D.TypeAAAA, aaaaMsg, []netip.Addr{netip.MustParseAddr("2001:db8::1")})

	pair, expireTime, hit := r.ipCache.GetWithExpire(ipCacheEntryKey(host, ipCachePairQtype))
	require.True(t, hit)
	require.Len(t, pair.ips, 2)
	require.False(t, expireTime.After(time.Now().Add(50*time.Second)))
	require.True(t, expireTime.After(time.Now().Add(49*time.Second)))
}
