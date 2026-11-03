package adapter

import (
	"bytes"
	"context"
	http "github.com/metacubex/http"
	"net"
	"testing"
)

func BenchmarkNewURLTestExchange(b *testing.B) {
	const response = "HTTP/1.1 204 No Content\r\nContent-Length: 0\r\n\r\n"
	request, err := http.NewRequestWithContext(context.Background(), http.MethodHead, "http://benchmark.invalid/", nil)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		client, server := net.Pipe()
		serverDone := make(chan struct{})
		go func() {
			defer close(serverDone)
			defer server.Close()

			requested := make([]byte, 0, 256)
			chunk := make([]byte, 256)
			for !bytes.Contains(requested, []byte("\r\n\r\n")) {
				n, readErr := server.Read(chunk)
				requested = append(requested, chunk[:n]...)
				if readErr != nil {
					return
				}
			}
			if _, err := server.Write([]byte(response)); err != nil {
				return
			}
		}()

		exchange, err := newURLTestExchange(context.Background(), client, "", false, nil)
		if err != nil {
			client.Close()
			<-serverDone
			b.Fatal(err)
		}
		response, err := exchange.Do(request)
		if err != nil {
			client.Close()
			<-serverDone
			b.Fatal(err)
		}
		if response.StatusCode != http.StatusNoContent {
			b.Fatalf("status = %d", response.StatusCode)
		}
		_ = response.Body.Close()
		exchange.release()
		client.Close()
		<-serverDone
	}
}
