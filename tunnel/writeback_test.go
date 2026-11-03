package tunnel

import (
	"net"
	"net/netip"
	"testing"

	C "github.com/metacubex/mihomo/constant"
	"github.com/stretchr/testify/require"
)

type flowWriteBackPacket struct{}

func (flowWriteBackPacket) Data() []byte { return []byte("ping") }
func (flowWriteBackPacket) Drop()        {}
func (flowWriteBackPacket) LocalAddr() net.Addr {
	return net.UDPAddrFromAddrPort(netip.MustParseAddrPort("127.0.0.1:53535"))
}
func (flowWriteBackPacket) WriteBack(b []byte, _ netip.AddrPort) (int, error) { return len(b), nil }

func TestUDPFlowOwnsWriteBackProxy(t *testing.T) {
	metadata := &C.Metadata{NetWork: C.UDP, DstIP: netip.MustParseAddr("203.0.113.20"), DstPort: 53}
	flow, err := newUDPFlowMetadata(metadata)
	require.NoError(t, err)

	adapter := C.NewPacketAdapter(flowWriteBackPacket{}, metadata)
	flow.writeBack.UpdateWriteBack(adapter)

	n, err := flow.writeBack.WriteBack([]byte("pong"), netip.MustParseAddrPort("198.51.100.7:5353"))
	require.NoError(t, err)
	require.Equal(t, 4, n)
}
