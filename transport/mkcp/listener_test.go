package mkcp

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUDPListenerContextClosesListener(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	pc, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	defer pc.Close()

	listener, err := ListenUDP(ctx, pc, Config{})
	require.NoError(t, err)
	cancel()

	require.Eventually(t, func() bool {
		_, err := listener.Accept()
		return errors.Is(err, net.ErrClosed)
	}, time.Second, time.Millisecond)
}
