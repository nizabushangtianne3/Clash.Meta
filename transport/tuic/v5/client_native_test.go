package v5

import (
	"bytes"
	"net/netip"
	"testing"

	"github.com/metacubex/mihomo/common/pool"
	"github.com/stretchr/testify/require"
)

func wirePacket(test *testing.T, packet Packet) []byte {
	test.Helper()
	var wire bytes.Buffer
	require.NoError(test, packet.WriteTo(&wire))
	return wire.Bytes()
}

func TestReadPacketFromWireMatchesTheStreamCodec(test *testing.T) {
	payload := []byte{0xaa, 0xbb, 0xcc, 0xdd}
	packets := []Packet{
		NewPacket(1, 2, 1, 0, 4,
			NewAddressAddrPort(netip.MustParseAddrPort("203.0.113.7:443")), payload),
		NewPacket(3, 4, 1, 0, 4,
			NewAddressAddrPort(netip.MustParseAddrPort("[2001:db8::7]:443")), payload),
	}
	domain := []byte{11, 'e', 'x', 'a', 'm', 'p', 'l', 'e', '.', 'c', 'o', 'm'}
	packets = append(packets, NewPacket(5, 6, 1, 0, 4,
		Address{typ: AtypDomainName, domain: domain, port: 443}, payload))

	for _, packet := range packets {
		raw := wirePacket(test, packet)
		received, used, err := readPacketFromWire(raw)
		require.NoError(test, err)
		require.Equal(test, len(raw), used)
		require.Equal(test, packet.ASSOC_ID, received.ASSOC_ID)
		require.Equal(test, packet.PKT_ID, received.PKT_ID)
		require.Equal(test, packet.FRAG_TOTAL, received.FRAG_TOTAL)
		require.Equal(test, packet.FRAG_ID, received.FRAG_ID)
		require.Equal(test, packet.SIZE, received.SIZE)
		require.Equal(test, packet.ADDR.typ, received.ADDR.typ)
		require.Equal(test, packet.ADDR.ip, received.ADDR.ip)
		require.Equal(test, packet.ADDR.port, received.ADDR.port)
		require.Equal(test, packet.ADDR.domain, received.ADDR.domain)
		require.Equal(test, payload, received.DATA)
		pool.Put(received.DATA)
	}
}

func TestReadPacketFromWireRejectsShortAndUnknownDatagrams(test *testing.T) {
	_, _, err := readPacketFromWire(nil)
	require.ErrorIs(test, err, errBadPacketWire)

	packet := NewPacket(1, 2, 1, 0, 4,
		NewAddressAddrPort(netip.MustParseAddrPort("203.0.113.7:443")),
		[]byte{0xaa, 0xbb, 0xcc, 0xdd})
	raw := wirePacket(test, packet)
	_, _, err = readPacketFromWire(raw[:len(raw)-1])
	require.ErrorIs(test, err, errShortPacketWire)

	raw[packetFrameHeaderLen] = 0xfe
	_, _, err = readPacketFromWire(raw)
	require.ErrorIs(test, err, errBadPacketWire)
}

func TestReadPacketFromWireAllocatesOnlyAPooledPayload(test *testing.T) {
	if testing.Short() {
		test.Skip("allocation gates are skipped in short mode")
	}
	payload := make([]byte, 1350)
	packet := NewPacket(1, 2, 1, 0, 1350,
		NewAddressAddrPort(netip.MustParseAddrPort("203.0.113.7:443")), payload)
	raw := wirePacket(test, packet)
	warm, _, err := readPacketFromWire(raw)
	require.NoError(test, err)
	pool.Put(warm.DATA)

	allocations := testing.AllocsPerRun(1000, func() {
		received, _, err := readPacketFromWire(raw)
		if err != nil {
			test.Fatal(err)
		}
		pool.Put(received.DATA)
	})
	require.LessOrEqual(test, allocations, 0.5)
}

func TestNativeDatagramWaitReadFrom(test *testing.T) {
	payload := []byte{0xaa, 0xbb}
	packet := NewPacket(7, 8, 1, 0, 2,
		NewAddressAddrPort(netip.MustParseAddrPort("203.0.113.7:443")), payload)
	raw := wirePacket(test, packet)
	conn := &quicStreamPacketConn{
		packetCh:     make(chan Packet, 1),
		nativeClosed: make(chan struct{}),
	}
	conn.receiveDatagram(raw)

	data, source, err := conn.WaitReadFrom()
	require.NoError(test, err)
	require.Equal(test, payload, data)
	require.Equal(test, netip.MustParseAddrPort("203.0.113.7:443"), source)
	pool.Put(data)
}

func TestNativeDatagramWaitReadFromAssemblesFragments(test *testing.T) {
	var raw []byte
	payload := []byte("abcd")
	for id, size := range []int{2, 2} {
		data := pool.Get(size)
		copy(data, payload[id*size:(id+1)*size])
		packet := fragmentPacket(uint8(id), 2, uint16(size), data[:size])
		packet.ASSOC_ID = 7
		raw = append(raw, wirePacket(test, packet)...)
	}
	conn := &quicStreamPacketConn{
		packetCh:     make(chan Packet, 2),
		nativeClosed: make(chan struct{}),
	}
	conn.receiveDatagram(raw)
	data, source, err := conn.WaitReadFrom()
	require.NoError(test, err)
	require.Equal(test, payload, data)
	require.Equal(test, netip.MustParseAddrPort("203.0.113.7:443"), source)
	pool.Put(data)
	require.Empty(test, conn.deFragger.bags)
}
