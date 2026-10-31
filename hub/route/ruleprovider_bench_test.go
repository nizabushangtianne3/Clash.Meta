package route

import (
	"fmt"
	"testing"

	mhttptest "github.com/metacubex/http/httptest"
	P "github.com/metacubex/mihomo/constant/provider"
	"github.com/metacubex/mihomo/rules/provider"
	"github.com/metacubex/mihomo/tunnel"
)

func benchmarkRuleProviders(providerCount, ruleCount int) map[string]P.RuleProvider {
	providers := make(map[string]P.RuleProvider, providerCount)
	for providerIndex := range providerCount {
		payload := make([]string, ruleCount)
		for ruleIndex := range payload {
			payload[ruleIndex] = fmt.Sprintf("DOMAIN-SUFFIX,r%d-%04d.example.com,DIRECT", providerIndex, ruleIndex)
		}
		name := fmt.Sprintf("rule-provider-%02d", providerIndex)
		providers[name] = provider.NewInlineProvider(
			name, P.Domain, payload, nil)
	}
	return providers
}

func BenchmarkGetRuleProvidersResponse(b *testing.B) {
	providers := benchmarkRuleProviders(8, 256)
	old := tunnel.RuleProviders()
	tunnel.UpdateRules(nil, nil, providers)
	b.Cleanup(func() { tunnel.UpdateRules(nil, nil, old) })

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		recorder := mhttptest.NewRecorder()
		request := mhttptest.NewRequest("GET", "/providers/rules", nil)
		getRuleProviders(recorder, request)
		if recorder.Code != 200 {
			b.Fatalf("status = %d", recorder.Code)
		}
	}
}
