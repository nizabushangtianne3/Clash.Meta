package deadline

import (
	"bytes"
	"net"
	"testing"
	"time"
)

func TestConnReadsPipedDataAcrossSmallReads(t *testing.T) {
	client, server := net.Pipe()
	t.Cleanup(func() { _ = client.Close(); _ = server.Close() })

	conn := NewConn(client)
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte("deadline-buffer "), 4096)
	go func() { _, _ = server.Write(payload) }()

	read := make([]byte, len(payload))
	for total := 0; total < len(read); {
		n, err := conn.Read(read[total:])
		if err != nil {
			t.Fatalf("read after %d bytes: %v", total, err)
		}
		total += n
	}
	if !bytes.Equal(read, payload) {
		t.Fatal("read payload mismatch")
	}
}

func TestConnCloseReleasesPendingPipeRead(t *testing.T) {
	client, server := net.Pipe()
	t.Cleanup(func() { _ = server.Close() })

	conn := NewConn(client)
	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	payload := make([]byte, 65536)
	go func() { _, _ = server.Write(payload) }()

	first := make([]byte, 1)
	if _, err := conn.Read(first); err != nil {
		t.Fatal(err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Read(first); err == nil {
		t.Fatal("closed connection accepted a read")
	}
}
