package outboundgroup

import (
	"net/netip"
	"testing"

	"github.com/metacubex/mihomo/adapter"
	"github.com/metacubex/mihomo/adapter/outbound"
	"github.com/metacubex/mihomo/common/utils"
	C "github.com/metacubex/mihomo/constant"
	P "github.com/metacubex/mihomo/constant/provider"

	"github.com/stretchr/testify/require"
)

const testUrl = "https://www.gstatic.com/generate_204"

func balancedProxies(count int) []C.Proxy {
	proxies := make([]C.Proxy, 0, count)
	for i := 0; i < count; i++ {
		proxies = append(proxies, adapter.NewProxy(outbound.NewDirect()))
	}
	return proxies
}

// The proxies are indistinguishable by name, so identity is the pointer.
func indexOf(t *testing.T, proxies []C.Proxy, selected C.Proxy) int {
	t.Helper()
	for i, proxy := range proxies {
		if proxy == selected {
			return i
		}
	}
	require.Fail(t, "selected proxy is not a member of the group")
	return -1
}

func request(user, host string) *C.Metadata {
	return &C.Metadata{
		NetWork: C.TCP,
		Host:    host,
		DstPort: 443,
		SrcIP:   netip.MustParseAddr("127.0.0.1"),
		InUser:  user,
	}
}

// One unit of work walking several destinations is the case both address-derived
// keys get wrong: the group is meant to hold that work on one egress, and the
// default key changes as soon as the host changes.
func TestLoadBalanceHashKeyInUserSurvivesADestinationChange(t *testing.T) {
	proxies := balancedProxies(8)
	hosts := []string{"a.example.com", "b.example.org", "c.example.net", "d.example.io"}

	byUser := strategyConsistentHashing(testUrl, getKeyWithInUser(getKey))
	pinnedProxy, _ := byUser(proxies, request("job-1", hosts[0]), false)
	pinned := indexOf(t, proxies, pinnedProxy)
	for _, host := range hosts {
		selected, _ := byUser(proxies, request("job-1", host), false)
		require.Equal(t, pinned, indexOf(t, proxies, selected),
			"hash-key: user must ignore the destination")
	}

	// Distinct destination keys are the deterministic contract of the fallback
	// key. Distinct keys may still legally land in the same consistent-hash
	// bucket, so asserting on selected proxy indexes would make this test flaky.
	seenKeys := map[uint64]struct{}{}
	for _, host := range hosts {
		seenKeys[getKey(request("job-1", host))] = struct{}{}
	}
	require.Len(t, seenKeys, len(hosts),
		"the default key must change with the destination")
}

// Pinning must preserve distinct inbound identities. Consistent hashing may
// legally put several distinct keys in one bucket, so this test checks the
// identity key before hashing instead of asserting a random bucket spread.
func TestLoadBalanceHashKeyInUserKeepsUsersDistinct(t *testing.T) {
	keyed := getKeyWithInUser(getKey)
	users := []string{"job-1", "job-2", "job-3", "job-4", "job-5", "job-6"}

	seenKeys := map[uint64]struct{}{}
	for _, user := range users {
		seenKeys[keyed(request(user, "a.example.com"))] = struct{}{}
	}
	require.Len(t, seenKeys, len(users))
}

// Sticky sessions keys on source and destination; a client behind one source
// address cannot separate its own concurrent jobs without a supplied identity.
func TestLoadBalanceHashKeyInUserSeparatesJobsSharingASourceAddress(t *testing.T) {
	proxies := balancedProxies(8)
	strategy := strategyStickySessions(testUrl, getKeyWithInUser(getKeyWithSrcAndDst))

	firstProxy, _ := strategy(proxies, request("job-1", "a.example.com"), false)
	first := indexOf(t, proxies, firstProxy)
	require.Equal(t, first,
		func() int {
			proxy, _ := strategy(proxies, request("job-1", "b.example.org"), false)
			return indexOf(t, proxies, proxy)
		}())

	shared := strategyStickySessions(testUrl, getKeyWithSrcAndDst)
	require.Equal(t,
		func() int {
			proxy, _ := shared(proxies, request("job-1", "a.example.com"), false)
			return indexOf(t, proxies, proxy)
		}(),
		func() int {
			proxy, _ := shared(proxies, request("job-2", "a.example.com"), false)
			return indexOf(t, proxies, proxy)
		}(),
		"without a supplied key the two jobs are one session")
}

func TestStickySessionsFallsBackToAnAliveProxy(t *testing.T) {
	proxies := balancedProxies(2)
	proxies[0].(aliveStateSetter).SetAliveForTestUrl(testUrl, false)
	strategy := strategyStickySessions(testUrl, getKeyWithSrcAndDst)
	metadata := request("job-1", "a.example.com")

	for i := 0; i < 100; i++ {
		require.Same(t, proxies[1], func() C.Proxy { proxy, _ := strategy(proxies, metadata, true); return proxy }())
	}
}

// An unauthenticated request keeps the strategy's own key. Returning a constant
// instead would herd every anonymous request onto one member.
func TestLoadBalanceHashKeyInUserFallsBackWhenUnauthenticated(t *testing.T) {
	keyed := getKeyWithInUser(getKey)
	require.Equal(t, utils.MapHash("example.com"), keyed(request("", "a.example.com")))
	require.Equal(t, utils.MapHash("job-1"), keyed(request("job-1", "a.example.com")))
	require.Equal(t, getKey(nil), keyed(nil))
}

// The option name is the contract with the config file, and nothing else here
// exercises it: every other test reaches the decorator directly, so renaming
// the case would leave them all green while `hash-key: in-user` stopped working.
func TestLoadBalanceHashKeyResolvesTheOptionName(t *testing.T) {
	withInUser, err := hashKey("in-user")
	require.NoError(t, err)
	require.Equal(t, utils.MapHash("job-1"), withInUser(getKey)(request("job-1", "a.example.com")))

	identity, err := hashKey("")
	require.NoError(t, err)
	require.Equal(t, getKey(request("job-1", "a.example.com")),
		identity(getKey)(request("job-1", "a.example.com")))
}

func TestLoadBalanceKeyDoesNotAllocateForDomainHosts(t *testing.T) {
	metadata := request("", "a.example.com")

	allocations := testing.AllocsPerRun(100, func() {
		if getKey(metadata) == 0 {
			t.Fatal("domain key was zero")
		}
	})
	require.EqualValues(t, 0, allocations)
}

func TestLoadBalanceKeyHashesIPHostsDirectly(t *testing.T) {
	metadata := request("", "192.0.2.10")

	require.Equal(t,
		hashAddr(netip.MustParseAddr("192.0.2.10")),
		getKey(metadata),
	)
}

func TestLoadBalanceHashKeyRejectsUnusableConfigs(t *testing.T) {
	_, err := hashKey("session")
	require.ErrorIs(t, err, errHashKey)

	// `user` was the name this option carried before review. Rejecting it keeps
	// the rename honest: without this the case above could still read `user`
	// and every test here would stay green.
	_, err = hashKey("user")
	require.ErrorIs(t, err, errHashKey)

	_, err = NewLoadBalance(GroupCommonOption{Name: "lb"},
		LoadBalanceOption{Strategy: "round-robin", HashKey: "in-user"}, nil, nil)
	require.ErrorIs(t, err, errHashKey)

	_, err = NewLoadBalance(GroupCommonOption{Name: "lb"},
		LoadBalanceOption{Strategy: "consistent-hashing", HashKey: "nonsense"}, nil, nil)
	require.ErrorIs(t, err, errHashKey)
}

func TestLoadBalanceUnwrapTracksAlternativeState(t *testing.T) {
	proxies := balancedProxies(4)
	group, err := NewLoadBalance(
		GroupCommonOption{Name: "lb-alternative", URL: testUrl},
		LoadBalanceOption{Strategy: "sticky-sessions"},
		nil,
		[]P.ProxyProvider{failoverProvider(t, proxies)},
	)
	require.NoError(t, err)
	metadata := request("job-1", "a.example.com")

	require.True(t, group.Unwrap(metadata, false) != nil)
	require.True(t, group.hasAlternative.Load())

	for _, proxy := range proxies[1:] {
		proxy.(aliveStateSetter).SetAliveForTestUrl(testUrl, false)
	}
	require.NotNil(t, group.Unwrap(metadata, false))
	require.False(t, group.hasAlternative.Load())
}

func BenchmarkLoadBalanceUnwrapSticky(b *testing.B) {
	proxies := balancedProxies(64)
	group, err := NewLoadBalance(
		GroupCommonOption{Name: "lb-unwrap", URL: testUrl},
		LoadBalanceOption{Strategy: "sticky-sessions"},
		nil,
		[]P.ProxyProvider{failoverProvider(b, proxies)},
	)
	require.NoError(b, err)
	metadata := request("", "a.example.com")
	metadata.SrcIP = netip.MustParseAddr("192.0.2.1")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		if group.Unwrap(metadata, false) == nil {
			b.Fatal("nil proxy")
		}
	}
}

func BenchmarkStickySessionsProxyPick(b *testing.B) {
	b.ReportAllocs()

	proxies := balancedProxies(64)
	strategy := strategyStickySessions(testUrl, getKeyWithSrcAndDst)
	srcIPs := []netip.Addr{
		netip.MustParseAddr("192.0.2.1"),
		netip.MustParseAddr("192.0.2.2"),
		netip.MustParseAddr("192.0.2.3"),
		netip.MustParseAddr("192.0.2.4"),
	}
	hosts := []string{
		"a.example.com",
		"b.example.org",
		"c.example.net",
		"d.example.io",
	}

	for i := 0; b.Loop(); i++ {
		metadata := request("", hosts[i%len(hosts)])
		metadata.SrcIP = srcIPs[(i/len(hosts))%len(srcIPs)]
		if proxy, _ := strategy(proxies, metadata, false); proxy == nil {
			b.Fatal("nil proxy")
		}
	}
}
