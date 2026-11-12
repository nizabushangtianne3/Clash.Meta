package encryption

import (
	"bytes"
	"crypto/rand"
	"io"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

type recordConn struct {
	net.Conn
	written *bytes.Buffer
	reads   []byte
}

func (c *recordConn) Write(b []byte) (int, error) {
	return c.written.Write(b)
}

func (c *recordConn) Read(b []byte) (int, error) {
	if len(c.reads) == 0 {
		return 0, io.EOF
	}
	n := copy(b, c.reads)
	c.reads = c.reads[n:]
	return n, nil
}

func newRecordConn(reply []byte) *recordConn {
	return &recordConn{written: &bytes.Buffer{}, reads: reply}
}

func randomBytes(t *testing.T, n int) []byte {
	t.Helper()
	b := make([]byte, n)
	_, err := rand.Read(b)
	require.NoError(t, err)
	return b
}

// XorConn masks only the TLS-looking headers it walks past. Payload bytes are
// handed to the AEAD above it, so they must travel unmasked on both sides; a
// regression that started masking payload bytes would break every "random"
// mode session, and these cases are what would catch it.
type xorMaterial struct {
	outKey, outIV, inKey, inIV []byte
}

func newXorMaterial(t *testing.T) xorMaterial {
	t.Helper()
	return xorMaterial{
		outKey: randomBytes(t, 32),
		outIV:  randomBytes(t, 16),
		inKey:  randomBytes(t, 32),
		inIV:   randomBytes(t, 16),
	}
}

func (m xorMaterial) undo(t *testing.T, wire []byte, n int) []byte {
	t.Helper()
	out := make([]byte, n)
	copy(out, wire[:n])
	NewCTR(m.outKey, m.outIV).XORKeyStream(out, out)
	return out
}

func (m xorMaterial) mask(t *testing.T, plain []byte, headerAt, headerLen int) []byte {
	t.Helper()
	out := make([]byte, len(plain))
	copy(out, plain)
	NewCTR(m.inKey, m.inIV).XORKeyStream(out[headerAt:headerAt+headerLen], out[headerAt:headerAt+headerLen])
	return out
}

func TestXorConnWriteSkipsTheBytesItWasHanded(t *testing.T) {
	m := newXorMaterial(t)
	conn := newRecordConn(nil)
	client := NewXorConn(conn, NewCTR(m.outKey, m.outIV), nil, 3, 0)

	payload := []byte("0123456789abcdef")
	n, err := client.Write(payload)
	require.NoError(t, err)
	require.Equal(t, len(payload), n)

	// The skipped bytes and everything after them go out verbatim.
	require.Equal(t, payload, conn.written.Bytes())
}

func TestXorConnWritePastAHeaderSkipsItsLength(t *testing.T) {
	m := newXorMaterial(t)
	conn := newRecordConn(nil)
	client := NewXorConn(conn, NewCTR(m.outKey, m.outIV), nil, 0, 0)

	header := make([]byte, 5)
	EncodeHeader(header, 12)
	payload := append(header, "tail-bytes!!"...)
	_, err := client.Write(payload)
	require.NoError(t, err)

	wire := conn.written.Bytes()
	require.Len(t, wire, len(payload))
	require.Equal(t, "tail-bytes!!", string(wire[5:]))
	require.Equal(t, header, m.undo(t, wire, 5))
}

func TestXorConnWriteKeepsAHeaderSplitAcrossWrites(t *testing.T) {
	m := newXorMaterial(t)
	conn := newRecordConn(nil)
	client := NewXorConn(conn, NewCTR(m.outKey, m.outIV), nil, 0, 0)

	// A header cut in half by a write boundary stays buffered, so the skip it
	// announces applies to the bytes after the second half.
	full := make([]byte, 5)
	EncodeHeader(full, 22)
	_, err := client.Write(append(append([]byte{}, full[:3]...), "ab"...))
	require.NoError(t, err)
	_, err = client.Write([]byte("cdefghij"))
	require.NoError(t, err)

	// The split header is masked in place across the boundary and the payload
	// behind it travels verbatim, which is what the peer's reader expects.
	wire := conn.written.Bytes()
	require.Len(t, wire, 13)
	require.Equal(t, "cdefghij", string(wire[5:]))
	require.Equal(t, 24922, client.OutSkip)
}

func TestXorConnWriteIgnoresAnEmptySlice(t *testing.T) {
	m := newXorMaterial(t)
	conn := newRecordConn(nil)
	client := NewXorConn(conn, NewCTR(m.outKey, m.outIV), nil, 0, 0)

	n, err := client.Write(nil)
	require.NoError(t, err)
	require.Equal(t, 0, n)
	require.Equal(t, 0, conn.written.Len())
}

func TestXorConnReadSkipsTheBytesItWasHanded(t *testing.T) {
	m := newXorMaterial(t)

	// Four bytes must stay untouched, then a header announcing the payload.
	header := make([]byte, 5)
	EncodeHeader(header, 11)
	plain := append([]byte{9, 9, 9, 9}, header...)
	plain = append(plain, "payload!!!!"...)

	serverConn := newRecordConn(m.mask(t, plain, 4, 5))
	server := NewXorConn(serverConn, nil, NewCTR(m.inKey, m.inIV), 0, 4)

	got := make([]byte, 20)
	n, err := io.ReadFull(server, got)
	require.NoError(t, err)
	require.Equal(t, len(plain), n)
	// The header comes back decrypted to its TLS-looking shape, and the payload
	// it was hiding travels through untouched.
	require.Equal(t, []byte{9, 9, 9, 9}, got[:4])
	require.Equal(t, header, got[4:9])
	require.Equal(t, "payload!!!!", string(got[9:]))
	require.Empty(t, serverConn.reads)
}

func TestXorConnReadKeepsAHeaderSplitAcrossPackets(t *testing.T) {
	m := newXorMaterial(t)

	full := make([]byte, 5)
	EncodeHeader(full, 11)
	plain := append(append([]byte{}, full[:3]...), "ab"...)
	plain = append(plain, "cdefghij"...)

	serverConn := newRecordConn(m.mask(t, plain, 0, 5))
	server := NewXorConn(serverConn, nil, NewCTR(m.inKey, m.inIV), 0, 0)

	// First read covers only the first half of the header, so the buffer keeps
	// it and the bytes after it are handed back as they arrived.
	first := make([]byte, 3)
	n, err := server.Read(first)
	require.NoError(t, err)
	require.Equal(t, 3, n)

	// The remaining wire is the header's second half plus the whole payload.
	rest := make([]byte, 10)
	n, err = io.ReadFull(server, rest)
	require.NoError(t, err)
	require.Equal(t, 10, n)
	require.Equal(t, "ab", string(rest[:2]))
	require.Equal(t, "cdefghij", string(rest[2:]))
}

func TestXorConnReadHandsBackZeroOnAnEmptyBuffer(t *testing.T) {
	m := newXorMaterial(t)
	serverConn := newRecordConn(nil)
	server := NewXorConn(serverConn, nil, NewCTR(m.inKey, m.inIV), 0, 0)

	n, err := server.Read(nil)
	require.NoError(t, err)
	require.Equal(t, 0, n)
}

func TestXorConnWriteSurfacesAPeerWriteFailure(t *testing.T) {
	m := newXorMaterial(t)
	client := NewXorConn(failingConn{}, NewCTR(m.outKey, m.outIV), nil, 0, 0)

	_, err := client.Write([]byte("anything"))
	require.ErrorIs(t, err, errPeerWrite)
}

type failingConn struct {
	net.Conn
}

func (failingConn) Write(b []byte) (int, error) { return 0, errPeerWrite }

var errPeerWrite = io.ErrClosedPipe
