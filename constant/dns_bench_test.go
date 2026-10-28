package constant

import "testing"

var dnsMarshalSink []byte

func BenchmarkDNSModeMarshalText(b *testing.B) {
	benchmarks := []struct {
		name string
		mode DNSMode
	}{
		{name: "normal", mode: DNSNormal},
		{name: "fake-ip", mode: DNSFakeIP},
		{name: "redir-host", mode: DNSMapping},
		{name: "hosts", mode: DNSHosts},
	}
	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				text, err := bm.mode.MarshalText()
				if err != nil {
					b.Fatal(err)
				}
				dnsMarshalSink = text
			}
		})
	}
}
