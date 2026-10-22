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
