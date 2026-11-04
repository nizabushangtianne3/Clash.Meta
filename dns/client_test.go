package dns

import (
	"context"
	"net"
	"testing"
	"time"

	D "github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

func startUDPDNSServer(tb testing.TB) (string, <-chan struct{}) {
	return startShapedUDPDNSServer(tb, nil)
}

// startShapedUDPDNSServer answers every query with the positive A record the
// tests expect, then hands the response to shape for negative or sectioned
// variants (NXDOMAIN, NODATA with SOA, and so on) to mutate it.
func startShapedUDPDNSServer(tb testing.TB, shape func(*D.Msg)) (string, <-chan struct{}) {
	tb.Helper()

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(tb, err)
	tb.Cleanup(func() { _ = conn.Close() })

	received := make(chan struct{}, 1)
	go func() {
		buffer := make([]byte, 4096)
		for {
			n, addr, err := conn.ReadFrom(buffer)
			if err != nil {
				return
			}
			var query D.Msg
			if err := query.Unpack(buffer[:n]); err != nil {
				continue
			}
			select {
			case received <- struct{}{}:
			default:
			}

			var response D.Msg
			response.SetReply(&query)
			if len(query.Question) != 0 {
				question := query.Question[0]
				response.Answer = append(response.Answer, &D.A{
					Hdr: D.RR_Header{
						Name:   question.Name,
						Rrtype: D.TypeA,
						Class:  D.ClassINET,
						Ttl:    60,
					},
					A: net.IPv4(192, 0, 2, 10),
				})
			}
			if shape != nil {
				shape(&response)
			}
			packed, err := response.Pack()
			if err != nil {
				continue
			}
			_, _ = conn.WriteTo(packed, addr)
		}
	}()

	_, port, err := net.SplitHostPort(conn.LocalAddr().String())
	require.NoError(tb, err)
	return port, received
}

func startTruncatedDNSServers(tb testing.TB) string {
	tb.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(tb, err)
	tb.Cleanup(func() { _ = listener.Close() })

	address := listener.Addr().String()
	packetConn, err := net.ListenPacket("udp", address)
	require.NoError(tb, err)
	tb.Cleanup(func() { _ = packetConn.Close() })

	go func() {
		buffer := make([]byte, 4096)
		for {
			n, addr, err := packetConn.ReadFrom(buffer)
			if err != nil {
				return
			}
			var query D.Msg
			if err := query.Unpack(buffer[:n]); err != nil {
				continue
			}
			var response D.Msg
			response.SetReply(&query)
			response.Truncated = true
			packed, err := response.Pack()
			if err != nil {
				continue
			}
			_, _ = packetConn.WriteTo(packed, addr)
		}
	}()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				dnsConn := &D.Conn{Conn: conn}
				query, err := dnsConn.ReadMsg()
				if err != nil {
					return
				}
				var response D.Msg
				response.SetReply(query)
				if len(query.Question) != 0 {
					question := query.Question[0]
					response.Answer = append(response.Answer, &D.A{
						Hdr: D.RR_Header{
							Name:   question.Name,
							Rrtype: D.TypeA,
							Class:  D.ClassINET,
							Ttl:    60,
						},
						A: net.IPv4(192, 0, 2, 11),
					})
				}
				_ = dnsConn.WriteMsg(&response)
			}()
		}
	}()

	_, port, err := net.SplitHostPort(address)
	require.NoError(tb, err)
	return port
}

func newLocalUDPClient(port string) *client {
	return newClient(net.JoinHostPort("127.0.0.1", port), nil, "udp", nil, nil, "")
}

func newClientTestQuery() *D.Msg {
	query := &D.Msg{}
	query.SetQuestion("example.com.", D.TypeA)
	return query
}

func TestClientExchangeUDP(t *testing.T) {
	port, received := startUDPDNSServer(t)
	response, err := newLocalUDPClient(port).ExchangeContext(context.Background(), newClientTestQuery())
	require.NoError(t, err)
	require.Len(t, response.Answer, 1)
	select {
	case <-received:
	case <-time.After(time.Second):
		t.Fatal("DNS server did not receive the query")
	}
}

func TestClientExchangeUDPDiscardsMismatchedIDs(t *testing.T) {
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	udp := conn.(*net.UDPConn)

	_, port, err := net.SplitHostPort(conn.LocalAddr().String())
	require.NoError(t, err)
	query := newClientTestQuery()

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		buffer := make([]byte, 1500)
		n, remote, readErr := udp.ReadFromUDP(buffer)
		if readErr != nil {
			return
		}
		var request D.Msg
		require.NoError(t, request.Unpack(buffer[:n]))
		response := D.Msg{}
		response.SetReply(&request)
		response.Answer = append(response.Answer, &D.A{
			Hdr: D.RR_Header{Name: request.Question[0].Name, Rrtype: D.TypeA, Class: D.ClassINET, Ttl: 60},
			A:   net.IPv4(192, 0, 2, 11),
		})

		response.Id++
		mismatched, packErr := response.Pack()
		require.NoError(t, packErr)
		response.Id--
		matching, packErr := response.Pack()
		require.NoError(t, packErr)

		_, err := udp.WriteToUDP(mismatched, remote)
		require.NoError(t, err)
		_, err = udp.WriteToUDP(matching, remote)
		require.NoError(t, err)
	}()

	response, err := newLocalUDPClient(port).ExchangeContext(context.Background(), query)
	require.NoError(t, err)
	require.Len(t, response.Answer, 1)
	<-serverDone
}
func TestClientExchangeUDPRejectsATsigReply(t *testing.T) {
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	udp := conn.(*net.UDPConn)

	_, port, err := net.SplitHostPort(conn.LocalAddr().String())
	require.NoError(t, err)
	query := newClientTestQuery()

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		buffer := make([]byte, 1500)
		n, remote, readErr := udp.ReadFromUDP(buffer)
		if readErr != nil {
			return
		}
		var request D.Msg
		require.NoError(t, request.Unpack(buffer[:n]))
		response := D.Msg{}
		response.SetReply(&request)
		response.Extra = append(response.Extra, &D.TSIG{
			Hdr:       D.RR_Header{Name: "hmac-sha256.", Rrtype: D.TypeTSIG, Class: D.ClassANY, Ttl: 0},
			Algorithm: D.HmacSHA256,
		})
		wire, packErr := response.Pack()
		require.NoError(t, packErr)
		_, err := udp.WriteToUDP(wire, remote)
		require.NoError(t, err)
	}()

	_, err = newLocalUDPClient(port).ExchangeContext(context.Background(), query)
	require.ErrorIs(t, err, D.ErrSig)
	<-serverDone
}
func TestClientExchangeUDPDeadline(t *testing.T) {
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer conn.Close()

	_, port, err := net.SplitHostPort(conn.LocalAddr().String())
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	_, err = newLocalUDPClient(port).ExchangeContext(ctx, newClientTestQuery())
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestClientExchangeRetriesTruncatedUDPOverTCP(t *testing.T) {
	port := startTruncatedDNSServers(t)
	response, err := newLocalUDPClient(port).ExchangeContext(context.Background(), newClientTestQuery())
	require.NoError(t, err)
	require.Len(t, response.Answer, 1)
	answer, ok := response.Answer[0].(*D.A)
	require.True(t, ok)
	require.Equal(t, net.IPv4(192, 0, 2, 11).To4(), answer.A.To4())
}

func BenchmarkClientExchangeUDP(b *testing.B) {
	benchmarkClientExchangeUDPShaped(b, nil)
}

// negativeReplyShape turns the positive reply into the NXDOMAIN or NODATA form
// a dead domain gets: an error or empty answer with the SOA in the authority.
func negativeReplyShape(rcode int) func(*D.Msg) {
	return func(response *D.Msg) {
		response.Rcode = rcode
		response.Answer = nil
		response.Ns = append(response.Ns, &D.SOA{
			Hdr:     D.RR_Header{Name: response.Question[0].Name, Rrtype: D.TypeSOA, Class: D.ClassINET, Ttl: 300},
			Ns:      "ns1.example.com.",
			Mbox:    "hostmaster.example.com.",
			Serial:  1,
			Refresh: 2,
			Retry:   3,
			Expire:  4,
			Minttl:  60,
		})
	}
}

func benchmarkClientExchangeUDPShaped(b *testing.B, shape func(*D.Msg)) {
	port, _ := startShapedUDPDNSServer(b, shape)
	dnsClient := newLocalUDPClient(port)
	query := newClientTestQuery()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := dnsClient.ExchangeContext(ctx, query); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkClientExchangeUDPNXDOMAIN(b *testing.B) {
	benchmarkClientExchangeUDPShaped(b, negativeReplyShape(D.RcodeNameError))
}

func BenchmarkClientExchangeUDPNODATA(b *testing.B) {
	benchmarkClientExchangeUDPShaped(b, negativeReplyShape(D.RcodeSuccess))
}

func BenchmarkDNSClientAddress(b *testing.B) {
	client := newLocalUDPClient("53")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if client.Address() == "" {
			b.Fatal("empty address")
		}
	}
}

func BenchmarkDNSOverTLSAddress(b *testing.B) {
	client := newDoTClient("203.0.113.7:853", nil, nil, nil, "")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if client.Address() == "" {
			b.Fatal("empty address")
		}
	}
}
