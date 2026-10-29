package tunnel

import (
	"net/netip"
	"testing"

	C "github.com/metacubex/mihomo/constant"
	"github.com/stretchr/testify/require"
)

type binaryFlowKeyPacket struct {
	C.PacketAdapter
	key netip.AddrPort
}

func (packet binaryFlowKeyPacket) FlowKey() netip.AddrPort {
	return packet.key
}

func TestPacketFlowKeyUsesBinaryKey(t *testing.T) {
	key := netip.MustParseAddrPort("192.168.1.2:54321")

	actual, ok := packetFlowKey(binaryFlowKeyPacket{key: key})
	require.True(t, ok)
	require.Equal(t, key, actual)
}

func TestPacketFlowKeyRejectsInvalidKey(t *testing.T) {
	_, ok := packetFlowKey(binaryFlowKeyPacket{})
	require.False(t, ok)
}
