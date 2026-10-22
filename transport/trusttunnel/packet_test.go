package trusttunnel

import (
	"bytes"
	"io"
	"net/netip"
	"testing"

	"github.com/metacubex/sing/common/buf"
	M "github.com/metacubex/sing/common/metadata"
	"github.com/stretchr/testify/require"
)

func TestClientWriteToServerReadRoundTrip(t *testing.T) {
	var wire bytes.Buffer
	client := &clientPacketConn{
		packetConn: packetConn{
			httpConn: httpConn{writer: &wire},
		},
	}
	destination := M.SocksaddrFrom(netip.MustParseAddr("203.0.113.7"), 443)
	payload := []byte("hello")

	require.NoError(t, client.writePacketToServer(buf.As(payload), destination))

	server := &serverPacketConn{
		packetConn: packetConn{
			httpConn: httpConn{
				body: io.NopCloser(bytes.NewReader(wire.Bytes())),
			},
		},
	}
	gotBuffer := buf.NewSize(len(payload))
	gotDestination, err := server.readPacketFromClient(gotBuffer)
	require.NoError(t, err)
	require.Equal(t, destination, gotDestination)
	require.Equal(t, payload, gotBuffer.Bytes())
	gotBuffer.Release()
}

func TestServerWriteToClientReadRoundTrip(t *testing.T) {
	var wire bytes.Buffer
	server := &serverPacketConn{
		packetConn: packetConn{
			httpConn: httpConn{writer: &wire},
		},
	}
	source := M.SocksaddrFrom(netip.MustParseAddr("198.51.100.7"), 5353)
	payload := []byte("reply")

	require.NoError(t, server.writePacketToClient(buf.As(payload), source))

	client := &clientPacketConn{
		packetConn: packetConn{
			httpConn: httpConn{
				body: io.NopCloser(bytes.NewReader(wire.Bytes())),
			},
		},
	}
	gotBuffer := buf.NewSize(len(payload))
	gotDestination, err := client.readPacketFromServer(gotBuffer)
	require.NoError(t, err)
	require.Equal(t, source, gotDestination)
	require.Equal(t, payload, gotBuffer.Bytes())
	gotBuffer.Release()
}
