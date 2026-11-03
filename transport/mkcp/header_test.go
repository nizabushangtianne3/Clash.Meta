package mkcp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPacketHeadersAcceptOnlySupportedModes(t *testing.T) {
	for _, name := range []string{"", "none", "noop", "srtp", "utp", "wechat-video", "wechat", "dtls", "wireguard"} {
		header, err := Config{Header: name}.packetHeader()
		require.NoError(t, err, name)
		require.NotNil(t, header, name)
	}

	header, err := Config{Header: "not-a-v2ray-header"}.packetHeader()
	require.Nil(t, header)
	require.EqualError(t, err, `mkcp: unsupported header "not-a-v2ray-header"`)
}
