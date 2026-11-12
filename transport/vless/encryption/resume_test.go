package encryption

import (
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The 0-RTT client writes its whole greeting without waiting for anything, so
// a reader on the other end that drains the socket is all the drive it needs.
func TestZeroRTTClientSendsATicketItCached(t *testing.T) {
	_, clientBase64, _, err := GenMLKEM768("")
	require.NoError(t, err)
	_, passwordBase64, _, err := GenX25519("")
	require.NoError(t, err)

	client, err := NewClient("mlkem768x25519plus.native.0rtt.100-35-35." + passwordBase64 + "." + clientBase64)
	require.NoError(t, err)

	// A live ticket is what puts this side on the 0-RTT branch; without one it
	// falls through to the key exchange and blocks waiting for the server.
	client.RWLock.Lock()
	client.PfsKey = make([]byte, 32)
	client.Ticket = make([]byte, 16)
	client.Expire = time.Now().Add(time.Hour)
	client.RWLock.Unlock()

	local, remote := loopbackPair(t)
	t.Cleanup(func() {
		local.Close()
		remote.Close()
	})

	sent := make(chan int, 1)
	go func() {
		got, err := io.ReadAll(remote)
		require.NoError(t, err)
		sent <- len(got)
	}()

	conn, err := client.Handshake(local)
	require.NoError(t, err)
	require.NotNil(t, conn)
	require.Nil(t, conn.PeerAEAD, "the peer's half arrives on its first write")
	require.NotEmpty(t, conn.PreWrite)

	// The greeting is only flushed by the first record, so this write is what
	// puts it on the wire together with the data behind it.
	writeErr := make(chan error, 1)
	go func() {
		n, err := conn.Write([]byte("the first record on a resumed session"))
		require.NoError(t, err)
		require.Equal(t, 37, n)
		writeErr <- err
		local.Close()
	}()
	require.Positive(t, <-sent)
	require.NoError(t, <-writeErr)
}

// A server handed bytes that are not a valid relay layout must fail rather
// than decode nonsense into a session key.
func TestServerRejectsAHandshakeItCannotRead(t *testing.T) {
	seedBase64, _, _, err := GenMLKEM768("")
	require.NoError(t, err)
	privateKeyBase64, _, _, err := GenX25519("")
	require.NoError(t, err)

	server, err := NewServer("mlkem768x25519plus.native.600s.100-35-35." + privateKeyBase64 + "." + seedBase64)
	require.NoError(t, err)

	local, remote := loopbackPair(t)
	t.Cleanup(func() {
		local.Close()
		remote.Close()
	})

	handshakeDone := make(chan error, 1)
	go func() {
		_, err := server.Handshake(remote, nil)
		handshakeDone <- err
	}()

	// Garbage in place of a greeting: the relay layout cannot be recovered, so
	// the handshake must end instead of building a key from it.
	_, err = local.Write(make([]byte, 16+server.RelaysLength+18))
	require.NoError(t, err)
	require.Error(t, <-handshakeDone)
}
