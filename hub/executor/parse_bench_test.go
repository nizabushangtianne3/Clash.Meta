package executor_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/metacubex/mihomo/hub/executor"
)

// A subscription a user switches to, which is the part of a config switch the
// user waits for: the parse that turns it into a live config.
func benchmarkParseConfig(b *testing.B, proxyCount int) []byte {
	b.Helper()

	var document strings.Builder
	document.WriteString("mixed-port: 7890\nproxies:\n")
	for index := range proxyCount {
		fmt.Fprintf(&document,
			"  - {name: node-%04d, type: ss, server: 10.%d.%d.1, port: 443, cipher: aes-128-gcm, password: password}\n",
			index, index>>8, index&0xff)
	}
	document.WriteString("proxy-groups:\n  - {name: Proxy, type: select, proxies: [")
	for index := range proxyCount {
		if index > 0 {
			document.WriteString(", ")
		}
		fmt.Fprintf(&document, "node-%04d", index)
	}
	document.WriteString("]}\nrules:\n  - MATCH,DIRECT\n")
	return []byte(document.String())
}

func BenchmarkParseConfig(b *testing.B) {
	for _, proxyCount := range []int{200, 800} {
		b.Run(fmt.Sprintf("proxies=%d", proxyCount), func(b *testing.B) {
			buf := benchmarkParseConfig(b, proxyCount)

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := executor.ParseWithBytes(buf); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
