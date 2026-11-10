package tunnel

import (
	"context"
	"net"
	"net/netip"
	"testing"
	"time"

	N "github.com/metacubex/mihomo/common/net"
	C "github.com/metacubex/mihomo/constant"
	"github.com/stretchr/testify/require"
)

// A connection whose warm-up write fails: the proxy took the dial and then
// could not carry the client's first bytes.
type warmupFailConn struct {
	N.ExtendedConn
}

func (c *warmupFailConn) NeedHandshake() bool           { return true }
func (c *warmupFailConn) Chains() C.Chain               { return nil }
func (c *warmupFailConn) ProviderChains() C.Chain       { return nil }
func (c *warmupFailConn) AppendToChains(C.ProxyAdapter) {}
func (c *warmupFailConn) RemoteDestination() string     { return "" }

func (c *warmupFailConn) Write(b []byte) (int, error) {
	// A refused TCP dial is the one write failure the retry loop classifies as
	// terminal, so the connection ends after one attempt instead of ten.
	_, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", "127.0.0.1:1")
	if err != nil {
		return 0, err
	}
	return c.ExtendedConn.Write(b)
}

// The same connection with a chain that says the proxy refused on purpose.
type rejectingWarmupConn struct {
	warmupFailConn
}

func (c *rejectingWarmupConn) Chains() C.Chain { return C.Chain{"REJECT"} }

type warmupFailProxy struct {
	directTestProxy
	reject bool
}

func (p *warmupFailProxy) DialContext(
	ctx context.Context,
	metadata *C.Metadata,
) (C.Conn, error) {
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", metadata.RemoteAddress())
	if err != nil {
		return nil, err
	}
	extended := N.NewExtendedConn(conn)
	if p.reject {
		return &rejectingWarmupConn{warmupFailConn{ExtendedConn: extended}}, nil
	}
	return &warmupFailConn{ExtendedConn: extended}, nil
}

// warmupFailureTest opens the pipe a connection arrives on, registers the proxy
// that takes the dial and cannot carry it, and hands back both ends plus the
// signal that says the dial happened.
func warmupFailureTest(t *testing.T, reject bool) (
	client net.Conn,
	server net.Conn,
	dstPort uint16,
) {
	t.Helper()
	// The dial has to reach something, so that the failure under test is the
	// warm-up write rather than the dial itself.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	dstPort = uint16(listener.Addr().(*net.TCPAddr).Port)
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	restoreStatus := status.Load()
	status.Store(Running)
	t.Cleanup(func() { status.Store(restoreStatus) })

	UpdateProxies(map[string]C.Proxy{
		"DIRECT": &warmupFailProxy{reject: reject},
	}, nil)
	SetMode(Direct)
	t.Cleanup(func() {
		SetMode(Direct)
		UpdateProxies(nil, nil)
	})

	client, server = net.Pipe()
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
	})
	return client, server, dstPort
}

func warmupFailureMetadata(srcPort uint16, dstPort uint16) *C.Metadata {
	return &C.Metadata{
		NetWork: C.TCP,
		Type:    C.INNER,
		SrcIP:   netip.AddrFrom4([4]byte{192, 168, 1, 100}),
		SrcPort: srcPort,
		DstIP:   netip.MustParseAddr("127.0.0.1"),
		DstPort: dstPort,
	}
}

// The warm-up write a chain calls a reject ends the connection quietly: what
// the proxy refused is not reported as a failure.
func TestHandleTCPConnEndsARejectedWarmUpWriteQuietly(t *testing.T) {
	client, server, dstPort := warmupFailureTest(t, true)

	returned := make(chan struct{})
	go func() {
		handleTCPConn(server, warmupFailureMetadata(53542, dstPort))
		close(returned)
	}()

	_, err := client.Write([]byte("client-hello"))
	require.NoError(t, err)
	require.NoError(t, client.Close())

	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("the connection never ended after the reject")
	}
}

// A warm-up write nothing claims as a reject ends the connection as a failure
// the retry loop stops on, not one it swallows.
func TestHandleTCPConnEndsAFailedWarmUpWrite(t *testing.T) {
	client, server, dstPort := warmupFailureTest(t, false)

	returned := make(chan struct{})
	go func() {
		handleTCPConn(server, warmupFailureMetadata(53543, dstPort))
		close(returned)
	}()

	_, err := client.Write([]byte("client-hello"))
	require.NoError(t, err)
	require.NoError(t, client.Close())

	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("the connection never ended after the failed warm-up write")
	}
}
