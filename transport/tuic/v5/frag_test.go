package v5

import (
	"bufio"
	"bytes"
	"net/netip"
	"sync"
	"testing"

	"github.com/metacubex/mihomo/common/pool"
	"github.com/stretchr/testify/require"
)

func fragmentPacket(id, total uint8, size uint16, data []byte) Packet {
	packet := Packet{
		CommandHead: NewCommandHead(PacketType),
		ASSOC_ID:    1,
		PKT_ID:      2,
		FRAG_ID:     id,
		FRAG_TOTAL:  total,
		SIZE:        size,
		ADDR:        NewAddressAddrPort(netip.MustParseAddrPort("203.0.113.7:443")),
		DATA:        data,
	}
	if id != 0 {
		packet.ADDR = Address{typ: AtypNone}
	}
	return packet
}

func TestFeedReturnsSingleFragmentDirectly(t *testing.T) {
	data := pool.Get(7)
	copy(data, "payload")
	packet := fragmentPacket(0, 1, 7, data[:7])
	defer pool.Put(packet.DATA)

	var d deFragger
	assembled, ok := d.Feed(&packet)
	require.True(t, ok)
	require.Equal(t, packet.ADDR, assembled.ADDR)
	require.Equal(t, []byte("payload"), assembled.DATA)
}

func TestFeedAssemblesPooledFragmentPayloads(t *testing.T) {
	var d deFragger
	payload := []byte("0123456789")
	offsets := []int{0, 4, 7}
	for i, offset := range offsets {
		end := len(payload)
		if i+1 < len(offsets) {
			end = offsets[i+1]
		}
		data := pool.Get(end - offset)
		copy(data, payload[offset:end])
		packet := fragmentPacket(uint8(i), 3, uint16(end-offset), data[:end-offset])
		assembled, ok := d.Feed(&packet)
		if i != 2 {
			require.False(t, ok)
			continue
		}
		require.True(t, ok)
		require.Equal(t, payload, assembled.DATA)
		require.Equal(t, uint16(len(payload)), assembled.SIZE)
		require.Zero(t, assembled.FRAG_ID)
		require.Equal(t, uint8(1), assembled.FRAG_TOTAL)
		pool.Put(assembled.DATA)
	}
}

func TestFeedRejectsInvalidAndDuplicateFragments(t *testing.T) {
	var d deFragger
	invalidData := pool.Get(2)
	invalid := fragmentPacket(2, 2, 2, invalidData[:2])
	assembled, ok := d.Feed(&invalid)
	require.False(t, ok)
	require.Nil(t, assembled.DATA)
	require.Nil(t, invalid.DATA)

	storedData := pool.Get(2)
	stored := fragmentPacket(0, 2, 2, storedData[:2])
	_, ok = d.Feed(&stored)
	require.False(t, ok)

	resetData := pool.Get(2)
	reset := fragmentPacket(0, 3, 2, resetData[:2])
	_, ok = d.Feed(&reset)
	require.False(t, ok)

	duplicateData := pool.Get(2)
	duplicate := fragmentPacket(0, 3, 2, duplicateData[:2])
	assembled, ok = d.Feed(&duplicate)
	require.False(t, ok)
	require.Nil(t, assembled.DATA)
	require.Nil(t, duplicate.DATA)
}
func TestFeedReplacesPartialMessageWhenTotalChanges(t *testing.T) {
	var d deFragger
	data := pool.Get(2)
	copy(data, "ab")
	first := fragmentPacket(0, 2, 2, data[:2])
	_, ok := d.Feed(&first)
	require.False(t, ok)

	data = pool.Get(3)
	copy(data, "cde")
	second := fragmentPacket(0, 1, 3, data[:3])
	assembled, ok := d.Feed(&second)
	require.True(t, ok)
	require.Equal(t, []byte("cde"), assembled.DATA)
	pool.Put(assembled.DATA)
}

func BenchmarkFeedFragmentedReceive(b *testing.B) {
	payload := make([]byte, 4096)
	fragSize := 1024
	raw := make([][]byte, 0, len(payload)/fragSize)
	for off := 0; off < len(payload); off += fragSize {
		packet := fragmentPacket(uint8(len(raw)), uint8(len(payload)/fragSize), uint16(fragSize), payload[off:off+fragSize])
		var wire bytes.Buffer
		if err := packet.WriteTo(&wire); err != nil {
			b.Fatal(err)
		}
		raw = append(raw, wire.Bytes())
	}
	rawReader := bytes.NewReader(nil)
	buffered := bufio.NewReaderSize(rawReader, 4096)
	assembled := Packet{}
	d := deFragger{}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, frame := range raw {
			rawReader.Reset(frame)
			buffered.Reset(rawReader)
			packet, err := readPacketInto(buffered)
			if err != nil {
				b.Fatal(err)
			}
			completed, ok := d.Feed(&packet)
			if ok {
				assembled = completed
			}
		}
		if len(assembled.DATA) != len(payload) {
			b.Fatal("short assembled payload")
		}
		pool.Put(assembled.DATA)
		assembled.DATA = nil
	}
}
func TestFeedCompletedBagStaysSafeForConcurrentDuplicate(t *testing.T) {
	var d deFragger
	for cycle := 0; cycle < 100; cycle++ {
		packetID := uint16(cycle)
		for i := 0; i < 3; i++ {
			data := pool.Get(2)
			packet := fragmentPacket(uint8(i), 4, 2, data[:2])
			packet.PKT_ID = packetID
			_, ok := d.Feed(&packet)
			require.False(t, ok)
		}

		finalData := pool.Get(2)
		final := fragmentPacket(3, 4, 2, finalData[:2])
		final.PKT_ID = packetID
		duplicateData := pool.Get(2)
		duplicate := fragmentPacket(0, 4, 2, duplicateData[:2])
		duplicate.PKT_ID = packetID

		var wg sync.WaitGroup
		var finalPacket Packet
		var finalOk bool
		wg.Add(2)
		go func() {
			defer wg.Done()
			finalPacket, finalOk = d.Feed(&final)
		}()
		go func() {
			defer wg.Done()
			_, ok := d.Feed(&duplicate)
			if ok {
				pool.Put(duplicate.DATA)
				duplicate.DATA = nil
			}
		}()
		wg.Wait()
		require.True(t, finalOk)
		require.Len(t, finalPacket.DATA, 8)
		pool.Put(finalPacket.DATA)
		finalPacket.DATA = nil
	}
}
