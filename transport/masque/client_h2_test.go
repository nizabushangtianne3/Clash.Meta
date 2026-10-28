package masque

import (
	"context"
	"io"
	"testing"

	"github.com/metacubex/quic-go/quicvarint"
	"github.com/stretchr/testify/require"
)

type resettableReader struct {
	data   []byte
	offset int
}

func (r *resettableReader) Read(p []byte) (int, error) {
	if r.offset == len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.offset:])
	r.offset += n
	return n, nil
}

func (r *resettableReader) Close() error {
	return nil
}

func TestH2DatagramStreamSendDatagramStaysAllocationFree(t *testing.T) {
	reader, writer := io.Pipe()
	stream := newH2DatagramStream(writer, nil, nil)
	copied := make(chan error, 1)
	go func() {
		_, err := io.Copy(io.Discard, reader)
		copied <- reader.CloseWithError(err)
	}()
	payload := make([]byte, 1200)

	var err error
	allocations := testing.AllocsPerRun(1000, func() {
		err = stream.SendDatagram(payload)
	})
	require.NoError(t, err)
	require.Zero(t, allocations)
	require.NoError(t, writer.Close())
	require.NoError(t, <-copied)
}

func TestH2DatagramStreamReceiveDatagramReusesTheReader(t *testing.T) {
	body := quicvarint.Append(nil, h2DatagramCapsuleType)
	body = append(body, 0)
	responseBody := &resettableReader{data: body}
	stream := newH2DatagramStream(nil, responseBody, nil)

	var data []byte
	var err error
	allocations := testing.AllocsPerRun(1000, func() {
		responseBody.offset = 0
		data, err = stream.ReceiveDatagram(context.Background())
	})
	require.NoError(t, err)
	require.Empty(t, data)
	require.Zero(t, allocations)
}

func BenchmarkH2DatagramStreamReceiveDatagram(b *testing.B) {
	body := quicvarint.Append(nil, h2DatagramCapsuleType)
	body = append(body, 0)
	responseBody := &resettableReader{data: body}
	stream := newH2DatagramStream(nil, responseBody, nil)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		responseBody.offset = 0
		if _, err := stream.ReceiveDatagram(ctx); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkH2DatagramStreamSendDatagram(b *testing.B) {
	reader, writer := io.Pipe()
	stream := newH2DatagramStream(writer, nil, nil)
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, reader)
		close(done)
	}()
	payload := make([]byte, 1200)
	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := stream.SendDatagram(payload); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	_ = writer.Close()
	<-done
}
