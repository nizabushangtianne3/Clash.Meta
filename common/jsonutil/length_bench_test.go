package jsonutil

import (
	"strings"
	"testing"
)

func benchmarkStringLen(b *testing.B, value string) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if StringLen(value) != len(value)+2 {
			b.Fatal("unexpected safe string size")
		}
	}
}

func BenchmarkStringLenSafeASCII32(b *testing.B) {
	benchmarkStringLen(b, strings.Repeat("a", 31)+".")
}

func BenchmarkStringLenSafeASCII256(b *testing.B) {
	benchmarkStringLen(b, strings.Repeat("abcdefghijklmnopqrstuvwxyz012345", 8))
}

func BenchmarkStringLenEscaped32(b *testing.B) {
	value := strings.Repeat(`a"b`, 11)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if StringLen(value) != len(value)+13 {
			b.Fatal("unexpected escaped string size")
		}
	}
}

func BenchmarkStringLenUnicode32(b *testing.B) {
	value := strings.Repeat("代理测试", 8)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if StringLen(value) != 96+2 {
			b.Fatal("unexpected unicode string size")
		}
	}
}
