//go:build !race

package protocol

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPutAuthDataAllocatesNothing(t *testing.T) {
	auth := &authData{connectionID: 1}
	buf := &bytes.Buffer{}

	allocations := testing.AllocsPerRun(1000, func() {
		buf.Reset()
		auth.putAuthData(buf)
	})
	require.LessOrEqual(t, allocations, 0.5)
}

func BenchmarkPutAuthData(b *testing.B) {
	auth := &authData{connectionID: 1}
	buf := &bytes.Buffer{}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		auth.putAuthData(buf)
	}
}
