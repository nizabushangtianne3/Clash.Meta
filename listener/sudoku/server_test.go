package sudoku

import (
	"errors"
	"io"
	"net"
	"os"
	"testing"
	"time"

	N "github.com/metacubex/mihomo/common/net"
	"github.com/stretchr/testify/require"
)

// A pipe already carries a correct deadline implementation, so wrapping it would
// add a layer that cannot honour the handshake timeout it is given.
func TestWrapSessionConnLeavesAPipeAlone(t *testing.T) {
	server, client := net.Pipe()
	t.Cleanup(func() {
		_ = server.Close()
		_ = client.Close()
	})

	wrapped := wrapSessionConn(server)
	require.NotNil(t, wrapped)
	require.Equal(t, server, N.UnwrapReader(wrapped), "a pipe must pass through unwrapped")

	// Deadline support is what the wrapper exists to keep; a pipe already has it.
	require.NoError(t, wrapped.SetReadDeadline(time.Now().Add(time.Second)))
	require.NoError(t, wrapped.SetWriteDeadline(time.Now().Add(time.Second)))
}

// A plain socket carries no deadline support, so the wrapper is what turns the
// peek deadlines mihomo applies from a no-op into a real interruption.
func TestWrapSessionConnWrapsAPlainConn(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })

	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		require.NoError(t, err)
		accepted <- conn
	}()

	dialed, err := net.Dial("tcp", listener.Addr().String())
	require.NoError(t, err)
	t.Cleanup(func() { _ = dialed.Close() })

	server := <-accepted
	t.Cleanup(func() { _ = server.Close() })

	wrapped := wrapSessionConn(server)
	require.NoError(t, wrapped.SetReadDeadline(time.Now().Add(50*time.Millisecond)))

	// Nothing arrives, so the deadline the wrapper accepted must fire.
	_, err = wrapped.Read(make([]byte, 1))
	require.Error(t, err)
	require.True(t, errors.Is(err, os.ErrDeadlineExceeded), "got %v", err)
}

func TestRawAddressReportsTheConfiguredListen(t *testing.T) {
	l := &Listener{addr: "127.0.0.1:7890"}
	require.Equal(t, "127.0.0.1:7890", l.RawAddress())
}

func TestAddressFallsBackWithoutAListener(t *testing.T) {
	require.Equal(t, "", (&Listener{}).Address())
}

// Closing twice must not panic and must still report closed, because the accept
// loop checks the flag when it wakes up on the listener's error.
func TestListenerCloseIsIdempotent(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	l := &Listener{addr: listener.Addr().String(), listener: listener}
	require.NotEmpty(t, l.Address())

	require.NoError(t, l.Close())
	require.True(t, l.closed)
	// A second close on a closed socket is an error, not a panic, and the flag
	// keeps the accept loop from treating it as a shutdown request of its own.
	require.Error(t, l.Close())
	require.True(t, l.closed)
}

func TestListenerCloseWithoutAListener(t *testing.T) {
	l := &Listener{}
	require.NoError(t, l.Close())
	require.True(t, l.closed)
}

// The fallback path replays the bytes the wrapper already read, so a peer that
// spoke a different protocol still gets a complete conversation.
func TestRelayToFallbackReplaysTheBufferedReads(t *testing.T) {
	buffered := []byte("GET / HTTP/1.1`r`nHost: example.com`r`n")

	// The peer that would have spoken a different protocol reads what the relay
	// hands it, so the replay has somewhere to land.
	fallback, fallbackPeer := net.Pipe()
	t.Cleanup(func() {
		_ = fallback.Close()
		_ = fallbackPeer.Close()
	})

	replayed := make(chan string, 1)
	go func() {
		buf := make([]byte, len(buffered))
		_, err := io.ReadFull(fallbackPeer, buf)
		if err != nil {
			replayed <- ""
			return
		}
		replayed <- string(buf)
	}()

	// The raw connection stays open-ended, because replaying is all this case
	// cares about: the bytes a wrapper already consumed must reach the fallback
	// before the relay takes over.
	raw, _ := net.Pipe()
	t.Cleanup(func() { _ = raw.Close() })

	relayDone := make(chan struct{})
	go func() {
		defer close(relayDone)
		relayToFallback(&recordingConn{Conn: raw, buffered: buffered}, raw, fallback)
	}()

	require.Equal(t, string(buffered), <-replayed)

	// Hanging up on the raw side is what ends the relay, so the test does not
	// leave a goroutine copying against a closed pipe.
	_ = raw.Close()
	select {
	case <-relayDone:
	case <-time.After(10 * time.Second):
		t.Fatal("relayToFallback did not return once the raw side closed")
	}
}

func TestRelayToFallbackWithoutBufferedReads(t *testing.T) {
	fallback, client := net.Pipe()
	t.Cleanup(func() {
		_ = fallback.Close()
		_ = client.Close()
	})

	done := make(chan struct{})
	go func() {
		relayToFallback(&recordingConn{Conn: client}, client, fallback)
		close(done)
	}()

	_ = client.Close()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("relayToFallback did not finish once the raw side closed")
	}
}

// recordingConn is the wrapper a failed handshake leaves behind: it holds the
// bytes that were already read off the wire, and relays the raw connection.
type recordingConn struct {
	net.Conn
	buffered []byte
}

func (c *recordingConn) GetBufferedAndRecorded() []byte {
	if c.buffered == nil {
		return nil
	}
	return c.buffered
}

// The datagram the tunnel is handed is the payload the reader produced, and it
// must survive being read before the sender takes it back.
func TestUoTPacketDataHandsBackThePayload(t *testing.T) {
	packet, peer, _ := newTestUoTPacket(t)
	t.Cleanup(func() { _ = peer.Close() })

	payload := []byte("datagram")
	copy(packet.payload, payload)

	require.Equal(t, payload, packet.Data())
	packet.Drop()
	require.Nil(t, packet.payload)
}
