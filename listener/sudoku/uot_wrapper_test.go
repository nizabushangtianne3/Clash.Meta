package sudoku

import (
	"net"
	"net/netip"
	"testing"

	N "github.com/metacubex/mihomo/common/net"
	"github.com/metacubex/mihomo/common/pool"
	"github.com/metacubex/mihomo/common/utils"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/transport/sudoku"
	"github.com/stretchr/testify/require"
)

func newTestUoTPacket(t *testing.T) (packet *uotPacket, peer net.Conn, connID string) {
	t.Helper()

	server, client := net.Pipe()
	t.Cleanup(func() {
		_ = server.Close()
		_ = client.Close()
	})

	connID = utils.NewUUIDV4().String() // the SNAT key the tunnel stores
	packet = &uotPacket{
		payload: pool.Get(8),
		writer:  sudoku.NewUoTPacketConn(server),
		rAddr:   N.NewCustomAddr(C.SUDOKU.String(), connID, server.RemoteAddr()),
	}
	return packet, client, connID
}

// The tunnel answers a flow through the last packet the sender handed it, so
// the wrapper has to keep its writer after the payload is released.
func TestUoTPacketWriteBackSurvivesDrop(t *testing.T) {
	packet, peer, _ := newTestUoTPacket(t)

	replyTarget := netip.MustParseAddrPort("5.6.7.8:1234")
	received := make(chan netip.AddrPort, 1)
	go func() {
		if addr, _, err := sudoku.NewUoTReader(peer).ReadDatagram(); err == nil {
			received <- netip.MustParseAddrPort(addr.String())
		}
	}()

	packet.Drop()
	require.Nil(t, packet.payload, "a dropped packet must release its payload")

	n, err := packet.WriteBack([]byte("reply"), replyTarget)
	require.NoError(t, err)
	require.Equal(t, len("reply"), n)
	require.Equal(t, replyTarget, <-received)
}

// Drop is single use: the second call must not hand the pool a slice it no
// longer owns.
func TestUoTPacketDropReleasesThePayloadOnce(t *testing.T) {
	packet, _, _ := newTestUoTPacket(t)

	packet.Drop()
	require.Nil(t, packet.payload)

	packet.Drop()
	require.Nil(t, packet.payload)
	require.NotNil(t, packet.writer)
}

// The wrapper reports the flow's SNAT key to the tunnel and keeps the real peer
// underneath it.
func TestUoTPacketLocalAddrCarriesTheFlow(t *testing.T) {
	packet, _, connID := newTestUoTPacket(t)

	require.Equal(t, connID, packet.LocalAddr().String())
	require.Equal(t, C.SUDOKU.String(), packet.LocalAddr().Network())

	if custom, ok := packet.LocalAddr().(N.CustomAddr); ok {
		require.NotNil(t, custom.RawAddr())
	}
}
