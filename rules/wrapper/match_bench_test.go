package wrapper

import (
	"testing"

	C "github.com/metacubex/mihomo/constant"
)

type benchmarkRule struct {
	matched bool
}

func (r *benchmarkRule) RuleType() C.RuleType { return C.Domain }
func (r *benchmarkRule) Match(*C.Metadata, C.RuleMatchHelper) (bool, string) {
	return r.matched, "DIRECT"
}
func (r *benchmarkRule) Adapter() string         { return "DIRECT" }
func (r *benchmarkRule) Payload() string         { return "counter.example" }
func (r *benchmarkRule) ProviderNames() []string { return nil }

func benchmarkMatch(b *testing.B, matched bool) {
	wrapper := NewRuleWrapper(&benchmarkRule{matched: matched}).(*RuleWrapper)
	metadata := &C.Metadata{Host: "counter.example"}
	helper := C.RuleMatchHelper{}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ok, _ := wrapper.Match(metadata, helper)
		if ok != matched {
			b.Fatal("unexpected match result")
		}
	}
}

func BenchmarkRuleWrapperHit(b *testing.B)  { benchmarkMatch(b, true) }
func BenchmarkRuleWrapperMiss(b *testing.B) { benchmarkMatch(b, false) }
