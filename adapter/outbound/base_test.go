package outbound

import (
	"context"
	"net"
	"net/netip"
	"testing"

	N "github.com/metacubex/mihomo/common/net"
	C "github.com/metacubex/mihomo/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testBase(t *testing.T, option BaseOption) *Base {
	t.Helper()
	base := NewBase(option)
	require.NotNil(t, base)
	return base
}

// The adapter every protocol embeds answers what the proxy table publishes:
// its type, its address, whether it carries UDP, and the document the host
// reads. None of it had a test of its own.
func TestBaseAnswersWhatTheProxyTablePublishes(t *testing.T) {
	base := testBase(t, BaseOption{
		Name:         "node-a",
		Addr:         "node-a.example.com:443",
		Type:         C.Shadowsocks,
		ProviderName: "provider-a",
		UDP:          true,
		XUDP:         true,
		TFO:          true,
		MPTCP:        true,
		Interface:    "eth0",
		RoutingMark:  7,
		Prefer:       C.IPv4Prefer,
	})

	assert.Equal(t, "node-a", base.Name())
	assert.Equal(t, "node-a.example.com:443", base.Addr())
	assert.Equal(t, C.Shadowsocks, base.Type())
	assert.True(t, base.SupportUDP())
	assert.False(t, base.SupportUOT())
	assert.False(t, base.IsL3Protocol(nil))
	assert.Nil(t, base.Unwrap(nil, false))
	assert.Len(t, base.DialOptions(), cap(base.DialOptions()),
		"a caller that appends a private option must reallocate, not share")
	require.NoError(t, base.Close())
	require.NoError(t, base.Close(), "a repeated close is still not an error")

	info := base.ProxyInfo()
	assert.True(t, info.XUDP)
	assert.True(t, info.TFO)
	assert.True(t, info.MPTCP)
	assert.False(t, info.SMUX)
	assert.Equal(t, "eth0", info.Interface)
	assert.Equal(t, 7, info.RoutingMark)
	assert.Equal(t, "provider-a", info.ProviderName)

	document, err := base.MarshalJSON()
	require.NoError(t, err)
	expected := `{"type":"Shadowsocks","id":"` + base.id.String() + `","name":"node-a"}`
	assert.JSONEq(t, expected, string(document))
	assert.Equal(t, expected, string(base.AppendJSON(nil)))
}

// A base carries nothing by itself: a dial through it is answered with the
// same "not supported" every protocol that overrides it would not answer.
func TestBaseRefusesTheDialItHasNoHandlerFor(t *testing.T) {
	base := testBase(t, BaseOption{Name: "bare", Type: C.Direct})

	_, err := base.DialContext(context.Background(), &C.Metadata{
		NetWork: C.TCP,
		DstIP:   netip.MustParseAddr("127.0.0.1"),
		DstPort: 443,
	})
	assert.ErrorIs(t, err, C.ErrNotSupport)

	_, err = base.ListenPacketContext(context.Background(), &C.Metadata{
		NetWork: C.UDP,
		DstIP:   netip.MustParseAddr("127.0.0.1"),
		DstPort: 53,
	})
	assert.ErrorIs(t, err, C.ErrNotSupport)

	// An already resolved destination keeps its address; the resolver is only
	// asked for one metadata has not resolved itself.
	metadata := &C.Metadata{
		NetWork: C.UDP,
		DstIP:   netip.MustParseAddr("127.0.0.1"),
		DstPort: 53,
	}
	assert.NoError(t, base.ResolveUDP(context.Background(), metadata))
	assert.Equal(t, netip.MustParseAddr("127.0.0.1"), metadata.DstIP)
}

// The dial options are what the interface, the routing mark and the address
// family preference come to: each is built once, and nothing is built when
// nothing is asked for.
func TestBaseBuildsADialOptionPerRequestedSetting(t *testing.T) {
	assert.Empty(t, NewBase(BaseOption{Name: "plain"}).DialOptions())

	assert.Len(t, NewBase(BaseOption{Name: "marked", Interface: "eth0"}).DialOptions(), 1)
	assert.Len(t, NewBase(BaseOption{Name: "marked", RoutingMark: 7}).DialOptions(), 1)
	assert.Len(t, NewBase(BaseOption{Name: "marked", TFO: true}).DialOptions(), 1)
	assert.Len(t, NewBase(BaseOption{Name: "marked", MPTCP: true}).DialOptions(), 1)
	for _, prefer := range []C.DNSPrefer{
		C.IPv4Only,
		C.IPv6Only,
		C.IPv4Prefer,
		C.IPv6Prefer,
	} {
		assert.Len(t, NewBase(BaseOption{Name: "marked", Prefer: prefer}).DialOptions(), 1)
	}
	assert.Len(
		t,
		NewBase(BaseOption{
			Name:        "everything",
			Interface:   "eth0",
			RoutingMark: 7,
			TFO:         true,
			MPTCP:       true,
			Prefer:      C.IPv4Prefer,
		}).DialOptions(),
		5,
	)
}

// The API dialer a proxy can carry has priority over every option the
// configuration built, which is what lets an answer carry a caller's own.
func TestBasicOptionNewDialerKeepsTheOneTheAPICarries(t *testing.T) {
	dialer := &apiTestDialer{}

	option := &BasicOption{DialerForAPI: dialer}
	assert.Same(t, dialer, option.NewDialer(nil))

	assert.NotNil(t, (&BasicOption{}).NewDialer(nil), "an option without one builds a dialer")
	assert.Nil(t, (&BasicOption{}).NewTunnel(), "an option without one carries no tunnel")
}

type apiTestDialer struct {
	C.Dialer
}

// The connection side of a dial carries the chains the proxy table reads: the
// adapter it went through and the provider it came from.
func TestConnReportsTheDestinationItsRemoteHolds(t *testing.T) {
	local, remote := net.Pipe()
	t.Cleanup(func() {
		_ = local.Close()
		_ = remote.Close()
	})
	connection := &conn{ExtendedConn: N.NewExtendedConn(remote)}

	connection.AppendToChains(&apiTestProxy{name: "proxy-a", provider: "provider-a"})
	connection.AppendToChains(&apiTestProxy{name: "proxy-b", provider: "provider-b"})
	assert.Equal(t, C.Chain{"proxy-a", "proxy-b"}, connection.Chains())
	assert.Equal(t, C.Chain{"provider-a", "provider-b"}, connection.ProviderChains())
	assert.NotNil(t, connection.Upstream())

	// A pipe end holds no address, so the adapter's own address stands in for
	// the remote one.
	fallback := &conn{ExtendedConn: N.NewExtendedConn(remote), adapterAddr: "node-a.example.com:443"}
	assert.Equal(t, "node-a.example.com", fallback.RemoteDestination())

	// A connection over a real listener reports the remote address it holds.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	accepted := make(chan net.Conn, 1)
	go func() {
		server, err := listener.Accept()
		if err != nil {
			return
		}
		accepted <- server
	}()
	client, err := net.Dial("tcp", listener.Addr().String())
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	server := <-accepted
	t.Cleanup(func() { _ = server.Close() })
	live := &conn{ExtendedConn: N.NewExtendedConn(server), adapterAddr: "node-a.example.com:443"}
	assert.Equal(t, "127.0.0.1", live.RemoteDestination(),
		"a connection with a remote address reports it, not the adapter's")
}

type apiTestProxy struct {
	Base
	name     string
	provider string
}

func (p *apiTestProxy) Name() string { return p.name }
func (p *apiTestProxy) ProxyInfo() (info C.ProxyInfo) {
	info.ProviderName = p.provider
	return
}
