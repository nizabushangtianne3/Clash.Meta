package adapter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The proxy answers the three reads the tunnel makes of it, and a url whose
// scheme carries no port is refused with the scheme named.
func TestProxyAnswersTheReadsTheTunnelMakes(t *testing.T) {
	proxy := testProxy()

	require.NotNil(t, proxy.Adapter(), "the adapter the proxy wraps comes back")
	require.Empty(t, proxy.DelayHistory(), "a proxy that never answered has no history")
	require.Empty(t, proxy.DelayHistoryForTestUrl("https://example.com"),
		"nor for any single url")
}

func TestURLToMetadataOpensTheDefaultPortsAndRefusesTheRest(t *testing.T) {
	for _, tc := range []struct {
		rawURL string
		port   uint16
	}{
		{"http://example.com", 80},
		{"https://example.com", 443},
		{"https://example.com:8443", 8443},
	} {
		metadata, err := urlToMetadata(tc.rawURL)
		require.NoError(t, err, tc.rawURL)
		require.Equal(t, tc.port, metadata.DstPort, tc.rawURL)
		require.False(t, metadata.DstIP.IsValid(),
			"a url is a host, not an address, until it is resolved")
	}

	_, err := urlToMetadata("not a url")
	require.Error(t, err)

	_, err = urlToMetadata("ftp://example.com")
	require.ErrorContains(t, err, "not Support",
		"a scheme that carries no port of its own is refused by name")
}
