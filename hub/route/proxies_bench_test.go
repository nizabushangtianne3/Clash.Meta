package route

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	mhttptest "github.com/metacubex/http/httptest"

	"github.com/metacubex/mihomo/adapter"
	"github.com/metacubex/mihomo/adapter/outbound"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/tunnel"
)

func benchmarkProxyNames(size int) map[string]C.Proxy {
	proxies := make(map[string]C.Proxy, size+1)
	proxies["DIRECT"] = adapter.NewProxy(outbound.NewDirectWithOption(outbound.DirectOption{Name: "DIRECT"}))
	for index := range size {
		name := fmt.Sprintf("node-%04d", index)
		proxies[name] = adapter.NewProxy(outbound.NewDirectWithOption(outbound.DirectOption{Name: name}))
	}
	return proxies
}

func BenchmarkGetProxiesResponse(b *testing.B) {
	proxies := benchmarkProxyNames(64)
	old := tunnel.Proxies()
	tunnel.UpdateProxies(proxies, nil)
	b.Cleanup(func() { tunnel.UpdateProxies(old, nil) })

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		recorder := mhttptest.NewRecorder()
		request := mhttptest.NewRequest("GET", "/proxies", nil)
		getProxies(recorder, request)
		if recorder.Code != 200 {
			b.Fatalf("status = %d", recorder.Code)
		}
	}
}
func TestAppendProxyDocumentSortsKeysAndPreservesDocuments(t *testing.T) {
	proxies := benchmarkProxyNames(3)
	proxies[`quote"html<&>`] = proxies["DIRECT"]

	document := appendProxyDocument(nil, proxies)
	var view struct {
		Proxies map[string]json.RawMessage `json:"proxies"`
	}
	require.NoError(t, json.Unmarshal(document, &view))
	require.Len(t, view.Proxies, len(proxies))

	var names []string
	for name := range view.Proxies {
		names = append(names, name)
	}
	require.ElementsMatch(t, []string{"DIRECT", "node-0000", "node-0001", "node-0002", "quote\"html<&>"}, names)
	require.Contains(t, string(document), `"quote\"html\u003c\u0026\u003e"`)
}
