package tunnel

import (
	"net/netip"
	"testing"

	C "github.com/metacubex/mihomo/constant"
	rulesCommon "github.com/metacubex/mihomo/rules/common"
	"github.com/stretchr/testify/require"
)

// The route a connection would take is answerable without dialing it, which is
// what a host that wants to name the adapter asks for. The same steps an
// incoming connection takes resolve it, so the match table decides.
func TestResolveRouteNamesTheAdapterTheRulesPick(t *testing.T) {
	UpdateProxies(map[string]C.Proxy{"PROXY": directTestProxy{}}, nil)
	SetMode(Rule)
	UpdateRules([]C.Rule{rulesCommon.NewMatch("PROXY")}, nil, nil)
	t.Cleanup(func() {
		SetMode(Rule)
		UpdateProxies(nil, nil)
		UpdateRules(nil, nil, nil)
		invalidateAllProxies()
	})

	metadata := &C.Metadata{
		NetWork: C.TCP,
		Type:    C.INNER,
		DstIP:   netip.MustParseAddr("127.0.0.1"),
		DstPort: 443,
	}
	proxy, err := ResolveRoute(metadata)
	require.NoError(t, err)
	require.NotNil(t, proxy)
	require.Equal(t, "DIRECT", proxy.Name(),
		"the match rule names PROXY, and the proxy held under it answers")

	// An inner connection carries no process, and none is looked up for it.
	metadata.Type = C.INNER
	_, err = ResolveRoute(metadata)
	require.NoError(t, err)
}

// A table with no rule and no proxy is reported, not answered with a guess.
func TestResolveRouteReportsATableWithNothingInIt(t *testing.T) {
	SetMode(Rule)
	UpdateProxies(nil, nil)
	UpdateRules(nil, nil, nil)
	t.Cleanup(func() {
		SetMode(Rule)
		invalidateAllProxies()
	})

	proxy, err := ResolveRoute(&C.Metadata{
		NetWork: C.TCP,
		Type:    C.INNER,
		DstIP:   netip.MustParseAddr("127.0.0.1"),
		DstPort: 443,
	})
	if err == nil {
		require.Nil(t, proxy, "an empty table names no adapter")
	}
}
