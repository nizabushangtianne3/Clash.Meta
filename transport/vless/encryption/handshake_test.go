package encryption

import (
	"bytes"
	"io"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

// loopbackPair gives the handshake a socket pair with kernel buffering, because
// both sides fragment their greeting before reading anything and an unbuffered
// pipe would deadlock on the first fragment.
func loopbackPair(t *testing.T) (net.Conn, net.Conn) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			accepted <- nil
			return
		}
		accepted <- conn
	}()

	client, err := net.Dial("tcp", listener.Addr().String())
	require.NoError(t, err)
	server := <-accepted
	require.NotNil(t, server)
	return client, server
}

// handshakePair runs the real handshake on a pipe, with the padding plan cut
// down so a test does not spend its life sleeping between fragments.
func handshakePair(t *testing.T, mode string, zeroRTT bool) (*CommonConn, *CommonConn) {
	t.Helper()

	// Client and server must share one X25519 pair and one ML-KEM seed: the
	// public halves go into the client string, the private halves into the
	// server's, exactly the way a user's config pairs them up.
	seedBase64, clientBase64, _, err := GenMLKEM768("")
	require.NoError(t, err)
	privateKeyBase64, passwordBase64, _, err := GenX25519("")
	require.NoError(t, err)

	rtt := "1rtt"
	if zeroRTT {
		rtt = "0rtt"
	}
	serverSeconds := "0s"
	if zeroRTT {
		serverSeconds = "600s"
	}

	client, err := NewClient("mlkem768x25519plus." + mode + "." + rtt + ".100-111-1111." + passwordBase64 + "." + clientBase64)
	require.NoError(t, err)
	server, err := NewServer("mlkem768x25519plus." + mode + "." + serverSeconds + ".100-111-1111." + privateKeyBase64 + "." + seedBase64)
	require.NoError(t, err)

	// The pair stays open for the caller to exchange frames on; closing it here
	// would kill the connection the handshake just finished.
	local, remote := loopbackPair(t)
	t.Cleanup(func() {
		local.Close()
		remote.Close()
	})

	serverDone := make(chan *CommonConn, 1)
	serverErr := make(chan error, 1)
	go func() {
		conn, err := server.Handshake(remote, nil)
		if err != nil {
			serverErr <- err
			return
		}
		serverDone <- conn
	}()

	clientConn, err := client.Handshake(local)
	if err != nil {
		t.Fatalf("client handshake: %v", err)
	}

	var peer *CommonConn
	select {
	case peer = <-serverDone:
	case err := <-serverErr:
		require.NoError(t, err)
	}
	return clientConn, peer
}

func TestHandshakeRoundTripsARecord(t *testing.T) {
	for _, mode := range []string{"native", "xorpub", "random"} {
		t.Run(mode, func(t *testing.T) {
			clientConn, serverConn := handshakePair(t, mode, false)

			payload := bytes.Repeat([]byte("a frame of payload"), 40)
			writeDone := make(chan error, 1)
			go func() {
				_, err := clientConn.Write(payload)
				writeDone <- err
			}()

			got := make([]byte, len(payload))
			_, err := io.ReadFull(serverConn, got)
			require.NoError(t, err)
			require.Equal(t, payload, got)
			require.NoError(t, <-writeDone)

			reply := []byte("and the same the other way")
			go func() {
				_, err := serverConn.Write(reply)
				writeDone <- err
			}()
			back := make([]byte, len(reply))
			_, err = io.ReadFull(clientConn, back)
			require.NoError(t, err)
			require.Equal(t, reply, back)
			require.NoError(t, <-writeDone)
		})
	}
}

// The outbound side splits at 8192 bytes, so a payload larger than that must
// arrive as several records the peer reassembles.
func TestHandshakeSplitsALargeWrite(t *testing.T) {
	clientConn, serverConn := handshakePair(t, "native", false)

	payload := bytes.Repeat([]byte("0123456789abcdef"), 2000) // 32000 bytes
	writeDone := make(chan error, 1)
	go func() {
		_, err := clientConn.Write(payload)
		writeDone <- err
	}()

	got := make([]byte, len(payload))
	_, err := io.ReadFull(serverConn, got)
	require.NoError(t, err)
	require.Equal(t, payload, got)
	require.NoError(t, <-writeDone)
}

func TestCommonConnWriteIgnoresAnEmptySlice(t *testing.T) {
	clientConn, _ := handshakePair(t, "native", false)
	n, err := clientConn.Write(nil)
	require.NoError(t, err)
	require.Equal(t, 0, n)
}

func TestCommonConnReadIgnoresAnEmptyBuffer(t *testing.T) {
	clientConn, _ := handshakePair(t, "native", false)
	n, err := clientConn.Read(nil)
	require.NoError(t, err)
	require.Equal(t, 0, n)
}

// A peer reading with a buffer smaller than the record gets the first slice,
// and the rest comes back on the following read without losing bytes.
func TestCommonConnReadSpillsTheRestIntoItsBuffer(t *testing.T) {
	clientConn, serverConn := handshakePair(t, "native", false)

	payload := []byte("a record longer than the read buffer it gets")
	go func() {
		_, err := serverConn.Write(payload)
		require.NoError(t, err)
	}()

	first := make([]byte, 10)
	n, err := io.ReadFull(clientConn, first)
	require.NoError(t, err)
	require.Equal(t, 10, n)
	require.Equal(t, payload[:10], first)

	rest := make([]byte, len(payload)-10)
	n, err = io.ReadFull(clientConn, rest)
	require.NoError(t, err)
	require.Equal(t, len(rest), n)
	require.Equal(t, payload[10:], rest)
}
