package dialer

import (
	"context"
	"net/netip"
	"testing"

	resolver "github.com/metacubex/mihomo/component/resolver"
	D "github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

type cacheResolver struct {
	ips []netip.Addr
}

func (cacheResolver) LookupIP(context.Context, string) ([]netip.Addr, error) {
	return nil, resolver.ErrIPNotFound
}

func (r cacheResolver) LookupIPv4(context.Context, string) ([]netip.Addr, error) {
	return r.ips, nil
}

func (cacheResolver) LookupIPv6(context.Context, string) ([]netip.Addr, error) {
	return nil, resolver.ErrIPv6Disabled
}

func (cacheResolver) ResolveECH(context.Context, string) ([]byte, error) {
	return nil, resolver.ErrIPNotFound
}

func (cacheResolver) ExchangeContext(context.Context, *D.Msg) (*D.Msg, error) {
	return nil, resolver.ErrIPNotFound
}

func (cacheResolver) Invalid() bool    { return true }
func (cacheResolver) ClearCache()      {}
func (cacheResolver) ResetConnection() {}

func TestParseAddrClonesBeforeRewritingSharedCache(t *testing.T) {
	source := netip.MustParseAddr("::ffff:192.0.2.1")
	resolver := cacheResolver{ips: []netip.Addr{source}}

	ips, port, err := parseAddr(context.Background(), "tcp4", "192.0.2.1:443", resolver)
	require.NoError(t, err)
	require.EqualValues(t, 443, port)
	require.Equal(t, []netip.Addr{netip.MustParseAddr("192.0.2.1")}, ips)
	require.Equal(t, source, resolver.ips[0], "shared cache entry was rewritten")
}
