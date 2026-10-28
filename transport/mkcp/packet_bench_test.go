package mkcp

import (
	"encoding/binary"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

const benchDataPayloadSize = 1350

type discardWriter struct{}

func newTestPacketReader() packetReader {
	return packetReader{
		dataPool:   &sync.Pool{New: func() any { return new(dataSegment) }},
		bufferPool: &sync.Pool{New: func() any { return new(receiveBuffer) }},
	}
}

func (discardWriter) Write(p []byte) (int, error) {
	return len(p), nil
}

func buildDataPacket(payloadLen int) []byte {
	return buildDataPacketWithNumber(1, string(make([]byte, payloadLen)))
}

func buildDataPacketWithNumber(number uint32, payload string) []byte {
	b := make([]byte, dataSegmentOverhead+len(payload))
	binary.BigEndian.PutUint16(b, 7)
	b[2] = byte(commandData)
	binary.BigEndian.PutUint32(b[4:], 1000)
	binary.BigEndian.PutUint32(b[8:], number)
	binary.BigEndian.PutUint32(b[12:], 1)
	binary.BigEndian.PutUint16(b[16:], uint16(len(payload)))
	copy(b[18:], payload)
	return b
}

func buildAckPacket(count int) []byte {
	b := make([]byte, 17+count*4)
	binary.BigEndian.PutUint16(b, 7)
	b[2] = byte(commandACK)
	binary.BigEndian.PutUint32(b[4:], 32)
	binary.BigEndian.PutUint32(b[8:], 1)
	binary.BigEndian.PutUint32(b[12:], 1000)
	b[16] = byte(count)
	for i := 0; i < count; i++ {
		binary.BigEndian.PutUint32(b[17+i*4:], uint32(i+1))
	}
	return b
}

// A parsed data payload aliases the datagram buffer the caller already owns.
func TestReadSegmentDoesNotCopyTheDataPayload(t *testing.T) {
	reader := newTestPacketReader()
	packet := buildDataPacket(benchDataPayloadSize)
	var segments []segment

	allocations := testing.AllocsPerRun(1000, func() {
		buffer := reader.newReceiveBuffer(len(packet))
		copy(buffer.data, packet)
		segments = reader.read(buffer.data, buffer, segments[:0])
		if len(segments) != 1 {
			t.Fatal("bad parse")
		}
		for _, item := range segments {
			releaseReadSegment(item)
		}
		clear(segments)
	})
	require.Zero(t, allocations)
}

func TestReadReleasesControlOnlyReceiveBuffer(t *testing.T) {
	reader := newTestPacketReader()
	packet := buildAckPacket(3)
	buffer := reader.newReceiveBuffer(len(packet))
	copy(buffer.data, packet)

	segments := reader.read(buffer.data, buffer, nil)

	require.Len(t, segments, 1)
	require.Zero(t, buffer.refs)
	require.Nil(t, buffer.data)
	releaseReadSegment(segments[0])
}

func TestReadSharesReceiveBufferWithDataSegments(t *testing.T) {
	reader := newTestPacketReader()
	packet := append(
		buildDataPacketWithNumber(1, "one"),
		buildDataPacketWithNumber(2, "two")...,
	)
	buffer := reader.newReceiveBuffer(len(packet))
	copy(buffer.data, packet)

	segments := reader.read(buffer.data, buffer, nil)

	require.Len(t, segments, 2)
	require.EqualValues(t, 2, buffer.refs)
	releaseReadSegment(segments[0])
	require.EqualValues(t, 1, buffer.refs)
	require.NotNil(t, buffer.data)
	releaseReadSegment(segments[1])
	require.Zero(t, buffer.refs)
	require.Nil(t, buffer.data)
}

func TestWriterOverheadIncludesTheAEADNonce(t *testing.T) {
	security, err := Config{Seed: "mihomo-mkcp-test-seed"}.security()
	require.NoError(t, err)

	require.EqualValues(t, 28, packetWriter{security: security}.overhead())
}

func TestReceiveSizeCoversThePeerAEADNonce(t *testing.T) {
	security, err := Config{Seed: "mihomo-mkcp-test-seed"}.security()
	require.NoError(t, err)

	require.EqualValues(t, 1362, receiveSize(Config{}, security))
}

func BenchmarkReadDataSegment(b *testing.B) {
	reader := newTestPacketReader()
	packet := buildDataPacket(benchDataPayloadSize)
	var segments []segment
	b.ReportAllocs()
	b.SetBytes(int64(len(packet)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buffer := reader.newReceiveBuffer(len(packet))
		copy(buffer.data, packet)
		segments = reader.read(buffer.data, buffer, segments[:0])
		if len(segments) != 1 {
			b.Fatal("bad parse")
		}
		for _, item := range segments {
			releaseReadSegment(item)
		}
		clear(segments)
	}
}

func TestPacketWriterWriteSegmentStaysAllocationFree(t *testing.T) {
	header := &utpHeader{}
	run := func(t *testing.T, writer packetWriter) {
		t.Helper()
		segment := &dataSegment{payload: make([]byte, benchDataPayloadSize)}

		allocations := testing.AllocsPerRun(1000, func() {
			if err := writer.writeSegment(segment); err != nil {
				t.Fatal(err)
			}
		})
		require.Zero(t, allocations)
	}
	t.Run("plain", func(t *testing.T) { run(t, packetWriter{header: header, writer: discardWriter{}}) })
	t.Run("authenticated", func(t *testing.T) {
		run(t, packetWriter{header: header, security: newSimpleAuthenticator(), writer: discardWriter{}})
	})
}

func BenchmarkWriteDataSegment(b *testing.B) {
	segment := &dataSegment{payload: make([]byte, benchDataPayloadSize)}
	writers := map[string]packetWriter{
		"plain":         {header: &utpHeader{}, writer: discardWriter{}},
		"authenticated": {header: &utpHeader{}, security: newSimpleAuthenticator(), writer: discardWriter{}},
	}
	for name, writer := range writers {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(segment.byteSize()))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := writer.writeSegment(segment); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkReadAckSegment(b *testing.B) {
	reader := newTestPacketReader()
	packet := buildAckPacket(3)
	var segments []segment
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buffer := reader.newReceiveBuffer(len(packet))
		copy(buffer.data, packet)
		segments = reader.read(buffer.data, buffer, segments[:0])
		if len(segments) != 1 {
			b.Fatal("bad parse")
		}
		for _, item := range segments {
			releaseReadSegment(item)
		}
		clear(segments)
	}
}
