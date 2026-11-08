package outboundgroup

import (
	"fmt"
	"testing"

	"github.com/metacubex/mihomo/adapter"
	"github.com/metacubex/mihomo/adapter/outbound"
	C "github.com/metacubex/mihomo/constant"
	P "github.com/metacubex/mihomo/constant/provider"
	"github.com/stretchr/testify/require"
)

func selectorBenchmarkGroup(tb testing.TB, size int) (*Selector, C.Proxy) {
	tb.Helper()

	proxies := make([]C.Proxy, size)
	for index := range proxies {
		proxies[index] = adapter.NewProxy(outbound.NewDirectWithOption(outbound.DirectOption{
			Name: fmt.Sprintf("node-%03d", index),
		}))
	}

	group, err := NewSelector(
		GroupCommonOption{Name: "selector-index"},
		SelectorOption{},
		adapter.NewProxy(outbound.NewDirectWithOption(outbound.DirectOption{Name: "COMPATIBLE"})),
		[]P.ProxyProvider{failoverProvider(tb, proxies)},
	)
	require.NoError(tb, err)
	require.NoError(tb, group.Set(proxies[len(proxies)-1].Name()))
	return group, proxies[len(proxies)-1]
}

func TestSelectorSelectedProxyUsesNameIndex(t *testing.T) {
	group, selected := selectorBenchmarkGroup(t, 64)

	require.Equal(t, selected.Name(), group.selectedProxy(false).Name())
	allocations := testing.AllocsPerRun(100, func() {
		if group.selectedProxy(false).Name() != selected.Name() {
			t.Fatal("selected proxy mismatch")
		}
	})
	require.EqualValues(t, 0, allocations)
}

// A host that caches a group's document keys it on this counter beside the
// provider versions instead of resolving the selection on every read, so every
// way a selection can move has to move the counter -- including a re-select of
// the name already selected, which the counter cannot tell from a real change.
func TestSelectorSelectionVersionMovesOnEverySelectionChange(t *testing.T) {
	group, _ := selectorBenchmarkGroup(t, 8)

	start := group.SelectionVersion()
	require.NoError(t, group.Set("node-003"))
	require.Equal(t, start+1, group.SelectionVersion())
	require.Equal(t, "node-003", group.Now())

	group.ForceSet("node-005")
	require.Equal(t, start+2, group.SelectionVersion())
	require.Equal(t, "node-005", group.Now())

	require.NoError(t, group.Set("node-005"))
	require.Equal(t, start+3, group.SelectionVersion(),
		"a re-select of the name already selected still moves the counter")

	group.ForceSet("node-000")
	require.Equal(t, start+4, group.SelectionVersion())
	require.Equal(t, "node-000", group.Now())
}

func TestSelectorSmallGroupKeepsLinearSelection(t *testing.T) {
	group, selected := selectorBenchmarkGroup(t, 2)
	require.Nil(t, group.providerCache.Load().byName)
	require.Equal(t, selected.Name(), group.selectedProxy(false).Name())
}

func BenchmarkSelectorSelectedProxyLargeGroup(b *testing.B) {
	group, selected := selectorBenchmarkGroup(b, 4096)
	want := selected.Name()

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if group.selectedProxy(false).Name() != want {
				b.Fatal("selected proxy mismatch")
			}
		}
	})
}
