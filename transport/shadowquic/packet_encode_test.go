//go:build !race

package shadowquic

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEncodeDatagramIntoWireFormat(t *testing.T) {
	scratch := make([]byte, 2+3)
	frame := encodeDatagramInto(scratch, 0x1234, []byte{'a', 'b', 'c'})
	require.Equal(t, []byte{0x12, 0x34, 'a', 'b', 'c'}, frame)
}

func TestEncodeDatagramIntoAllocatesNothing(t *testing.T) {
	scratch := make([]byte, 2+1350)
	payload := make([]byte, 1350)

	allocations := testing.AllocsPerRun(1000, func() {
		_ = encodeDatagramInto(scratch, 0x1234, payload)
	})
	require.LessOrEqual(t, allocations, 0.5)
}

func BenchmarkEncodeDatagramInto(b *testing.B) {
	scratch := make([]byte, 2+1350)
	payload := make([]byte, 1350)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = encodeDatagramInto(scratch, 0x1234, payload)
	}
}
