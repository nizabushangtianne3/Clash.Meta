//go:build !race

package obfs

import (
	"bytes"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

type readOnlyConn struct {
	net.Conn
	r *bytes.Reader
}

func (c *readOnlyConn) Read(b []byte) (int, error) { return c.r.Read(b) }

func TestTLSObfsReadAllocatesNothing(t *testing.T) {
	raw := []byte{0x00, 0x03, 'a', 'b', 'c'}
	reader := bytes.NewReader(raw)
	conn := &TLSObfs{Conn: &readOnlyConn{r: reader}}
	buf := make([]byte, 16)

	allocations := testing.AllocsPerRun(1000, func() {
		reader.Reset(raw)
		n, err := conn.read(buf, 0)
		if err != nil {
			t.Fatal(err)
		}
		if n != 3 {
			t.Fatalf("read %d bytes, want 3", n)
		}
	})
	require.LessOrEqual(t, allocations, 0.5)
}

func TestTLSObfsServerReadAllocatesNothing(t *testing.T) {
	raw := []byte{0x00, 0x03, 'a', 'b', 'c'}
	reader := bytes.NewReader(raw)
	conn := &TLSObfsServer{Conn: &readOnlyConn{r: reader}}
	buf := make([]byte, 16)

	allocations := testing.AllocsPerRun(1000, func() {
		reader.Reset(raw)
		n, err := conn.read(buf, 0)
		if err != nil {
			t.Fatal(err)
		}
		if n != 3 {
			t.Fatalf("read %d bytes, want 3", n)
		}
	})
	require.LessOrEqual(t, allocations, 0.5)
}

func BenchmarkTLSObfsRead(b *testing.B) {
	raw := []byte{0x00, 0x03, 'a', 'b', 'c'}
	reader := bytes.NewReader(raw)
	conn := &TLSObfs{Conn: &readOnlyConn{r: reader}}
	buf := make([]byte, 16)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader.Reset(raw)
		if _, err := conn.read(buf, 0); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTLSObfsServerRead(b *testing.B) {
	raw := []byte{0x00, 0x03, 'a', 'b', 'c'}
	reader := bytes.NewReader(raw)
	conn := &TLSObfsServer{Conn: &readOnlyConn{r: reader}}
	buf := make([]byte, 16)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader.Reset(raw)
		if _, err := conn.read(buf, 0); err != nil {
			b.Fatal(err)
		}
	}
}
