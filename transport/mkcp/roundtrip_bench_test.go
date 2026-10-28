package mkcp

import (
	"bytes"
	"context"
	"io"
	"net"
	"testing"
	"time"
)

func BenchmarkRoundTrip(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pc, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		b.Fatal(err)
	}
	ln, err := ListenUDP(ctx, pc, Config{})
	if err != nil {
		b.Fatal(err)
	}
	defer ln.Close()

	serverDone := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			serverDone <- err
			return
		}
		defer conn.Close()
		_, err = io.Copy(conn, conn)
		serverDone <- err
	}()

	raw, err := net.Dial("udp", ln.Addr().String())
	if err != nil {
		b.Fatal(err)
	}
	conn, err := Dial(ctx, raw, Config{})
	if err != nil {
		b.Fatal(err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(30 * time.Second)); err != nil {
		b.Fatal(err)
	}

	payload := bytes.Repeat([]byte("b"), 16*1024)
	received := make([]byte, len(payload))
	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := conn.Write(payload); err != nil {
			b.Fatal(err)
		}
		if _, err := io.ReadFull(conn, received); err != nil {
			b.Fatal(err)
		}
	}
}
