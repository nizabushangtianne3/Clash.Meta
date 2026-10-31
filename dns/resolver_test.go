package dns

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/metacubex/mihomo/common/lru"
	D "github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

type fakeDNSClient struct {
	queries  atomic.Int32
	response *D.Msg
	// when release is non-nil, calls park until it closes or their context ends
	release chan struct{}
}

func (c *fakeDNSClient) ExchangeContext(ctx context.Context, m *D.Msg) (*D.Msg, error) {
	if c.release != nil {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-c.release:
		}
	}
	c.queries.Add(1)
	return c.response, nil
}

func (c *fakeDNSClient) Address() string  { return "fake" }
func (c *fakeDNSClient) ResetConnection() {}

func testQueryMsg() *D.Msg {
	return &D.Msg{
		Question: []D.Question{{Name: "example.org.", Qtype: D.TypeA, Qclass: D.ClassINET}},
	}
}

func testAResponseMsg() *D.Msg {
	response := &D.Msg{
		Answer: []D.RR{
			&D.A{Hdr: D.RR_Header{Name: "example.org.", Rrtype: D.TypeA, Class: D.ClassINET, Ttl: 300}, A: net.IP{93, 184, 216, 34}},
			&D.A{Hdr: D.RR_Header{Name: "example.org.", Rrtype: D.TypeA, Class: D.ClassINET, Ttl: 300}, A: net.IP{93, 184, 216, 35}},
		},
	}
	response.Response = true
	return response
}

func testResolver(main *fakeDNSClient) *Resolver {
	return &Resolver{
		main:  []dnsClient{main},
		cache: lru.New(lru.WithSize[dnsCacheKey, *D.Msg](64)),
	}
}

func TestLooksLikeIPLiteral(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{host: "192.168.1.2", want: true},
		{host: "2001:db8::1", want: true},
		{host: "fe80::1%eth0", want: true},
		{host: "cafe::1", want: true},
		{host: "www.example.com", want: false},
		{host: "benchmark.example.com", want: false},
	}

	for _, test := range tests {
		t.Run(test.host, func(t *testing.T) {
			require.Equal(t, test.want, looksLikeIPLiteral(test.host))
		})
	}
}

func TestExchangeContextFreshCacheHit(t *testing.T) {
	client := &fakeDNSClient{response: testAResponseMsg()}
	resolver := testResolver(client)
	query := testQueryMsg()

	putMsgToCache(resolver.cache, questionKey(query.Question[0]), testAResponseMsg())
	msg, err := resolver.ExchangeContext(context.Background(), query)
	require.NoError(t, err)
	require.Len(t, msg.Answer, 2)
	// the served TTLs count down from the cached expiry instead of staying at 300
	require.Less(t, msg.Answer[0].Header().Ttl, uint32(300))
	require.EqualValues(t, 0, client.queries.Load(), "a fresh cache hit must not query upstream")
}

func TestExchangeContextCacheHitCopiesAnswerPayload(t *testing.T) {
	resolver := testResolver(&fakeDNSClient{response: testAResponseMsg()})
	query := testQueryMsg()

	putMsgToCache(resolver.cache, questionKey(query.Question[0]), testAResponseMsg())
	first, err := resolver.ExchangeContext(context.Background(), query)
	require.NoError(t, err)
	first.Answer[0].(*D.A).A[0] = 127

	second, err := resolver.ExchangeContext(context.Background(), query)
	require.NoError(t, err)
	require.Equal(t, byte(93), second.Answer[0].(*D.A).A[0],
		"a caller mutation must not corrupt the cached answer")
}

func TestCopyCachedMsgCopiesMixedAnswerPayloads(t *testing.T) {
	source := &D.Msg{Answer: []D.RR{
		&D.A{Hdr: D.RR_Header{Name: "example.org.", Rrtype: D.TypeA}, A: net.IP{93, 184, 216, 34}},
		&D.AAAA{Hdr: D.RR_Header{Name: "example.org.", Rrtype: D.TypeAAAA}, AAAA: net.ParseIP("2001:db8::1").To16()},
	}}

	copied := copyCachedMsg(source)
	copied.Answer[0].(*D.A).A[0] = 127
	copied.Answer[1].(*D.AAAA).AAAA[0] = 0x20

	require.Equal(t, byte(93), source.Answer[0].(*D.A).A[0])
	require.Equal(t, byte(0x20), source.Answer[1].(*D.AAAA).AAAA[0])
	require.Equal(t, byte(127), copied.Answer[0].(*D.A).A[0])
	require.Equal(t, byte(0x20), copied.Answer[1].(*D.AAAA).AAAA[0])
}

func TestExchangeContextExpiredCacheHitRefreshes(t *testing.T) {
	client := &fakeDNSClient{response: testAResponseMsg()}
	resolver := testResolver(client)
	query := testQueryMsg()

	question := query.Question[0]
	expired := testAResponseMsg()
	resolver.cache.SetWithExpire(questionKey(question), expired, time.Now().Add(-time.Second))

	msg, err := resolver.ExchangeContext(context.Background(), query)
	require.NoError(t, err)
	require.Len(t, msg.Answer, 2)
	// the expired entry is served with a 1s TTL while the refresh runs
	require.EqualValues(t, 1, msg.Answer[0].Header().Ttl)
	require.Eventually(t, func() bool {
		return client.queries.Load() >= 1
	}, 2*time.Second, 5*time.Millisecond, "an expired hit must schedule a refresh")
}

func TestExchangeContextDeadlineTriggersRefresh(t *testing.T) {
	client := &fakeDNSClient{response: testAResponseMsg()}
	client.release = make(chan struct{})
	resolver := testResolver(client)
	query := &D.Msg{
		Question: []D.Question{{Name: "example.org.", Qtype: D.TypeTXT, Qclass: D.ClassINET}},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := resolver.ExchangeContext(ctx, query)
	require.ErrorIs(t, err, context.DeadlineExceeded)

	// the background refresh runs with its own timeout and reaches the client
	close(client.release)
	require.Eventually(t, func() bool {
		return client.queries.Load() >= 1
	}, 2*time.Second, 5*time.Millisecond, "a deadline cut-off must schedule a refresh")
}

func TestPutMsgToCacheSkipsAcmeChallenge(t *testing.T) {
	resolver := testResolver(&fakeDNSClient{response: testAResponseMsg()})
	query := &D.Msg{
		Question: []D.Question{{Name: "_acme-challenge.example.org.", Qtype: D.TypeTXT, Qclass: D.ClassINET}},
	}

	putMsgToCache(resolver.cache, questionKey(query.Question[0]), testAResponseMsg())

	_, _, hit := resolver.cache.GetWithExpire(questionKey(query.Question[0]))
	require.False(t, hit, "acme challenge answers must bypass the cache")
}
