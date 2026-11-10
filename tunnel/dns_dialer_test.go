package tunnel

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"testing"

	N "github.com/metacubex/mihomo/common/net"
	"github.com/metacubex/mihomo/component/resolver"
	C "github.com/metacubex/mihomo/constant"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

// A resolver the test owns: it answers what it was told to, or fails on
// demand, so no query ever leaves the process.
type dnsTestResolver struct {
	ip  netip.Addr
	err error
}

func (r *dnsTestResolver) LookupIP(ctx context.Context, host string) ([]netip.Addr, error) {
	return []netip.Addr{r.ip}, r.err
}

func (r *dnsTestResolver) LookupIPv4(ctx context.Context, host string) ([]netip.Addr, error) {
	return []netip.Addr{r.ip}, r.err
}

func (r *dnsTestResolver) LookupIPv6(ctx context.Context, host string) ([]netip.Addr, error) {
	return nil, r.err
}

func (r *dnsTestResolver) ResolveECH(ctx context.Context, host string) ([]byte, error) {
	return nil, r.err
}

func (r *dnsTestResolver) ExchangeContext(ctx context.Context, m *dns.Msg) (*dns.Msg, error) {
	return nil, r.err
}

func (r *dnsTestResolver) Invalid() bool    { return r.err != nil }
func (r *dnsTestResolver) ClearCache()      {}
func (r *dnsTestResolver) ResetConnection() {}

// An adapter that answers a dial with a pipe and a listen with a loopback UDP
// socket, so the tunnel paths are exercised without a network.
type dnsTestAdapter struct {
	// The nil embedded interface stands for the methods this adapter never
	// answers: the dialer calls the ones below, and if it ever calls another,
	// the test fails loudly instead of passing quietly.
	C.ProxyAdapter
	remotes   []net.Conn
	sockets   []net.PacketConn
	udp       bool
	l3        bool
	dialErr   error
	listenErr error
}

func (a *dnsTestAdapter) Name() string                  { return "DIRECT" }
func (a *dnsTestAdapter) Type() C.AdapterType           { return C.Direct }
func (a *dnsTestAdapter) SupportUDP() bool              { return a.udp }
func (a *dnsTestAdapter) IsL3Protocol(*C.Metadata) bool { return a.l3 }

func (a *dnsTestAdapter) DialContext(ctx context.Context, metadata *C.Metadata) (C.Conn, error) {
	if a.dialErr != nil {
		return nil, a.dialErr
	}
	local, remote := net.Pipe()
	a.remotes = append(a.remotes, remote)
	return &peekRelayConn{ExtendedConn: N.NewExtendedConn(local)}, nil
}

func (a *dnsTestAdapter) ListenPacketContext(ctx context.Context, metadata *C.Metadata) (C.PacketConn, error) {
	if a.listenErr != nil {
		return nil, a.listenErr
	}
	socket, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	a.sockets = append(a.sockets, socket)
	return &chainlessPacketConn{EnhancePacketConn: N.NewEnhancePacketConn(socket)}, nil
}

func (a *dnsTestAdapter) close(t *testing.T) {
	t.Helper()
	for _, closer := range a.remotes {
		if closer != nil {
			_ = closer.Close()
		}
	}
	for _, socket := range a.sockets {
		_ = socket.Close()
	}
}

func newDNSDialerTest(t *testing.T, resolver resolver.Resolver, adapter C.ProxyAdapter, name string) *DNSDialer {
	t.Helper()
	dialer := NewDNSDialer(resolver, adapter, name)
	t.Cleanup(func() {
		if closable, ok := adapter.(*dnsTestAdapter); ok {
			closable.close(t)
		}
	})
	return dialer
}

// The dialer parses its address before it does anything else, so an address
// it cannot parse is answered for both transports.
func TestDNSDialerRejectsAnAddressItCannotParse(t *testing.T) {
	dialer := newDNSDialerTest(t, &dnsTestResolver{}, &dnsTestAdapter{}, "")

	_, err := dialer.DialContext(context.Background(), "tcp", "not an address")
	require.Error(t, err)

	_, err = dialer.ListenPacket(context.Background(), "udp", "not an address")
	require.Error(t, err)
}

// UDP has no remote to fall back on, so a host that will not resolve stops the
// dial before any packet path is chosen.
func TestDNSDialerStopsWhenTheHostWillNotResolve(t *testing.T) {
	adapter := &dnsTestAdapter{udp: true}
	dialer := newDNSDialerTest(t, &dnsTestResolver{err: errors.New("no resolver here")}, adapter, "")

	_, err := dialer.DialContext(context.Background(), "udp", "dns.example:53")
	require.Error(t, err)

	_, err = dialer.ListenPacket(context.Background(), "udp", "dns.example:53")
	require.Error(t, err)
}

// A proxy that cannot carry UDP refuses the dial with the proxy named, on both
// transports, rather than falling back to something else.
func TestDNSDialerRefusesAProxyThatCannotCarryUDP(t *testing.T) {
	adapter := &dnsTestAdapter{}
	dialer := newDNSDialerTest(t, &dnsTestResolver{ip: netip.MustParseAddr("127.0.0.1")}, adapter, "")

	_, err := dialer.DialContext(context.Background(), "udp", "127.0.0.1:53")
	require.ErrorContains(t, err, "UDP is not supported")

	_, err = dialer.ListenPacket(context.Background(), "udp", "127.0.0.1:53")
	require.ErrorContains(t, err, "UDP is not supported")
}

// A dial that fails inside the proxy is reported as it came, with the rule the
// tunnel would have logged left to its own path.
func TestDNSDialerReportsTheProxysOwnFailure(t *testing.T) {
	adapter := &dnsTestAdapter{dialErr: errors.New("proxy refused")}
	dialer := newDNSDialerTest(t, &dnsTestResolver{ip: netip.MustParseAddr("127.0.0.1")}, adapter, "")

	_, err := dialer.DialContext(context.Background(), "tcp", "127.0.0.1:443")
	require.ErrorContains(t, err, "proxy refused")

	udpAdapter := &dnsTestAdapter{udp: true, listenErr: errors.New("no packet path")}
	udpDialer := newDNSDialerTest(t, &dnsTestResolver{ip: netip.MustParseAddr("127.0.0.1")}, udpAdapter, "")
	_, err = udpDialer.ListenPacket(context.Background(), "udp", "127.0.0.1:53")
	require.ErrorContains(t, err, "no packet path")
}

// The two transports the tunnel carries: a TCP dial arrives as a counted conn,
// and a UDP listen as a counted packet conn.
func TestDNSDialerAnswersBothTransportsThroughTheAdapter(t *testing.T) {
	adapter := &dnsTestAdapter{udp: true}
	dialer := newDNSDialerTest(t, &dnsTestResolver{ip: netip.MustParseAddr("127.0.0.1")}, adapter, "")

	conn, err := dialer.DialContext(context.Background(), "tcp", "127.0.0.1:443")
	require.NoError(t, err)
	require.NotNil(t, conn)
	t.Cleanup(func() { _ = conn.Close() })

	packet, err := dialer.ListenPacket(context.Background(), "udp", "127.0.0.1:53")
	require.NoError(t, err)
	require.NotNil(t, packet)
	t.Cleanup(func() { _ = packet.Close() })
}

// A name the tunnel holds picks the proxy by name and marks the metadata, so a
// dial is not left to the interface fallback.
func TestDNSDialerPicksTheProxyTheTunnelHoldsByName(t *testing.T) {
	adapter := &dnsTestAdapter{}
	dialer := newDNSDialerTest(t, &dnsTestResolver{ip: netip.MustParseAddr("127.0.0.1")}, adapter, "DIRECT")

	UpdateProxies(map[string]C.Proxy{"DIRECT": directTestProxy{}}, nil)
	t.Cleanup(func() { UpdateProxies(nil, nil) })

	_, err := dialer.DialContext(context.Background(), "tcp", "127.0.0.1:443")
	require.NoError(t, err)

	// The adapter above refuses UDP, so the same path on a UDP dial is the
	// refusal this proxy answers with.
	udpAdapter := &dnsTestAdapter{}
	udpDialer := newDNSDialerTest(t, &dnsTestResolver{ip: netip.MustParseAddr("127.0.0.1")}, udpAdapter, "DIRECT")
	_, err = udpDialer.ListenPacket(context.Background(), "udp", "127.0.0.1:53")
	require.ErrorContains(t, err, "UDP is not supported")
}

// A name the tunnel does not hold is an interface, not a proxy: the dial is
// left to the platform, and a name that cannot become one is reported.
func TestDNSDialerLeavesAnUnknownNameToTheInterface(t *testing.T) {
	// No adapter of its own: the name is all this dialer has, and the tunnel
	// holds no proxy by that name.
	dialer := newDNSDialerTest(t, &dnsTestResolver{ip: netip.MustParseAddr("127.0.0.1")}, nil, "no-such-interface")

	UpdateProxies(nil, nil)
	t.Cleanup(func() { UpdateProxies(nil, nil) })

	// The interface path owns the dial, and a platform without that interface
	// answers its own failure - never the adapter the dialer was not given.
	// The platform's own wording for a missing interface is not ours to pin,
	// so this stops at "the platform answered, not a proxy".
	_, err := dialer.DialContext(context.Background(), "tcp", "127.0.0.1:443")
	require.Error(t, err)
	require.NotContains(t, err.Error(), "proxy",
		"a dial with no proxy of its own must not be answered by one, error = %v", err)
}
