package mkcp

import (
	"bytes"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

func referenceXorForward(x []byte) {
	for i := 4; i < len(x); i++ {
		x[i] ^= x[i-4]
	}
}

func referenceXorBackward(x []byte) {
	for i := len(x) - 1; i >= 4; i-- {
		x[i] ^= x[i-4]
	}
}

func TestXorForwardMatchesReference(t *testing.T) {
	for _, size := range []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 15, 16, 17, 31, 32, 33, 1350, 1356, 1357} {
		input := make([]byte, size)
		_, err := rand.New(rand.NewSource(int64(size))).Read(input)
		require.NoError(t, err)

		want := append([]byte(nil), input...)
		referenceXorForward(want)
		got := append([]byte(nil), input...)
		xorfwd(got)

		require.Equal(t, want, got, "size %d", size)
	}
}

func TestXorBackwardMatchesReference(t *testing.T) {
	for _, size := range []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 15, 16, 17, 31, 32, 33, 1350, 1356, 1357} {
		input := make([]byte, size)
		_, err := rand.New(rand.NewSource(int64(size))).Read(input)
		require.NoError(t, err)

		want := append([]byte(nil), input...)
		referenceXorBackward(want)
		got := append([]byte(nil), input...)
		xorbkd(got)

		require.Equal(t, want, got, "size %d", size)
	}
}

func TestXorForwardBackwardRoundTrip(t *testing.T) {
	input := bytes.Repeat([]byte("mihomo mkcp xor round trip"), 64)
	got := append([]byte(nil), input...)

	xorfwd(got)
	require.NotEqual(t, input, got)
	xorbkd(got)
	require.Equal(t, input, got)
}

func BenchmarkXorForward(b *testing.B) {
	input := make([]byte, 1356)
	_, err := rand.New(rand.NewSource(1356)).Read(input)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.SetBytes(int64(len(input)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		xorfwd(input)
	}
}

func BenchmarkXorForwardReference(b *testing.B) {
	input := make([]byte, 1356)
	_, err := rand.New(rand.NewSource(1356)).Read(input)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.SetBytes(int64(len(input)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		referenceXorForward(input)
	}
}

func BenchmarkXorBackward(b *testing.B) {
	input := make([]byte, 1356)
	_, err := rand.New(rand.NewSource(1357)).Read(input)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.SetBytes(int64(len(input)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		xorbkd(input)
	}
}

func BenchmarkXorBackwardReference(b *testing.B) {
	input := make([]byte, 1356)
	_, err := rand.New(rand.NewSource(1357)).Read(input)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.SetBytes(int64(len(input)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		referenceXorBackward(input)
	}
}
