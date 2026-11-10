package tunnel

import (
	"context"
	"net"
	"net/netip"
	"testing"
	"time"

	N "github.com/metacubex/mihomo/common/net"
	"github.com/metacubex/mihomo/component/resolver"
	"github.com/metacubex/mihomo/component/sniffer"
	C "github.com/metacubex/mihomo/constant"
	snifferType "github.com/metacubex/mihomo/constant/sniffer"
	"github.com/stretchr/testify/require"
)

// The proxy a TCP connection is dialed through: it reaches the destination the
// metadata names, which is what the direct adapter does.
type directDialProxy struct {
	directTestProxy
}

func (directDialProxy) DialContext(ctx context.Context, metadata *C.Metadata) (C.Conn, error) {
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", metadata.RemoteAddress())
	if err != nil {
		return nil, err
	}
	return &directDialConn{ExtendedConn: N.NewExtendedConn(conn)}, nil
}

// The connection side of C.Conn: chains are empty because nothing wraps this
// dial.
type directDialConn struct {
	N.ExtendedConn
}

func (c *directDialConn) Chains() C.Chain               { return nil }
func (c *directDialConn) ProviderChains() C.Chain       { return nil }
func (c *directDialConn) AppendToChains(C.ProxyAdapter) {}
func (c *directDialConn) RemoteDestination() string     { return "" }

// A TCP connection a proxy accepts: the caller's bytes reach the destination
// and its answer comes back over the same connection. Together these are the
// TCP data path, of which the tunnel's own tests held only allocation and
// retry shape.
func TestHandleTCPConnRelaysToTheDestination(t *testing.T) {
	restoreStatus := status.Load()
	status.Store(Running)
	defer status.Store(restoreStatus)

	restoreMapper := resolver.DefaultHostMapper
	resolver.DefaultHostMapper = directHostMapper{}
	defer func() { resolver.DefaultHostMapper = restoreMapper }()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := listener.Addr().(*net.TCPAddr)
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			server, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer server.Close()
				buf := make([]byte, 2048)
				for {
					n, err := server.Read(buf)
					if err != nil {
						return
					}
					if _, err := server.Write(append([]byte("echo:"), buf[:n]...)); err != nil {
						return
					}
				}
			}()
		}
	}()

	UpdateProxies(map[string]C.Proxy{"DIRECT": directDialProxy{}}, nil)
	SetMode(Direct)
	// Sniffing is what asks for the buffer, which is the branch this test is
	// about: the connection already is one, and must not be wrapped twice.
	dispatcher, err := sniffer.NewDispatcher(&sniffer.Config{
		Enable:      true,
		ParsePureIp: true,
		Sniffers: map[snifferType.Type]sniffer.SnifferConfig{
			snifferType.TLS: {OverrideDest: true},
		},
	})
	require.NoError(t, err)
	UpdateSniffer(dispatcher)
	SetSniffing(true)
	t.Cleanup(func() {
		UpdateSniffer(nil)
		SetSniffing(false)
		SetMode(Direct)
		UpdateProxies(nil, nil)
		invalidateAllProxies()
	})

	client, serverSide := net.Pipe()
	t.Cleanup(func() {
		_ = client.Close()
		_ = serverSide.Close()
	})
	answered := make(chan string, 1)
	go func() {
		buf := make([]byte, 2048)
		n, err := client.Read(buf)
		if err != nil {
			return
		}
		answered <- string(buf[:n])
	}()

	metadata := &C.Metadata{
		NetWork: C.TCP,
		Type:    C.INNER,
		SrcIP:   netip.AddrFrom4([4]byte{192, 168, 1, 100}),
		SrcPort: 53534,
		DstIP:   netip.MustParseAddr("127.0.0.1"),
		DstPort: uint16(address.Port),
	}

	// The relay runs until the caller's side closes, so it owns its goroutine.
	go handleTCPConn(serverSide, metadata)

	_, err = client.Write([]byte("hello"))
	require.NoError(t, err)

	select {
	case reply := <-answered:
		require.Equal(t, "echo:hello", reply)
	case <-time.After(5 * time.Second):
		t.Fatal("the destination never answered over the relayed connection")
	}
}

// Some listeners hand the tunnel a connection that has already been buffered;
// the relay then reads its own buffer rather than wrapping the connection a
// second time.
func TestHandleTCPConnRelaysAnAlreadyBufferedConnection(t *testing.T) {
	restoreStatus := status.Load()
	status.Store(Running)
	defer status.Store(restoreStatus)

	restoreMapper := resolver.DefaultHostMapper
	resolver.DefaultHostMapper = directHostMapper{}
	defer func() { resolver.DefaultHostMapper = restoreMapper }()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := listener.Addr().(*net.TCPAddr)
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			server, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer server.Close()
				buf := make([]byte, 2048)
				for {
					n, err := server.Read(buf)
					if err != nil {
						return
					}
					if _, err := server.Write(append([]byte("echo:"), buf[:n]...)); err != nil {
						return
					}
				}
			}()
		}
	}()

	UpdateProxies(map[string]C.Proxy{"DIRECT": directDialProxy{}}, nil)
	SetMode(Direct)
	// Sniffing is what asks for the buffer, which is the branch this test is
	// about: the connection already is one, and must not be wrapped twice.
	dispatcher, err := sniffer.NewDispatcher(&sniffer.Config{
		Enable:      true,
		ParsePureIp: true,
		Sniffers: map[snifferType.Type]sniffer.SnifferConfig{
			snifferType.TLS: {OverrideDest: true},
		},
	})
	require.NoError(t, err)
	UpdateSniffer(dispatcher)
	SetSniffing(true)
	t.Cleanup(func() {
		UpdateSniffer(nil)
		SetSniffing(false)
		SetMode(Direct)
		UpdateProxies(nil, nil)
		invalidateAllProxies()
	})

	client, serverSide := net.Pipe()
	t.Cleanup(func() {
		_ = client.Close()
		_ = serverSide.Close()
	})
	answered := make(chan string, 1)
	go func() {
		buf := make([]byte, 2048)
		n, err := client.Read(buf)
		if err != nil {
			return
		}
		answered <- string(buf[:n])
	}()

	returned := make(chan struct{})
	go func() {
		handleTCPConn(N.NewBufferedConn(serverSide), &C.Metadata{
			Type:    C.INNER,
			SrcIP:   netip.AddrFrom4([4]byte{192, 168, 1, 100}),
			SrcPort: 53544,
			DstIP:   netip.MustParseAddr("127.0.0.1"),
			DstPort: uint16(address.Port),
		})
		close(returned)
	}()

	_, err = client.Write([]byte("hello"))
	require.NoError(t, err)

	select {
	case reply := <-answered:
		require.Equal(t, "echo:hello", reply)
	case <-time.After(5 * time.Second):
		t.Fatal("the destination never answered over the buffered connection")
	}
	require.NoError(t, client.Close())
	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("the relay never returned")
	}
}

// A connection pinned to a proxy the table does not hold is dropped: there is
// no adapter to carry it and no rule to fall through to.
func TestHandleTCPConnDropsAConnectionPinnedToAMissingProxy(t *testing.T) {
	restoreStatus := status.Load()
	status.Store(Running)
	defer status.Store(restoreStatus)

	UpdateProxies(nil, nil)
	SetMode(Direct)
	t.Cleanup(func() {
		SetMode(Direct)
		UpdateProxies(nil, nil)
	})

	client, serverSide := net.Pipe()
	t.Cleanup(func() {
		_ = client.Close()
		_ = serverSide.Close()
	})

	handleTCPConn(serverSide, &C.Metadata{
		NetWork:      C.TCP,
		Type:         C.INNER,
		SpecialProxy: "gone",
		SrcIP:        netip.AddrFrom4([4]byte{192, 168, 1, 100}),
		SrcPort:      53545,
		DstIP:        netip.MustParseAddr("127.0.0.1"),
		DstPort:      1,
	})

	_, err := client.Write([]byte("hello"))
	require.Error(t, err, "the connection was dropped without a dial")
}
