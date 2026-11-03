package constant

import "testing"

func BenchmarkAdapterTypeString(b *testing.B) {
	for _, at := range []AdapterType{Direct, Shadowsocks, URLTest, GostRelay} {
		b.Run(at.String(), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if at.String() == "" {
					b.Fatal("empty type")
				}
			}
		})
	}
}
