package v4

import (
	"bufio"
	"bytes"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

func encodePacket(t *testing.T, packet Packet) []byte {
	t.Helper()
	var buffer bytes.Buffer
	require.NoError(t, packet.WriteTo(&buffer))
	return buffer.Bytes()
}

func TestReadPacketIntoKeepsTheAddrPort(t *testing.T) {
	for _, target := range []netip.AddrPort{
		netip.MustParseAddrPort("203.0.113.7:443"),
		netip.MustParseAddrPort("[2001:db8::7]:443"),
	} {
		packet := NewPacket(1, 4, NewAddressAddrPort(target), []byte{0xaa, 0xbb, 0xcc, 0xdd})
		raw := encodePacket(t, packet)
		scratch := make([]byte, addressScratchSize)
		reader := bufio.NewReaderSize(bytes.NewReader(raw), 4096)

		got, err := readPacketInto(reader, scratch)
		require.NoError(t, err)
		require.Equal(t, target, got.ADDR.AddrPort())
		require.Equal(t, packet.DATA, got.DATA)
	}
}
