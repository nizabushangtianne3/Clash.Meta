package adapter

import "testing"

func BenchmarkProxyAppendJSONEmptyExtra(b *testing.B) {
	proxy := testProxy()
	dst := make([]byte, 0, 2048)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dst = proxy.AppendJSON(dst[:0])
		if len(dst) == 0 {
			b.Fatal("empty proxy JSON")
		}
	}
}
