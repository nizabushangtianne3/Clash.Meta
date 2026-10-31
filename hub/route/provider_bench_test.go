package route

import (
	"fmt"

	C "github.com/metacubex/mihomo/constant"
	"testing"

	mhttptest "github.com/metacubex/http/httptest"
	"github.com/metacubex/mihomo/adapter"
	"github.com/metacubex/mihomo/adapter/outbound"
	"github.com/metacubex/mihomo/adapter/provider"
	P "github.com/metacubex/mihomo/constant/provider"
	"github.com/metacubex/mihomo/tunnel"
)

func benchmarkProxyProviders(providerCount, proxyCount int) map[string]P.ProxyProvider {
	providers := make(map[string]P.ProxyProvider, providerCount)
	for providerIndex := range providerCount {
		proxies := make([]C.Proxy, proxyCount)
		for proxyIndex := range proxies {
			name := fmt.Sprintf("p%d-node-%04d", providerIndex, proxyIndex)
			proxies[proxyIndex] = adapter.NewProxy(outbound.NewDirectWithOption(outbound.DirectOption{Name: name}))
		}
		health := provider.NewHealthCheck(proxies, "", 0, 0, false, nil)
		name := fmt.Sprintf("provider-%02d", providerIndex)
		pd, err := provider.NewCompatibleProvider(name, proxies, health)
		if err != nil {
			panic(err)
		}
		providers[name] = pd
	}
	return providers
}

func BenchmarkGetProvidersResponse(b *testing.B) {
	providers := benchmarkProxyProviders(8, 64)
	old := tunnel.Providers()
	tunnel.UpdateProxies(nil, providers)
	b.Cleanup(func() { tunnel.UpdateProxies(nil, old) })

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		recorder := mhttptest.NewRecorder()
		request := mhttptest.NewRequest("GET", "/providers/proxies", nil)
		getProviders(recorder, request)
		if recorder.Code != 200 {
			b.Fatalf("status = %d", recorder.Code)
		}
	}
}
