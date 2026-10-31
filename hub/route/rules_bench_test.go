package route

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	mhttptest "github.com/metacubex/http/httptest"
	C "github.com/metacubex/mihomo/constant"
	rulesCommon "github.com/metacubex/mihomo/rules/common"
	"github.com/metacubex/mihomo/rules/wrapper"
	"github.com/metacubex/mihomo/tunnel"
	"github.com/stretchr/testify/require"
)

func TestAppendRulesDocumentMatchesAPIContract(t *testing.T) {
	rules := []C.Rule{wrapper.NewRuleWrapper(rulesCommon.NewDomain("example.com", "DIRECT"))}

	document := string(appendRulesDocument(nil, rules))
	var view struct {
		Rules []struct {
			Index   int    `json:"index"`
			Type    string `json:"type"`
			Payload string `json:"payload"`
			Proxy   string `json:"proxy"`
			Size    int    `json:"size"`
			Extra   *struct {
				Disabled  bool      `json:"disabled"`
				HitCount  uint64    `json:"hitCount"`
				HitAt     time.Time `json:"hitAt"`
				MissCount uint64    `json:"missCount"`
				MissAt    time.Time `json:"missAt"`
			} `json:"extra"`
		} `json:"rules"`
	}
	require.NoError(t, json.Unmarshal([]byte(document), &view))
	require.Len(t, view.Rules, 1)

	rule := view.Rules[0]
	require.Equal(t, 0, rule.Index)
	require.Equal(t, "Domain", rule.Type)
	require.Equal(t, "example.com", rule.Payload)
	require.Equal(t, "DIRECT", rule.Proxy)
	require.Equal(t, -1, rule.Size)
	require.NotNil(t, rule.Extra)
	require.False(t, rule.Extra.Disabled)
	require.Zero(t, rule.Extra.HitCount)
	require.Zero(t, rule.Extra.MissCount)
}

func benchmarkRulesResponse(b *testing.B, count int) {
	rules := make([]C.Rule, count)
	for index := range rules {
		rules[index] = wrapper.NewRuleWrapper(rulesCommon.NewDomain(
			fmt.Sprintf("rule-%04d.example.com", index), "DIRECT"))
	}
	old := tunnel.Rules()
	tunnel.UpdateRules(rules, nil, nil)
	b.Cleanup(func() { tunnel.UpdateRules(old, nil, nil) })

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		recorder := mhttptest.NewRecorder()
		request := mhttptest.NewRequest("GET", "/rules", nil)
		getRules(recorder, request)
		if recorder.Code != 200 {
			b.Fatalf("status = %d", recorder.Code)
		}
	}
}

func BenchmarkGetRulesResponse1000(b *testing.B) { benchmarkRulesResponse(b, 1000) }
func BenchmarkGetRulesResponse5000(b *testing.B) { benchmarkRulesResponse(b, 5000) }
