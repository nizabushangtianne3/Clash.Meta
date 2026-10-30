package v5

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPacketRoutingReadsOnlyTheRouteHeader(t *testing.T) {
	packet := NewPacket(7, 8, 1, 0, 4,
		NewAddressAddrPort(netip.MustParseAddrPort("203.0.113.7:443")),
		[]byte{0xaa, 0xbb, 0xcc, 0xdd})
	raw := encodePacket(t, packet)

	assocId, ok := packetRouting(raw[:packetCommandHeadLen+packetRoutingHeaderLen])
	require.True(t, ok)
	require.EqualValues(t, 7, assocId)
}

func TestRawPacketFrameLen(t *testing.T) {
	tests := []struct {
		name   string
		packet Packet
	}{
		{
			name: "ipv4",
			packet: NewPacket(1, 2, 1, 0, 4,
				NewAddressAddrPort(netip.MustParseAddrPort("203.0.113.7:443")),
				[]byte{0xaa, 0xbb, 0xcc, 0xdd}),
		},
		{
			name: "ipv6",
			packet: NewPacket(1, 2, 1, 0, 4,
				NewAddressAddrPort(netip.MustParseAddrPort("[2001:db8::7]:443")),
				[]byte{0xaa, 0xbb, 0xcc, 0xdd}),
		},
		{
			name:   "domain",
			packet: NewPacket(1, 2, 1, 0, 4, Address{typ: AtypDomainName, domain: []byte{11, 'e', 'x', 'a', 'm', 'p', 'l', 'e', '.', 'c', 'o', 'm'}, port: 443}, []byte{0xaa, 0xbb, 0xcc, 0xdd}),
		},
		{
			name: "fragment without address",
			packet: NewPacket(1, 2, 2, 1, 2, Address{typ: AtypNone},
				[]byte{0xaa, 0xbb}),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw := encodePacket(t, test.packet)
			frameLen, ok := rawPacketFrameLen(raw[:min(len(raw), packetFramePrefixCap)])
			require.True(t, ok)
			require.Equal(t, len(raw), frameLen)
		})
	}
}

func TestRawPacketFrameLenRejectsShortAndUnknownHeaders(t *testing.T) {
	_, ok := rawPacketFrameLen(nil)
	require.False(t, ok)

	packet := NewPacket(1, 2, 1, 0, 4,
		NewAddressAddrPort(netip.MustParseAddrPort("203.0.113.7:443")),
		[]byte{0xaa, 0xbb, 0xcc, 0xdd})
	raw := encodePacket(t, packet)
	raw[packetFrameHeaderLen] = 0xfe
	_, ok = rawPacketFrameLen(raw[:packetFramePrefixCap])
	require.False(t, ok)
}
