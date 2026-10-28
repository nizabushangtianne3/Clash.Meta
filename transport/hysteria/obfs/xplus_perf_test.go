//go:build !race

package obfs

import (
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKeyAndSaltSumMatchesTheDigest(t *testing.T) {
	salt := make([]byte, saltLen)
	for _, key := range [][]byte{nil, make([]byte, 48), make([]byte, 49)} {
		sum := keyAndSaltSum(key, salt)
		want := sha256.Sum256(append(append([]byte{}, key...), salt...))
		require.Equal(t, want, sum, len(key))
	}
}

func TestXPlusObfuscatorStackKeysAllocateNothing(t *testing.T) {
	x := NewXPlusObfuscator(make([]byte, 48))
	payload := make([]byte, 1200)
	wire := make([]byte, len(payload)+saltLen)
	plain := make([]byte, len(payload))

	allocations := testing.AllocsPerRun(1000, func() {
		n := x.Obfuscate(payload, wire)
		if x.Deobfuscate(wire[:n], plain) != len(payload) {
			t.Fatal("unexpected round-trip length")
		}
	})
	require.LessOrEqual(t, allocations, 0.5)
}

func BenchmarkXPlusObfuscate(b *testing.B) {
	x := NewXPlusObfuscator(make([]byte, 48))
	payload := make([]byte, 1200)
	wire := make([]byte, len(payload)+saltLen)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		x.Obfuscate(payload, wire)
	}
}
