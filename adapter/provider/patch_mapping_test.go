package provider

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/metacubex/mihomo/component/resource"

	"github.com/stretchr/testify/require"
)

// The proxy detail dialog reads one proxy out of the provider's content with
// the provider's own filters and overrides applied, so the mapping it answers
// with has to be the one the provider would build.
func TestProxyMappingReturnsTheNamedProxyWithTheProviderApplied(t *testing.T) {
	path := filepath.Join(t.TempDir(), "members.yaml")
	content := "proxies:\n" +
		"  - {name: node-a, type: ss, server: a.example, port: 8388, cipher: aes-128-gcm, password: pw}\n" +
		"  - {name: node-b, type: trojan, server: b.example, port: 443, password: pw, sni: b.example}\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	hc := NewHealthCheck(nil, "", 0, 0, false, nil)
	pd, err := NewProxySetProvider("members", 0, nil, memberListParser(), resource.NewFileVehicle(path), hc)
	require.NoError(t, err)

	got, err := ProxyMapping(pd, map[string]any{
		"type":         "file",
		"dialer-proxy": "dialer",
		"override":     map[string]any{"udp": true},
	}, "node-b")
	require.NoError(t, err)
	require.Equal(t, "node-b", got["name"])
	require.Equal(t, true, got["udp"], "the provider's override has to reach the mapping")
	require.Equal(t, "dialer", got["dialer-proxy"])

	missing, err := ProxyMapping(pd, map[string]any{"type": "file"}, "node-z")
	require.NoError(t, err)
	require.Nil(t, missing, "a name the provider does not carry answers nil, not an empty mapping")
}

// The filters name the members the provider keeps, so a proxy a filter drops
// is not a member and answers nil.
func TestProxyMappingHonoursTheProviderFilters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "members.yaml")
	content := "proxies:\n" +
		"  - {name: node-a, type: ss, server: a.example, port: 8388, cipher: aes-128-gcm, password: pw}\n" +
		"  - {name: node-b, type: trojan, server: b.example, port: 443, password: pw}\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	hc := NewHealthCheck(nil, "", 0, 0, false, nil)
	pd, err := NewProxySetProvider("members", 0, nil, memberListParser(), resource.NewFileVehicle(path), hc)
	require.NoError(t, err)

	for _, test := range []struct {
		name    string
		mapping map[string]any
		want    string
	}{
		// The schema's vehicle type is required, so every mapping names one.
		{"filter keeps it", withFilterType("filter", "node-b"), "node-b"},
		{"filter drops it", withFilterType("filter", "node-a"), ""},
		{"exclude filter drops it", withFilterType("exclude-filter", "node-b"), ""},
		{"exclude type drops it", withFilterType("exclude-type", "trojan"), ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := ProxyMapping(pd, test.mapping, "node-b")
			require.NoError(t, err)
			if test.want == "" {
				require.Nil(t, got, "a filtered-out proxy answers nil")
				return
			}
			require.NotNil(t, got)
			require.Equal(t, test.want, got["name"])
		})
	}

	// A provider whose filter does not compile answers the error, so a typo in
	// the config reaches the host instead of silently dropping every member.
	_, err = ProxyMapping(pd, withFilterType("filter", "["), "node-b")
	require.Error(t, err, "a filter that does not compile has to be reported")
}

func withFilterType(key, value string) map[string]any {
	return map[string]any{"type": "file", key: value}
}

// A provider that carries its payload instead of a file answers from it.
func TestProxyMappingAnswersFromThePayload(t *testing.T) {
	payload := []map[string]any{{
		"name": "node-a", "type": "ss", "server": "a.example", "port": 8388,
		"cipher": "aes-128-gcm", "password": "pw",
	}}
	// A provider of no particular kind holds its content as a payload rather
	// than a file, so the answer comes from the mapping's own payload.
	carrier, err := NewCompatibleProvider("inline", directProxies("carry"), NewHealthCheck(nil, "", 0, 0, false, nil))
	require.NoError(t, err)
	got, err := ProxyMapping(carrier, map[string]any{"type": "inline", "payload": payload}, "node-a")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "node-a", got["name"])
}
