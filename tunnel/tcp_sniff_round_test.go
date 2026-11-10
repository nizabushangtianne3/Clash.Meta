package tunnel

import (
	"context"
	"encoding/binary"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"

	N "github.com/metacubex/mihomo/common/net"
	"github.com/metacubex/mihomo/component/resolver"
	"github.com/metacubex/mihomo/component/sniffer"
	"github.com/metacubex/mihomo/component/trie"
	C "github.com/metacubex/mihomo/constant"
	snifferType "github.com/metacubex/mihomo/constant/sniffer"
	"github.com/stretchr/testify/require"
)

type missingFakeRecordMapper struct{}

func (missingFakeRecordMapper) FakeIPEnabled() bool  { return true }
func (missingFakeRecordMapper) MappingEnabled() bool { return true }

// Only the range a test marks fake is fake, so an address a hosts file maps
// can carry the dial.
func (missingFakeRecordMapper) IsFakeIP(addr netip.Addr) bool {
	return addr.Is4() && addr.Unmap().As4()[0] == 198 && addr.Unmap().As4()[1] == 18
}
func (missingFakeRecordMapper) IsFakeBroadcastIP(netip.Addr) bool { return false }
func (missingFakeRecordMapper) IsExistFakeIP(netip.Addr) bool     { return false }
func (missingFakeRecordMapper) FindHostByIP(netip.Addr) (string, bool) {
	return "", false
}
func (missingFakeRecordMapper) FlushFakeIP() error                { return nil }
func (missingFakeRecordMapper) InsertHostByIP(netip.Addr, string) {}
func (missingFakeRecordMapper) StoreFakePoolState()               {}

// A proxy that records the name it was dialed with and answers the destination
// the test names rather than the metadata's: the sniffed domain has no address
// of its own in a test, and the branch under test is what names it.
type recordingDialProxy struct {
	directTestProxy
	dialed chan dialRecord
	target string
	refuse bool
}

// What a dial was handed: the name it carries and the name something read
// out of the connection. A hosts mapping leaves the second where the first was.
type dialRecord struct {
	host      string
	sniffHost string
}

func (p recordingDialProxy) DialContext(
	ctx context.Context,
	metadata *C.Metadata,
) (C.Conn, error) {
	select {
	case p.dialed <- dialRecord{host: metadata.Host, sniffHost: metadata.SniffHost}:
	default:
	}
	if p.refuse {
		_, err := (&net.Dialer{}).DialContext(ctx, "tcp", "127.0.0.1:1")
		if err != nil {
			return nil, err
		}
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", p.target)
	if err != nil {
		return nil, err
	}
	return &directDialConn{ExtendedConn: N.NewExtendedConn(conn)}, nil
}

// A connection that names no destination is closed without a dial: there is
// nothing to route it by.
func TestHandleTCPConnClosesAConnectionWithNoDestination(t *testing.T) {
	restoreStatus := status.Load()
	status.Store(Running)
	defer status.Store(restoreStatus)

	dialed := make(chan dialRecord, 1)
	UpdateProxies(map[string]C.Proxy{
		"DIRECT": recordingDialProxy{dialed: dialed, target: "127.0.0.1:1"},
	}, nil)
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
		NetWork: C.TCP,
		Type:    C.INNER,
		SrcIP:   netip.AddrFrom4([4]byte{192, 168, 1, 100}),
		SrcPort: 53537,
		DstPort: 443,
	})

	select {
	case name := <-dialed:
		t.Fatalf("a connection with no destination was dialed with %q", name)
	default:
	}
	_, err := client.Write([]byte("hello"))
	require.Error(t, err, "the connection was closed without a dial")
}

// A dial the proxy refuses ends the connection: the retry budget is spent on
// errors that may still succeed, and a refused one is not one of them.
func TestHandleTCPConnEndsAConnectionTheProxyRefuses(t *testing.T) {
	restoreStatus := status.Load()
	status.Store(Running)
	defer status.Store(restoreStatus)

	restoreMapper := resolver.DefaultHostMapper
	resolver.DefaultHostMapper = missingFakeRecordMapper{}
	defer func() { resolver.DefaultHostMapper = restoreMapper }()

	dialed := make(chan dialRecord, 1)
	UpdateProxies(map[string]C.Proxy{
		"DIRECT": recordingDialProxy{dialed: dialed, target: "127.0.0.1:1", refuse: true},
	}, nil)
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
		NetWork: C.TCP,
		Type:    C.INNER,
		SrcIP:   netip.AddrFrom4([4]byte{192, 168, 1, 100}),
		SrcPort: 53538,
		DstIP:   netip.MustParseAddr("127.0.0.1"),
		DstPort: 1,
	})

	select {
	case <-dialed:
	default:
		t.Fatal("the proxy was never asked to dial")
	}
	_, err := client.Write([]byte("hello"))
	require.Error(t, err, "the connection ended with the dial it refused")
}

// A client hello with the server name extension: the same wire shape the
// sniffer's own tests build, kept here so the tunnel test reads it as bytes off
// a connection rather than as a slice.
func testClientHello(host string) []byte {
	var extensions []byte
	if host != "" {
		serverName := []byte(host)
		serverNameListLen := 1 + 2 + len(serverName)
		extensionDataLen := 2 + serverNameListLen
		extensions = make([]byte, 4+extensionDataLen)
		binary.BigEndian.PutUint16(extensions[2:4], uint16(extensionDataLen))
		binary.BigEndian.PutUint16(extensions[4:6], uint16(serverNameListLen))
		binary.BigEndian.PutUint16(extensions[7:9], uint16(len(serverName)))
		copy(extensions[9:], serverName)
	}

	body := make([]byte, 0, 64+len(host))
	body = append(body, 0x03, 0x03)          // legacy_version
	body = append(body, make([]byte, 32)...) // random
	body = append(body, 0)                   // legacy_session_id
	body = append(body, 0, 2, 0x13, 0x01)    // cipher_suites
	body = append(body, 1, 0)                // compression_methods
	body = binary.BigEndian.AppendUint16(body, uint16(len(extensions)))
	body = append(body, extensions...)

	hello := append([]byte{
		tlsHandshakeTypeClientHello,
		byte(len(body) >> 16),
		byte(len(body) >> 8),
		byte(len(body)),
	}, body...)

	// The handshake rides in a TLS record, which is where the sniffer starts
	// reading.
	record := make([]byte, 0, tlsRecordHeaderLen+len(hello))
	record = append(record, tlsRecordTypeHandshake, 0x03, 0x01, 0, 0)
	binary.BigEndian.PutUint16(record[3:5], uint16(len(hello)))
	return append(record, hello...)
}

const (
	tlsRecordTypeHandshake      = 0x16
	tlsRecordHeaderLen          = 5
	tlsHandshakeTypeClientHello = 0x01
)

// A connection that arrives as a fake IP with no record behind it is answered
// by reading it: the sniffer recovers the server name, and only then is the
// connection dialed. The recovery is what the fake-ip mode leans on for every
// rule that matches a domain.
func TestHandleTCPConnSniffsTheDomainAFakeIPCannotName(t *testing.T) {
	restoreStatus := status.Load()
	status.Store(Running)
	defer status.Store(restoreStatus)

	restoreMapper := resolver.DefaultHostMapper
	resolver.DefaultHostMapper = missingFakeRecordMapper{}
	defer func() { resolver.DefaultHostMapper = restoreMapper }()

	// The sniffed name maps to the test's own address, which is what keeps the
	// dial off the network the way the hosts file does in production.
	restoreHosts := resolver.DefaultHosts
	domainTrie := trie.New[resolver.HostValue]()
	value, err := resolver.NewHostValueByIPs([]netip.Addr{netip.MustParseAddr("127.0.0.1")})
	require.NoError(t, err)
	require.NoError(t, domainTrie.Insert("sniffed.example.com", value))
	domainTrie.Optimize()
	resolver.DefaultHosts = resolver.NewHosts(domainTrie)
	defer func() { resolver.DefaultHosts = restoreHosts }()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	target := listener.Addr().String()
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			server, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer server.Close()
				buf := make([]byte, 4096)
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

	dispatcher, err := sniffer.NewDispatcher(&sniffer.Config{
		// Reading a pure IP is what puts sniffing in charge of an IP
		// destination, and replacing the destination is what lets the name the
		// sniffer read carry the dial.
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
		invalidateAllProxies()
	})

	dialed := make(chan dialRecord, 1)
	UpdateProxies(map[string]C.Proxy{
		"DIRECT": recordingDialProxy{dialed: dialed, target: target},
	}, nil)
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
	replies := make(chan string, 4)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := client.Read(buf)
			if err != nil {
				return
			}
			replies <- string(buf[:n])
		}
	}()

	metadata := &C.Metadata{
		NetWork: C.TCP,
		Type:    C.INNER,
		SrcIP:   netip.AddrFrom4([4]byte{192, 168, 1, 100}),
		SrcPort: 53535,
		DstIP:   netip.MustParseAddr("198.18.0.1"),
		DstPort: 443,
	}

	go handleTCPConn(&prefilledConn{Conn: serverSide, buffered: testClientHello("sniffed.example.com")}, metadata)

	select {
	case name := <-dialed:
		require.Equal(t, "sniffed.example.com", name.sniffHost,
			"the connection was dialed with the name the sniffer read out of it")
	case <-time.After(5 * time.Second):
		t.Fatal("the connection was never dialed, so the sniff never named it")
	}

	_, err = client.Write([]byte("after-the-sniff"))
	require.NoError(t, err)

	// The bytes are echoed after the client hello the sniffer read ahead, and
	// how the echo splits across reads is not what the test is about.
	deadline := time.After(5 * time.Second)
	for {
		select {
		case reply := <-replies:
			if strings.Contains(reply, "echo:after-the-sniff") {
				return
			}
		case <-deadline:
			t.Fatal("the destination never answered the bytes after the client hello")
		}
	}

}

// A connection whose first bytes are already in the buffer: the sniffer reads
// them without waiting on the read deadline, which is what keeps the sniff
// independent of how quickly a test writes.
type prefilledConn struct {
	net.Conn
	buffered []byte
}

func (c *prefilledConn) Read(p []byte) (int, error) {
	if len(c.buffered) > 0 {
		n := copy(p, c.buffered)
		c.buffered = c.buffered[n:]
		return n, nil
	}
	return c.Conn.Read(p)
}

// With nothing to read a name from, a connection whose record is missing is
// dropped instead of dialed: the destination it would dial to is a fake IP with
// nothing behind it.
func TestHandleTCPConnDropsAFakeIPNothingSniffs(t *testing.T) {
	restoreStatus := status.Load()
	status.Store(Running)
	defer status.Store(restoreStatus)

	restoreMapper := resolver.DefaultHostMapper
	resolver.DefaultHostMapper = missingFakeRecordMapper{}
	defer func() { resolver.DefaultHostMapper = restoreMapper }()

	dialed := make(chan dialRecord, 1)
	UpdateProxies(map[string]C.Proxy{
		"DIRECT": recordingDialProxy{dialed: dialed, target: "127.0.0.1:1"},
	}, nil)
	SetMode(Direct)
	SetSniffing(false)
	t.Cleanup(func() {
		SetMode(Direct)
		SetSniffing(false)
		UpdateProxies(nil, nil)
	})

	client, serverSide := net.Pipe()
	t.Cleanup(func() {
		_ = client.Close()
		_ = serverSide.Close()
	})

	metadata := &C.Metadata{
		NetWork: C.TCP,
		Type:    C.INNER,
		SrcIP:   netip.AddrFrom4([4]byte{192, 168, 1, 100}),
		SrcPort: 53536,
		DstIP:   netip.MustParseAddr("198.18.0.1"),
		DstPort: 443,
	}

	handleTCPConn(serverSide, metadata)

	select {
	case name := <-dialed:
		t.Fatalf("the connection was dialed with %q after its record went missing", name)
	default:
	}
	_, err := client.Write([]byte("hello"))
	require.Error(t, err, "the connection was closed without a dial")
}
