package mkcp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSimpleAuthenticatorRoundTripStaysAllocationFree(t *testing.T) {
	auth := newSimpleAuthenticator()
	plain := []byte("mihomo mkcp segment")
	sealed := make([]byte, 0, auth.Overhead()+len(plain)+3)
	openedBuf := make([]byte, 0, auth.Overhead()+len(plain)+3)
	var opened []byte

	allocations := testing.AllocsPerRun(1000, func() {
		sealed = auth.Seal(sealed[:0], nil, plain, nil)
		opened, _ = auth.Open(openedBuf[:0], nil, sealed, nil)
	})
	require.Equal(t, plain, opened)
	require.Zero(t, allocations)
}

func BenchmarkSimpleAuthenticatorRoundTrip(b *testing.B) {
	auth := newSimpleAuthenticator()
	plain := make([]byte, 1350)
	sealed := make([]byte, 0, auth.Overhead()+len(plain)+3)
	openedBuf := make([]byte, 0, auth.Overhead()+len(plain)+3)
	b.ReportAllocs()
	b.SetBytes(int64(len(plain)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sealed = auth.Seal(sealed[:0], nil, plain, nil)
		opened, _ := auth.Open(openedBuf[:0], nil, sealed, nil)
		if len(opened) != len(plain) {
			b.Fatal("bad round trip")
		}
	}
}
