package outboundgroup

import (
	"testing"

	"github.com/metacubex/mihomo/adapter"
	"github.com/metacubex/mihomo/adapter/outbound"
	C "github.com/metacubex/mihomo/constant"
	P "github.com/metacubex/mihomo/constant/provider"

	"github.com/stretchr/testify/require"
)

// touchProvider records the touches the group hands to it, so a test can tell
// a read that touched from one that did not.
type touchProvider struct {
	P.ProxyProvider
	touches int
}

func (p *touchProvider) Name() string               { return "p1" }
func (p *touchProvider) VehicleType() P.VehicleType { return P.File }
func (p *touchProvider) Version() uint32            { return 1 }
func (p *touchProvider) Proxies() []C.Proxy {
	return []C.Proxy{namedProxy("p1-hk"), namedProxy("p1-us")}
}
func (p *touchProvider) Touch() { p.touches++ }

// A read that touches hands the touch to every provider behind the group, and
// one that does not leaves them alone: the touch is the caller's own
// liveness signal, so it must not travel unasked.
func TestGetProxiesTouchesOnlyWhenAsked(t *testing.T) {
	prov := &touchProvider{}
	group := NewGroupBase(GroupBaseOption{
		Name:          "Group",
		Type:          C.Selector,
		EmptyFallback: namedProxy("COMPATIBLE"),
		Providers:     []P.ProxyProvider{prov},
	})

	group.GetProxies(false)
	require.Zero(t, prov.touches, "a read that did not ask touched anyway")

	group.GetProxies(true)
	require.Equal(t, 1, prov.touches, "a touch read touched once")

	group.GetProxies(true)
	require.Equal(t, 2, prov.touches, "every touch read touches")
}

// A filter keeps the members it matches, and a provider the config declared
// compatible is read unfiltered: those already came filtered from wherever the
// operator built them.
func TestGetProxiesHonoursTheFilterAndLeavesCompatibleAlone(t *testing.T) {
	filtered := &filterProvider{
		fakeProvider: &fakeProvider{name: "p1", proxies: []C.Proxy{namedProxy("p1-hk"), namedProxy("p1-us")}},
		vehicle:      P.File,
	}
	compatible := &filterProvider{
		fakeProvider: &fakeProvider{name: "p2", proxies: []C.Proxy{namedProxy("p2-hk"), namedProxy("p2-us")}},
		vehicle:      P.Compatible,
	}
	group := NewGroupBase(GroupBaseOption{
		Name:          "Group",
		Type:          C.Selector,
		EmptyFallback: namedProxy("COMPATIBLE"),
		Filter:        "hk",
		Providers:     []P.ProxyProvider{filtered, compatible},
	})

	var names []string
	for _, proxy := range group.GetProxies(false) {
		names = append(names, proxy.Name())
	}
	require.Contains(t, names, "p1-hk", "the filter keeps what it matches")
	require.NotContains(t, names, "p1-us", "the filter drops what it does not match")
	require.Contains(t, names, "p2-hk", "the compatible provider is read as it stands")
	require.Contains(t, names, "p2-us",
		"and its unmatched members survive, because nobody filtered them")
}

type filterProvider struct {
	*fakeProvider
	vehicle P.VehicleType
}

func (f *filterProvider) VehicleType() P.VehicleType { return f.vehicle }

// Two filters over two providers reorder the joined list by filter order, so a
// member the earlier filter named comes first even though its provider lists
// it later.
func TestGetProxiesOrdersTheJoinedListByFilterOrder(t *testing.T) {
	first := &fakeProvider{name: "p1", proxies: []C.Proxy{namedProxy("p1-us"), namedProxy("p1-hk")}}
	second := &fakeProvider{name: "p2", proxies: []C.Proxy{namedProxy("p2-us"), namedProxy("p2-hk")}}
	group := NewGroupBase(GroupBaseOption{
		Name:          "Group",
		Type:          C.Selector,
		EmptyFallback: namedProxy("COMPATIBLE"),
		Filter:        "hk`us",
		Providers:     []P.ProxyProvider{first, second},
	})

	var names []string
	for _, proxy := range group.GetProxies(false) {
		names = append(names, proxy.Name())
	}
	require.Equal(t, []string{"p1-hk", "p2-hk", "p1-us", "p2-us"}, names,
		"the joins are ordered by filter, not by provider")
}

// An exclude filter drops the members it names wherever they came from, and an
// exclude type drops a whole adapter type from the table.
func TestGetProxiesDropsWhatTheExcludesName(t *testing.T) {
	prov := &fakeProvider{name: "p1", proxies: []C.Proxy{namedProxy("p1-hk"), namedProxy("p1-us"), namedProxy("p1-sg")}}
	group := NewGroupBase(GroupBaseOption{
		Name:          "Group",
		Type:          C.Selector,
		EmptyFallback: namedProxy("COMPATIBLE"),
		ExcludeFilter: "sg`us",
		Providers:     []P.ProxyProvider{prov},
	})

	var names []string
	for _, proxy := range group.GetProxies(false) {
		names = append(names, proxy.Name())
	}
	require.Equal(t, []string{"p1-hk"}, names, "both exclude filters drop their match")

	typed := &fakeProvider{name: "p1", proxies: []C.Proxy{namedProxy("p1-hk"), adapter.NewProxy(outbound.NewReject())}}
	byType := NewGroupBase(GroupBaseOption{
		Name:          "Group",
		Type:          C.Selector,
		EmptyFallback: namedProxy("COMPATIBLE"),
		ExcludeType:   "Direct",
		Providers:     []P.ProxyProvider{typed},
	})
	names = nil
	for _, proxy := range byType.GetProxies(false) {
		names = append(names, proxy.Name())
	}
	require.Len(t, names, 1, "the excluded type leaves the rest")
	require.Equal(t, "REJECT", names[0])
}

// A filter that matches nothing leaves the group empty, and an empty group
// answers with the fallback it was built with rather than nothing.
func TestGetProxiesFallsBackWhenEveryMemberIsFilteredOut(t *testing.T) {
	prov := &fakeProvider{name: "p1", proxies: []C.Proxy{namedProxy("p1-hk")}}
	group := NewGroupBase(GroupBaseOption{
		Name:          "Group",
		Type:          C.Selector,
		EmptyFallback: namedProxy("FALLBACK"),
		Filter:        "nothing-matches-this",
		Providers:     []P.ProxyProvider{prov},
	})

	members := group.GetProxies(false)
	require.Len(t, members, 1, "an empty group answers one member")
	require.Equal(t, "FALLBACK", members[0].Name())
}
