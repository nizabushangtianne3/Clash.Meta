package outboundgroup

import (
	"testing"

	"github.com/metacubex/mihomo/adapter"
	"github.com/metacubex/mihomo/adapter/outbound"
	C "github.com/metacubex/mihomo/constant"
	P "github.com/metacubex/mihomo/constant/provider"

	"github.com/stretchr/testify/require"
)

// A list is stamped with the versions read beside it. Reading the version
// after the lists would stamp a provider that swapped during the rebuild with
// its newer version, and the list that read saw would be served from the cache
// for the rest of the run.
func TestGetProxiesDoesNotCacheAListItsVersionOutran(t *testing.T) {
	first := adapter.NewProxy(outbound.NewDirectWithOption(outbound.DirectOption{Name: "first"}))
	second := adapter.NewProxy(outbound.NewDirectWithOption(outbound.DirectOption{Name: "second"}))
	emptyFallback := adapter.NewProxy(outbound.NewDirectWithOption(outbound.DirectOption{Name: "COMPATIBLE"}))
	pd := newSwapProvider("swap", []C.Proxy{first})

	group, err := NewSelector(
		GroupCommonOption{Name: "group"},
		SelectorOption{},
		emptyFallback,
		[]P.ProxyProvider{pd},
	)
	require.NoError(t, err)

	// The rebuild reads a list the swap already replaced underneath it.
	pd.swapDuringRead([]C.Proxy{second})
	read := group.GetProxies(false)
	require.Len(t, read, 1)
	require.Equal(t, "first", read[0].Name(), "the read saw the list it was handed")

	again := group.GetProxies(false)
	require.Len(t, again, 1)
	require.Equal(t, "second", again[0].Name(),
		"a list whose version outran it was cached for the rest of the run")
}
