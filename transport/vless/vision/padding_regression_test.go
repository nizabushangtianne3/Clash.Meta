package vision

import (
	"bytes"
	"testing"

	"github.com/gofrs/uuid/v5"

	"github.com/metacubex/mihomo/common/buf"
)

func TestReshapeBufferCopiesPooledBuffersWithoutFrontHeadroom(t *testing.T) {
	vc := &Conn{writeFilterApplicationData: true, writeOnceUserUUID: make([]byte, 16)}
	payload := bytes.Repeat([]byte("vision-buffer-overflow-regression-"), 960)

	source := buf.NewSize(32768)
	if _, err := source.Write(payload); err != nil {
		t.Fatal(err)
	}

	chunks := vc.ReshapeBuffer(source)
	if len(chunks) == 0 {
		t.Fatal("no reshaped chunks")
	}

	var reshaped []byte
	for _, chunk := range chunks {
		if chunk.Start() < PaddingHeaderLen-uuid.Size {
			t.Fatalf("chunk has insufficient command headroom: start=%d", chunk.Start())
		}
		reshaped = append(reshaped, chunk.Bytes()...)
		ApplyPadding(chunk, commandPaddingContinue, nil, true)
		chunk.Release()
	}

	if !bytes.Equal(reshaped, payload) {
		t.Fatalf("reshaped payload mismatch: got=%d want=%d", len(reshaped), len(payload))
	}
}

func TestReshapeBufferCopiesSmallBuffersWithoutFrontHeadroom(t *testing.T) {
	vc := &Conn{writeFilterApplicationData: true, writeOnceUserUUID: make([]byte, 16)}
	payload := []byte("small pooled buffer")

	source := buf.NewSize(32768)
	if _, err := source.Write(payload); err != nil {
		t.Fatal(err)
	}

	chunks := vc.ReshapeBuffer(source)
	if len(chunks) != 1 {
		t.Fatalf("chunk count = %d, want 1", len(chunks))
	}
	if chunks[0].Start() < PaddingHeaderLen-uuid.Size {
		t.Fatalf("chunk has insufficient command headroom: start=%d", chunks[0].Start())
	}
	if !bytes.Equal(chunks[0].Bytes(), payload) {
		t.Fatal("small payload mismatch")
	}
}
