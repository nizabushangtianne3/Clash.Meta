package mkcp

import (
	"crypto/cipher"
	"crypto/rand"
	"io"
	"sync"

	"github.com/metacubex/mihomo/common/pool"
)

type packetReader struct {
	security cipher.AEAD
	header   packetHeader
	dataPool *sync.Pool
}

// read parses every segment into the caller's slice. Segment payloads alias b,
// so b must stay owned and unmodified until every returned segment is released.
func (r packetReader) read(b []byte, segments []segment) []segment {
	segments = segments[:0]
	if r.header != nil {
		headerSize := r.header.size()
		if len(b) <= headerSize {
			return segments
		}
		b = b[headerSize:]
	}
	if r.security != nil {
		nonceSize := r.security.NonceSize()
		overhead := r.security.Overhead()
		if len(b) <= nonceSize+overhead {
			return segments
		}
		out, err := r.security.Open(b[nonceSize:nonceSize], b[:nonceSize], b[nonceSize:], nil)
		if err != nil {
			return segments
		}
		b = out
	}
	for len(b) > 0 {
		seg, extra := readSegment(b, r.dataPool)
		if seg == nil {
			break
		}
		segments = append(segments, seg)
		b = extra
	}
	return segments
}

type packetWriter struct {
	security cipher.AEAD
	header   packetHeader
	writer   io.Writer
}

func (w packetWriter) overhead() int {
	if w.security == nil {
		if w.header == nil {
			return 0
		}
		return w.header.size()
	}
	overhead := w.security.Overhead()
	if w.header != nil {
		overhead += w.header.size()
	}
	return overhead
}

func (w packetWriter) writeSegment(seg segment) error {
	payload := pool.Get(seg.byteSize())
	seg.serialize(payload)
	defer pool.Put(payload)

	headerSize := 0
	if w.header != nil {
		headerSize = w.header.size()
	}

	frame := pool.Get(headerSize + len(payload) + w.securityOverhead() + 3)
	out := frame[:headerSize]
	if headerSize > 0 {
		w.header.serialize(out[:headerSize])
	}

	if w.security != nil {
		nonceSize := w.security.NonceSize()
		out = out[:headerSize+nonceSize]
		if nonceSize > 0 {
			_, _ = rand.Read(out[headerSize : headerSize+nonceSize])
		}
		sealed := w.security.Seal(out[headerSize+nonceSize:headerSize+nonceSize], out[headerSize:headerSize+nonceSize], payload, nil)
		out = append(out[:headerSize+nonceSize], sealed...)
	} else {
		out = append(out, payload...)
	}

	_, err := w.writer.Write(out)
	_ = pool.Put(frame)
	return err
}

func (w packetWriter) securityOverhead() int {
	if w.security == nil {
		return 0
	}
	return w.security.Overhead()
}
