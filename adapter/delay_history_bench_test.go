package adapter

import (
	"testing"
	"time"

	C "github.com/metacubex/mihomo/constant"
)

func BenchmarkProxyDelayHistory(b *testing.B) {
	proxy := testProxy()
	for range 10 {
		proxy.history.Put(C.DelayHistory{Time: time.Now(), Delay: 100})
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if history := proxy.DelayHistory(); len(history) != 10 {
			b.Fatal("short history")
		}
	}
}
