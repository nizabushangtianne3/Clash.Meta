package constant

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetRemoteAddressTakesAnIPDestination(t *testing.T) {
	metadata := &Metadata{}

	require.NoError(t, metadata.SetRemoteAddress("192.168.1.2:443"))

	assert.Empty(t, metadata.Host)
	assert.Equal(t, netip.MustParseAddr("192.168.1.2"), metadata.DstIP)
	assert.EqualValues(t, 443, metadata.DstPort)
	assert.Equal(t, "192.168.1.2:443", metadata.RemoteAddress())
}

func TestSetRemoteAddressKeepsAnIPv6Destination(t *testing.T) {
	metadata := &Metadata{}

	require.NoError(t, metadata.SetRemoteAddress("[2001:db8::1]:8443"))

	assert.Equal(t, netip.MustParseAddr("2001:db8::1"), metadata.DstIP)
	assert.EqualValues(t, 8443, metadata.DstPort)
}

func TestSetRemoteAddressKeepsADomainDestination(t *testing.T) {
	metadata := &Metadata{}

	require.NoError(t, metadata.SetRemoteAddress("example.com:80"))

	assert.Equal(t, "example.com", metadata.Host)
	assert.False(t, metadata.DstIP.IsValid())
	assert.EqualValues(t, 80, metadata.DstPort)

	// A trailing dot is left alone here; the callers that trim it do so on
	// their own.
	require.NoError(t, metadata.SetRemoteAddress("example.com.:80"))
	assert.Equal(t, "example.com.", metadata.Host)
}

func TestSetRemoteAddressRejectsWhatItCannotParse(t *testing.T) {
	for _, rawAddress := range []string{
		"no-port-here",
		"192.168.1.2",
		"192.168.1.2:",
		// A port that is not a number used to be accepted and silently turned
		// into port zero, which sends the connection nowhere.
		"192.168.1.2:http",
		"192.168.1.2:-1",
		"192.168.1.2:65536",
		"192.168.1.2:0x50",
	} {
		metadata := &Metadata{}
		err := metadata.SetRemoteAddress(rawAddress)

		require.Error(t, err, rawAddress)
		assert.EqualValues(t, 0, metadata.DstPort, rawAddress)
	}
}

func TestMetadataSourceAddressMatchesJoinHostPort(t *testing.T) {
	tests := []struct {
		name string
		addr netip.Addr
		want string
	}{
		{name: "ipv4", addr: netip.MustParseAddr("192.168.1.2"), want: "192.168.1.2:54321"},
		{name: "ipv6", addr: netip.MustParseAddr("2001:db8::1"), want: "[2001:db8::1]:54321"},
		{name: "ipv6-zone", addr: netip.MustParseAddr("fe80::1%eth0"), want: "[fe80::1%eth0]:54321"},
		{name: "ipv4-in-ipv6", addr: netip.AddrFrom16(netip.MustParseAddr("192.0.2.1").As16()), want: "[::ffff:192.0.2.1]:54321"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			metadata := Metadata{SrcIP: test.addr, SrcPort: 54321}
			require.Equal(t, test.want, metadata.SourceAddress())
		})
	}

	invalid := Metadata{SrcPort: 54321}
	require.Equal(t, "invalid IP:54321", invalid.SourceAddress())
}

func TestMetadataSourceDetailMatchesLegacyFormatting(t *testing.T) {
	base := Metadata{SrcIP: netip.MustParseAddr("192.168.1.2"), SrcPort: 54321}
	require.Equal(t, "192.168.1.2:54321", base.SourceDetail())

	process := base
	process.Process = "chrome.exe"
	require.Equal(t, "192.168.1.2:54321(chrome.exe)", process.SourceDetail())

	uid := base
	uid.Uid = 1001
	require.Equal(t, "192.168.1.2:54321(uid=1001)", uid.SourceDetail())

	processUID := base
	processUID.Process = "chrome.exe"
	processUID.Uid = 1001
	require.Equal(t, "192.168.1.2:54321(chrome.exe, uid=1001)", processUID.SourceDetail())

	inner := base
	inner.Type = INNER
	require.Equal(t, MihomoName, inner.SourceDetail())
}
