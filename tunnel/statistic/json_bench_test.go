package statistic

import (
	"strings"
	"testing"

	"github.com/metacubex/mihomo/common/jsonutil"
)

func benchmarkAppendJSONString(b *testing.B, value string) {
	dst := make([]byte, 0, len(value)+64)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dst = jsonutil.AppendString(dst[:0], value)
	}
}

func BenchmarkAppendConnectionJSONStringSafeASCII256(b *testing.B) {
	benchmarkAppendJSONString(b, strings.Repeat("abcdefghijklmnopqrstuvwxyz012345", 8))
}

func BenchmarkAppendConnectionJSONStringEscaped32(b *testing.B) {
	benchmarkAppendJSONString(b, strings.Repeat(`a"b`, 11))
}

func BenchmarkAppendConnectionJSONStringUnicode32(b *testing.B) {
	benchmarkAppendJSONString(b, strings.Repeat("代理测试", 8))
}
