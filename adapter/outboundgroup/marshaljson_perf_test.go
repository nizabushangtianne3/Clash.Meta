package outboundgroup

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/metacubex/mihomo/adapter"
	"github.com/metacubex/mihomo/adapter/outbound"
	C "github.com/metacubex/mihomo/constant"
	P "github.com/metacubex/mihomo/constant/provider"
	"github.com/stretchr/testify/require"
)

func groupViewMembers(tb testing.TB, size int) []C.Proxy {
	tb.Helper()
	proxies := make([]C.Proxy, size)
	for index := range proxies {
		proxies[index] = adapter.NewProxy(outbound.NewDirectWithOption(outbound.DirectOption{
			Name: fmt.Sprintf("node-%04d", index),
		}))
	}
	return proxies
}

// The host embeds a group document verbatim, so the key set and the name it
// carries are the contract this file has to keep true.
func groupViewGroups(tb testing.TB, size int) map[string]C.Proxy {
	tb.Helper()
	proxies := groupViewMembers(tb, size)
	compatible := adapter.NewProxy(outbound.NewDirectWithOption(outbound.DirectOption{Name: "COMPATIBLE"}))
	providers := []P.ProxyProvider{failoverProvider(tb, proxies)}

	selector, err := NewSelector(
		GroupCommonOption{Name: "selector-view"},
		SelectorOption{},
		compatible,
		providers,
	)
	require.NoError(tb, err)

	urlTest, err := NewURLTest(
		GroupCommonOption{Name: "url-test-view", URL: "https://example.com/generate_204"},
		URLTestOption{},
		compatible,
		providers,
	)
	require.NoError(tb, err)

	fallback, err := NewFallback(
		GroupCommonOption{Name: "fallback-view", URL: "https://example.com/generate_204"},
		FallbackOption{},
		compatible,
		providers,
	)
	require.NoError(tb, err)

	loadBalance, err := NewLoadBalance(
		GroupCommonOption{Name: "load-balance-view"},
		LoadBalanceOption{Strategy: "round-robin"},
		compatible,
		providers,
	)
	require.NoError(tb, err)

	return map[string]C.Proxy{
		"Selector":    adapter.NewProxy(selector),
		"URLTest":     adapter.NewProxy(urlTest),
		"Fallback":    adapter.NewProxy(fallback),
		"LoadBalance": adapter.NewProxy(loadBalance),
	}
}

func TestGroupViewKeySet(t *testing.T) {
	groups := groupViewGroups(t, 3)
	want := map[string][]string{
		"Selector":    {"type", "name", "now", "all", "testUrl", "hidden", "icon", "emptyFallback"},
		"URLTest":     {"type", "name", "now", "all", "testUrl", "expectedStatus", "fixed", "hidden", "icon", "emptyFallback"},
		"Fallback":    {"type", "name", "now", "all", "testUrl", "expectedStatus", "fixed", "hidden", "icon", "emptyFallback"},
		"LoadBalance": {"type", "name", "all", "testUrl", "expectedStatus", "hidden", "icon", "emptyFallback"},
	}

	for kind, proxy := range groups {
		data, err := proxy.Adapter().MarshalJSON()
		require.NoError(t, err, kind)

		view := map[string]json.RawMessage{}
		require.NoError(t, json.Unmarshal(data, &view), kind)
		require.Len(t, view, len(want[kind]), "%s keys: %s", kind, data)
		for _, key := range want[kind] {
			require.Contains(t, view, key, "%s is missing %q: %s", kind, key, data)
		}
		require.JSONEq(t, `"`+proxy.Name()+`"`, string(view["name"]), kind)
	}
}

func BenchmarkGroupViewMarshalOnly(b *testing.B) {
	for _, size := range []int{20, 200} {
		b.Run(fmt.Sprintf("members=%d", size), func(b *testing.B) {
			group, _ := selectorBenchmarkGroup(b, size)
			proxy := adapter.NewProxy(group)

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := proxy.Adapter().MarshalJSON(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// The host encodes the document it received straight into its response, so the
// payload is what a group marshal actually costs downstream.
func BenchmarkGroupViewEncodedPayload(b *testing.B) {
	type payload struct {
		Proxies map[string]any `json:"proxies"`
	}
	for _, size := range []int{20, 200} {
		b.Run(fmt.Sprintf("members=%d", size), func(b *testing.B) {
			group, _ := selectorBenchmarkGroup(b, size)
			proxy := adapter.NewProxy(group)

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				data, err := proxy.Adapter().MarshalJSON()
				if err != nil {
					b.Fatal(err)
				}
				if _, err := json.Marshal(payload{
					Proxies: map[string]any{"group": json.RawMessage(data)},
				}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
