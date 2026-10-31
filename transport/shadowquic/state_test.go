package shadowquic

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConnStateReleasesAssociationIDs(t *testing.T) {
	state := &connState{activeSend: map[uint16]struct{}{}, recv: map[uint16]*recvSlot{}}
	active := &association{sendIDs: map[netip.AddrPort]uint16{}}
	retired := &association{sendIDs: map[netip.AddrPort]uint16{}}

	active.sendIDs[netip.MustParseAddrPort("1.2.3.4:53")] = 10
	active.sendIDs[netip.MustParseAddrPort("[2001:db8::1]:53")] = 11
	retired.sendIDs[netip.MustParseAddrPort("5.6.7.8:53")] = 12
	state.activeSend[10] = struct{}{}
	state.activeSend[11] = struct{}{}
	state.activeSend[12] = struct{}{}
	state.recv[12] = &recvSlot{target: &recvTarget{assoc: retired}}
	state.recv[13] = &recvSlot{target: &recvTarget{assoc: active}}

	state.releaseSendID(10)
	require.NotContains(t, state.activeSend, uint16(10))
	require.Contains(t, state.activeSend, uint16(11))

	state.removeRecvIDsFor(retired)
	require.NotContains(t, state.recv, uint16(12))
	require.Contains(t, state.recv, uint16(13))
}
