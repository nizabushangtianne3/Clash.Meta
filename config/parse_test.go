package config

import (
	"fmt"
	"strings"
	"testing"

	"github.com/metacubex/mihomo/component/geodata"
	"github.com/metacubex/mihomo/component/http"
	"github.com/metacubex/mihomo/component/resolver"
	"github.com/metacubex/mihomo/component/resource"
	T "github.com/metacubex/mihomo/tunnel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The smallest document that walks the path a config switch walks: two
// proxies, a group, a rule and the geodata settings the parse depends on.
const parseConfigDocument = `mixed-port: 7890
mode: rule
log-level: info
ipv6: false
geodata-mode: false
global-ua: mihomo-parse-test
etag-support: false
proxies:
  - {name: node-0000, type: ss, server: 10.0.0.1, port: 443, cipher: aes-128-gcm, password: password}
  - {name: node-0001, type: ss, server: 10.0.0.2, port: 443, cipher: aes-128-gcm, password: password}
proxy-groups:
  - {name: Proxy, type: select, proxies: [node-0000, node-0001]}
rules:
  - MATCH,DIRECT
`

func TestParseRawConfig(t *testing.T) {
	rawCfg, err := UnmarshalRawConfig([]byte(parseConfigDocument))
	require.NoError(t, err)

	// what a parse applies while it loads its rules is taken back out afterwards
	mode, ua, etag := T.Mode(), http.UA(), resource.ETag()
	geoIpUrl, geoSiteUrl := geodata.GeoIpUrl(), geodata.GeoSiteUrl()
	geodataMode, ipv6Disabled := geodata.GeodataMode(), resolver.DisableIPv6

	cfg, err := ParseRawConfig(rawCfg)
	require.NoError(t, err)

	assert.Contains(t, cfg.Proxies, "node-0000")
	assert.Contains(t, cfg.Proxies, "node-0001")
	assert.Contains(t, cfg.Proxies, "Proxy")
	assert.Len(t, cfg.Rules, 1)
	assert.Equal(t, T.Rule, cfg.General.Mode)
	assert.Equal(t, 7890, cfg.General.MixedPort)
	assert.Equal(t, "mihomo-parse-test", cfg.General.GlobalUA)

	assert.Equal(t, mode, T.Mode())
	assert.Equal(t, ua, http.UA())
	assert.Equal(t, etag, resource.ETag())
	assert.Equal(t, geodataMode, geodata.GeodataMode())
	assert.Equal(t, geoIpUrl, geodata.GeoIpUrl())
	assert.Equal(t, geoSiteUrl, geodata.GeoSiteUrl())
	assert.Equal(t, ipv6Disabled, resolver.DisableIPv6)
}

func TestTemporarilySetGeneralRollsBack(t *testing.T) {
	mode, ua, etag := T.Mode(), http.UA(), resource.ETag()
	geoIpUrl, geoSiteUrl := geodata.GeoIpUrl(), geodata.GeoSiteUrl()
	mmdbUrl, asnUrl := geodata.MmdbUrl(), geodata.ASNUrl()
	geodataMode, ipv6Disabled := geodata.GeodataMode(), resolver.DisableIPv6

	rollback := TemporarilySetGeneral(&General{
		Mode:        T.Direct,
		IPv6:        ipv6Disabled,
		GeodataMode: !geodataMode,
		GeoXUrl: GeoXUrl{
			GeoIp:   "https://example.invalid/geoip.dat",
			Mmdb:    "https://example.invalid/mmdb.dat",
			ASN:     "https://example.invalid/asn.dat",
			GeoSite: "https://example.invalid/geosite.dat",
		},
		GlobalUA:    "rollback-test",
		ETagSupport: !etag,
	})

	assert.Equal(t, T.Direct, T.Mode())
	assert.Equal(t, !ipv6Disabled, resolver.DisableIPv6)
	assert.Equal(t, !geodataMode, geodata.GeodataMode())
	assert.Equal(t, "https://example.invalid/geoip.dat", geodata.GeoIpUrl())
	assert.Equal(t, "https://example.invalid/geosite.dat", geodata.GeoSiteUrl())
	assert.Equal(t, "rollback-test", http.UA())
	assert.Equal(t, !etag, resource.ETag())

	rollback()

	assert.Equal(t, mode, T.Mode())
	assert.Equal(t, ipv6Disabled, resolver.DisableIPv6)
	assert.Equal(t, geodataMode, geodata.GeodataMode())
	assert.Equal(t, geoIpUrl, geodata.GeoIpUrl())
	assert.Equal(t, geoSiteUrl, geodata.GeoSiteUrl())
	assert.Equal(t, mmdbUrl, geodata.MmdbUrl())
	assert.Equal(t, asnUrl, geodata.ASNUrl())
	assert.Equal(t, ua, http.UA())
	assert.Equal(t, etag, resource.ETag())
}

// A large profile is what a config switch parses, so the parse itself is the
// startup cost this benchmark pins: the document is unmarshalled and parsed
// exactly as the executor does it.
func BenchmarkParseRawConfigBig(b *testing.B) {
	doc := bigParseDocument(800, 200)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rawCfg, err := UnmarshalRawConfig([]byte(doc))
		if err != nil {
			b.Fatal(err)
		}
		if _, err := ParseRawConfig(rawCfg); err != nil {
			b.Fatal(err)
		}
	}
}

func bigParseDocument(proxies, groups int) string {
	var sb strings.Builder
	sb.WriteString("mixed-port: 7890\nmode: rule\nlog-level: info\nipv6: false\ngeodata-mode: false\netag-support: false\nproxies:\n")
	for i := 0; i < proxies; i++ {
		fmt.Fprintf(&sb, "  - {name: node-%04d, type: ss, server: 10.0.%d.%d, port: 443, cipher: aes-128-gcm, password: password}\n", i, i/256%256, i%256)
	}
	sb.WriteString("proxy-groups:\n")
	for g := 0; g < groups; g++ {
		fmt.Fprintf(&sb, "  - {name: group-%03d, type: select, proxies: [node-%04d, node-%04d, node-%04d]}\n", g, g*3%proxies, (g*3+1)%proxies, (g*3+2)%proxies)
	}
	sb.WriteString("rules:\n")
	for g := 0; g < groups; g++ {
		fmt.Fprintf(&sb, "  - DOMAIN-SUFFIX,example-%03d.test,group-%03d\n", g, g)
	}
	sb.WriteString("  - MATCH,DIRECT\n")
	return sb.String()
}
