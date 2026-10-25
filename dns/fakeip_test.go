package dns

import (
	"context"
	"testing"

	"github.com/metacubex/mihomo/component/fakeip"
	C "github.com/metacubex/mihomo/constant"
	D "github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

// The reply is built on the query's own header, question and EDNS0 record, so
// all three have to come back: the host reads them, and a client that
// advertised a UDP size has to keep it.
func TestFakeIPReplyCarriesTheQuestionAndEDNS0(t *testing.T) {
	pool := newTestFakeIPPool(t, "198.18.0.0/16")
	mapper := NewEnhancer(EnhancerConfig{
		EnhancedMode:  C.DNSFakeIP,
		FakeIPPool:    pool,
		FakeIPSkipper: &fakeip.Skipper{},
		FakeIPTTL:     77,
	})
	service := NewService(nil, mapper)

	query := &D.Msg{}
	query.SetQuestion(D.Fqdn("www.example.com"), D.TypeA)
	query.SetEdns0(4096, true)

	reply, err := service.ServeMsg(context.Background(), query)
	require.NoError(t, err)

	require.Equal(t, query.Question[0], reply.Question[0])
	require.Len(t, reply.Answer, 1)
	require.EqualValues(t, 77, reply.Answer[0].Header().Ttl)
	require.True(t, reply.Response)
	require.EqualValues(t, D.RcodeSuccess, reply.Rcode)
	require.True(t, reply.RecursionAvailable)

	opt := reply.IsEdns0()
	require.NotNil(t, opt)
	require.EqualValues(t, 4096, opt.UDPSize())
	require.True(t, opt.Do())
}

// The empty answer carries the query's own header, question and EDNS0 record
// too, so a lookup that gets nothing back still echoes all three.
func TestFakeIPEmptyAnswerCarriesTheQuestionAndEDNS0(t *testing.T) {
	mapper := NewEnhancer(EnhancerConfig{
		EnhancedMode:  C.DNSFakeIP,
		FakeIPSkipper: &fakeip.Skipper{},
	})
	service := NewService(nil, mapper)

	query := &D.Msg{}
	query.SetQuestion(D.Fqdn("www.example.com"), D.TypeA)
	query.SetEdns0(4096, true)

	reply, err := service.ServeMsg(context.Background(), query)
	require.NoError(t, err)

	require.Equal(t, query.Question[0], reply.Question[0])
	require.Empty(t, reply.Answer)
	require.True(t, reply.Response)
	require.EqualValues(t, D.RcodeSuccess, reply.Rcode)
	require.True(t, reply.RecursionAvailable)

	opt := reply.IsEdns0()
	require.NotNil(t, opt)
	require.EqualValues(t, 4096, opt.UDPSize())
	require.True(t, opt.Do())
}
