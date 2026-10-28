package mkcp

import (
	"crypto/cipher"
	"crypto/rand"
	"io"
	"sync"

	"github.com/metacubex/mihomo/common/pool"
)

type packetReader struct {
	security   cipher.AEAD
	header     packetHeader
	dataPool   *sync.Pool
	bufferPool *sync.Pool
}

type receiveBuffer struct {
	data  []byte
	refs  int
	owner *sync.Pool
}

func receiveSize(cfg Config, security cipher.AEAD) int {
	// V2Ray counts the AEAD nonce outside its configured MTU.
	return int(cfg.mtu()) + security.NonceSize()
}

func (r packetReader) newReceiveBuffer(size int) *receiveBuffer {
	buffer := r.bufferPool.Get().(*receiveBuffer)
	buffer.data = pool.Get(size)
	buffer.refs = 0
	buffer.owner = r.bufferPool
	return buffer
}

func (b *receiveBuffer) release() {
	if b.refs > 0 {
		b.refs--
		if b.refs > 0 {
			return
		}
	}
	data := b.data
	owner := b.owner
	b.data = nil
	b.refs = 0
	b.owner = nil
	_ = pool.Put(data)
	owner.Put(b)
}

// read parses every segment into the caller's slice. Segment payloads alias b,
// so b must stay owned and unmodified until every returned segment is released.
func (r packetReader) read(b []byte, buffer *receiveBuffer, segments []segment) []segment {
	segments = segments[:0]
	if r.header != nil {
		headerSize := r.header.size()
		if len(b) <= headerSize {
			buffer.release()
			return segments
		}
		b = b[headerSize:]
	}
	if r.security != nil {
		nonceSize := r.security.NonceSize()
		overhead := r.security.Overhead()
		if len(b) <= nonceSize+overhead {
			buffer.release()
			return segments
		}
		out, err := r.security.Open(b[nonceSize:nonceSize], b[:nonceSize], b[nonceSize:], nil)
		if err != nil {
			buffer.release()
			return segments
		}
		b = out
	}
	for len(b) > 0 {
		seg, extra := readSegment(b, r.dataPool, buffer)
		if seg == nil {
			break
		}
		segments = append(segments, seg)
		b = extra
	}
	if buffer.refs == 0 {
		buffer.release()
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
	overhead := w.security.NonceSize() + w.security.Overhead()
	if w.header != nil {
		overhead += w.header.size()
	}
	return overhead
}

func (w packetWriter) writeSegment(seg segment) error {
	headerSize := 0
	if w.header != nil {
		headerSize = w.header.size()
	}
	segmentSize := seg.byteSize()

	frame := pool.Get(headerSize + segmentSize + w.securityOverhead() + 3)
	out := frame[:headerSize]
	if headerSize > 0 {
		w.header.serialize(out[:headerSize])
	}

	if w.security != nil {
		payload := pool.Get(segmentSize)
		seg.serialize(payload)
		nonceSize := w.security.NonceSize()
		out = out[:headerSize+nonceSize]
		if nonceSize > 0 {
			_, _ = rand.Read(out[headerSize : headerSize+nonceSize])
		}
		sealed := w.security.Seal(out[headerSize+nonceSize:headerSize+nonceSize], out[headerSize:headerSize+nonceSize], payload, nil)
		out = append(out[:headerSize+nonceSize], sealed...)
		_ = pool.Put(payload)
		_, err := w.writer.Write(out)
		_ = pool.Put(frame)
		return err
	}

	seg.serialize(frame[headerSize : headerSize+segmentSize])
	out = frame[:headerSize+segmentSize]
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
