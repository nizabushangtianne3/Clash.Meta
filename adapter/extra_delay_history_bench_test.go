package adapter

import (
	"testing"
	"time"

	C "github.com/metacubex/mihomo/constant"
)

func BenchmarkProxyExtraDelayHistories(b *testing.B) {
	proxy := testProxy()
	url := "https://bench.example/"
	proxy.SetAliveForTestUrl(url, true)
	for range 10 {
		state, _ := proxy.extra.Load(url)
		state.history.PutBounded(defaultHistoriesNum, C.DelayHistory{Time: time.Now(), Delay: 100})
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if histories := proxy.ExtraDelayHistories(); len(histories[url].History) != 10 {
			b.Fatal("short extra history")
		}
	}
}
