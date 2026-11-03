package jsonutil

import (
	"strings"
	"testing"
)

func benchmarkAppendString(b *testing.B, value string) {
	dst := make([]byte, 0, len(value)+64)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dst = AppendString(dst[:0], value)
	}
}

func BenchmarkAppendStringSafeASCII32(b *testing.B) {
	benchmarkAppendString(b, strings.Repeat("a", 31)+".")
}

func BenchmarkAppendStringSafeASCII256(b *testing.B) {
	benchmarkAppendString(b, strings.Repeat("abcdefghijklmnopqrstuvwxyz012345", 8))
}

func BenchmarkAppendStringEscaped32(b *testing.B) {
	benchmarkAppendString(b, strings.Repeat(`a"b`, 11))
}

func BenchmarkAppendStringUnicode32(b *testing.B) {
	benchmarkAppendString(b, strings.Repeat("代理测试", 8))
}
