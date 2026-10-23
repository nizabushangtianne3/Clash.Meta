package mekya

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"

	"github.com/metacubex/mihomo/common/pool"
	"github.com/stretchr/testify/require"
)

func TestReadPacketBundleStaysAllocationFree(t *testing.T) {
	payload := make([]byte, 1200)
	body := make([]byte, len(payload)+2)
	binary.BigEndian.PutUint16(body, uint16(len(payload)))
	copy(body[2:], payload)
	reader := bytes.NewReader(body)

	allocations := testing.AllocsPerRun(1000, func() {
		reader.Reset(body)
		packet, err := readPacketBundle(reader)
		if err != nil {
			t.Fatal(err)
		}
		if cap(packet)&(cap(packet)-1) == 0 {
			_ = pool.Put(packet)
		}
	})
	require.Zero(t, allocations)
}

func BenchmarkReadPacketBundle(b *testing.B) {
	payload := make([]byte, 1200)
	body := make([]byte, len(payload)+2)
	binary.BigEndian.PutUint16(body, uint16(len(payload)))
	copy(body[2:], payload)
	reader := bytes.NewReader(body)
	b.ReportAllocs()
	b.SetBytes(int64(len(body)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader.Reset(body)
		packet, err := readPacketBundle(reader)
		if err != nil {
			b.Fatal(err)
		}
		if cap(packet)&(cap(packet)-1) == 0 {
			_ = pool.Put(packet)
		}
	}
}

func TestWritePacketBundleStaysAllocationFree(t *testing.T) {
	packet := pool.Get(1200)
	defer pool.Put(packet)

	allocations := testing.AllocsPerRun(1000, func() {
		if err := writePacketBundle(io.Discard, packet); err != nil {
			t.Fatal(err)
		}
	})
	require.Zero(t, allocations)
}

func BenchmarkWritePacketBundle(b *testing.B) {
	packet := pool.Get(1200)
	defer pool.Put(packet)
	b.ReportAllocs()
	b.SetBytes(int64(len(packet) + packetBundleOverhead))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := writePacketBundle(io.Discard, packet); err != nil {
			b.Fatal(err)
		}
	}
}
