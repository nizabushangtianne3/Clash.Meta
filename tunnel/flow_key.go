package tunnel

import (
	"net/netip"

	C "github.com/metacubex/mihomo/constant"
)

func packetFlowKey(packet C.PacketAdapter) (netip.AddrPort, bool) {
	key := packet.FlowKey()
	if !key.IsValid() {
		return netip.AddrPort{}, false
	}
	return key, true
}
