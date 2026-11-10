package tunnel

import (
	"context"
	"net"
	"testing"

	N "github.com/metacubex/mihomo/common/net"
	C "github.com/metacubex/mihomo/constant"
	"github.com/stretchr/testify/require"
)

// The sender answers the small surface a write-back proxy needs, and the only
// answer that changes after a close is the error.
func TestPacketSenderAnswersItsWriteBackSurface(t *testing.T) {
	sender := newPacketSender().(*packetSender)

	deadline, ok := sender.Deadline()
	require.False(t, ok, "a sender has no deadline of its own")
	require.True(t, deadline.IsZero())
	require.NotNil(t, sender.Done(), "a caller may wait on the sender")
	require.NoError(t, sender.Err())
	require.Nil(t, sender.Value("anything"))
	require.NoError(t, sender.DoSniff(&C.Metadata{}))

	sender.Close()
	require.ErrorIs(t, sender.Err(), context.Canceled,
		"a closed sender reports why the caller was let go")
}

// A packet with no address has nowhere to go, and one with an address is
// written to the conn the caller handed over.
func TestHandleUDPToRemoteNeedsAnAddressAndWritesThePacket(t *testing.T) {
	socket, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	t.Cleanup(func() { _ = socket.Close() })
	pc := &chainlessPacketConn{EnhancePacketConn: N.NewEnhancePacketConn(socket)}

	packet := deadlineTestPacket{
		localAddr: &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 53535},
		data:      []byte("ping"),
	}

	require.Error(t, handleUDPToRemote(packet, pc, nil),
		"a packet with no address is refused, not dropped silently")

	remote, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	t.Cleanup(func() { _ = remote.Close() })

	delivered := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 16)
		n, _, _ := remote.ReadFrom(buf)
		delivered <- buf[:n]
	}()

	require.NoError(t, handleUDPToRemote(packet, pc, remote.LocalAddr().(*net.UDPAddr)))
	require.Equal(t, []byte("ping"), <-delivered)
}
