package obfs

import (
	"bytes"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

type framedConn struct {
	net.Conn
	r *bytes.Reader
}

func (c *framedConn) Read(b []byte) (int, error) { return c.r.Read(b) }

type recordingConn struct {
	net.Conn
	buf bytes.Buffer
}

func (c *recordingConn) Write(b []byte) (int, error) { return c.buf.Write(b) }

func TestTLSObfsReadFrame(t *testing.T) {
	conn := &TLSObfs{
		Conn: &framedConn{r: bytes.NewReader([]byte{0x00, 0x03, 'a', 'b', 'c'})},
	}
	buf := make([]byte, 8)

	n, err := conn.read(buf, 0)
	require.NoError(t, err)
	require.Equal(t, 3, n)
	require.Equal(t, "abc", string(buf[:n]))
}

func TestTLSObfsServerReadFrame(t *testing.T) {
	conn := &TLSObfsServer{
		Conn: &framedConn{r: bytes.NewReader([]byte{0x00, 0x03, 'a', 'b', 'c'})},
	}
	buf := make([]byte, 8)

	n, err := conn.read(buf, 0)
	require.NoError(t, err)
	require.Equal(t, 3, n)
	require.Equal(t, "abc", string(buf[:n]))
}

func TestTLSObfsWriteFrame(t *testing.T) {
	conn := &recordingConn{}
	client := &TLSObfs{Conn: conn, firstRequest: false}

	n, err := client.write([]byte{'a', 'b', 'c'})
	require.NoError(t, err)
	require.Equal(t, 3, n)
	require.Equal(t, []byte{0x17, 0x03, 0x03, 0x00, 0x03, 'a', 'b', 'c'}, conn.buf.Bytes())
}

func TestTLSObfsServerWriteFrame(t *testing.T) {
	conn := &recordingConn{}
	server := &TLSObfsServer{Conn: conn, firstResponse: false}

	n, err := server.write([]byte{'a', 'b', 'c'})
	require.NoError(t, err)
	require.Equal(t, 3, n)
	require.Equal(t, []byte{0x17, 0x03, 0x03, 0x00, 0x03, 'a', 'b', 'c'}, conn.buf.Bytes())
}
