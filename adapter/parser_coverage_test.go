package adapter

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/require"
)

// A raw key is 32 random bytes to whoever only parses it: no handshake runs
// here, so a construct never learns the key is not a real one.
func wireGuardKey(t *testing.T) string {
	t.Helper()
	raw := make([]byte, 32)
	_, err := rand.Read(raw)
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(raw)
}

// MASQUE authenticates a real EC key, so the test mints a pair instead of
// pretending raw bytes are keys.
func masqueKeys(t *testing.T) (private string, public string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	der, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	spki, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(der), base64.StdEncoding.EncodeToString(spki)
}

// Every branch of ParseProxy needs a witness: the switch is the single place a
// mapping becomes a proxy, and a type nobody parses is a type nobody can dial.
// Only tailscale stays uncovered: the default build disables it behind a build
// tag, so the type cannot be parsed here even though the branch exists.
func TestParseProxyCoversEveryBranchItCanBuild(t *testing.T) {
	for _, tc := range []struct {
		proxyType string
		mapping   map[string]any
	}{
		{"ss", map[string]any{
			"name": "node", "type": "ss", "server": "10.0.0.1", "port": 443,
			"cipher": "aes-128-gcm", "password": "password",
		}},
		{"ssr", map[string]any{
			"name": "node", "type": "ssr", "server": "10.0.0.1", "port": 443,
			"cipher": "aes-128-cfb", "password": "password", "protocol": "origin", "obfs": "plain",
		}},
		{"socks5", map[string]any{"name": "node", "type": "socks5", "server": "10.0.0.1", "port": 1080}},
		{"http", map[string]any{"name": "node", "type": "http", "server": "10.0.0.1", "port": 8080}},
		{"vmess", map[string]any{
			"name": "node", "type": "vmess", "server": "10.0.0.1", "port": 443,
			"uuid": "13868f42-9809-4d0e-8679-b8537bad1c47", "alterId": 0, "cipher": "auto",
		}},
		{"vless", map[string]any{
			"name": "node", "type": "vless", "server": "10.0.0.1", "port": 443,
			"uuid": "13868f42-9809-4d0e-8679-b8537bad1c47",
		}},
		{"snell", map[string]any{
			"name": "node", "type": "snell", "server": "10.0.0.1", "port": 443, "psk": "secret",
		}},
		{"trojan", map[string]any{
			"name": "node", "type": "trojan", "server": "10.0.0.1", "port": 443, "password": "password",
		}},
		{"hysteria2", map[string]any{
			"name": "node", "type": "hysteria2", "server": "10.0.0.1", "port": 443, "password": "password",
		}},
		{"hysteria", map[string]any{
			"name": "node", "type": "hysteria", "server": "10.0.0.1", "port": 443,
			"up": "10 Mbps", "down": "50 Mbps", "auth-str": "password",
		}},
		{"tuic", map[string]any{
			"name": "node", "type": "tuic", "server": "10.0.0.1", "port": 443,
			"uuid": "13868f42-9809-4d0e-8679-b8537bad1c47", "password": "password",
		}},
		{"ssh", map[string]any{
			"name": "node", "type": "ssh", "server": "10.0.0.1", "port": 22,
			"username": "user", "password": "password",
		}},
		{"anytls", map[string]any{
			"name": "node", "type": "anytls", "server": "10.0.0.1", "port": 443,
			"password": "password", "sni": "example.com",
		}},
		{"mieru", map[string]any{
			"name": "node", "type": "mieru", "server": "10.0.0.1", "port": 8443,
			"username": "user", "password": "password", "transport": "TCP",
		}},
		{"sudoku", map[string]any{
			"name": "node", "type": "sudoku", "server": "10.0.0.1", "port": 8443,
			"key": "0123456789abcdef", "aead-method": "chacha20-poly1305",
		}},
		{"gost-relay", map[string]any{
			"name": "node", "type": "gost-relay", "server": "10.0.0.1", "port": 8443,
		}},
		{"shadowquic", map[string]any{
			"name": "node", "type": "shadowquic", "server": "10.0.0.1", "port": 8443,
			"username": "user", "password": "password",
		}},
		{"trusttunnel", map[string]any{
			"name": "node", "type": "trusttunnel", "server": "10.0.0.1", "port": 8443,
			"username": "user", "password": "password",
		}},
		{"easytier", map[string]any{
			"name": "node", "type": "easytier", "network-name": "net", "network-secret": "secret",
			"peers": []string{"tcp://10.0.0.1:11010"},
		}},
		{"rematch", map[string]any{
			"name": "node", "type": "rematch", "target-rematch-name": "GLOBAL",
		}},
		{"direct", map[string]any{"name": "node", "type": "direct"}},
		{"dns", map[string]any{"name": "node", "type": "dns"}},
		{"reject", map[string]any{"name": "node", "type": "reject"}},
		{"wireguard", map[string]any{
			"name": "node", "type": "wireguard", "server": "10.0.0.1", "port": 51820,
			"ip": "10.0.0.2/32", "private-key": wireGuardKey(t),
			"peers": []map[string]any{{
				"server": "10.0.0.1", "port": 51820,
				"public-key":  wireGuardKey(t),
				"allowed-ips": []string{"0.0.0.0/0"},
			}},
		}},
		{"openvpn", map[string]any{
			"name": "node", "type": "openvpn", "server": "10.0.0.1", "port": 1194,
			"proto": "udp", "dev": "tun", "cipher": "AES-128-GCM", "auth": "SHA256",
			"ca":       "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n",
			"username": "user", "password": "password",
		}},
		{"zerotier", map[string]any{
			"name": "node", "type": "zerotier", "network": "8056c2e21c000001",
		}},
	} {
		t.Run(tc.proxyType, func(t *testing.T) {
			proxy, err := ParseProxy(tc.mapping, nil)
			require.NoError(t, err)
			require.NotNil(t, proxy)
			require.Equal(t, "node", proxy.Name())
		})
	}
}

// MASQUE rides the same EC key a wireguard peer would, so it is parsed apart
// from the table above: the pair is minted once and both halves are used.
func TestParseProxyBuildsAMasque(t *testing.T) {
	private, public := masqueKeys(t)

	proxy, err := ParseProxy(map[string]any{
		"name": "node", "type": "masque", "server": "10.0.0.1", "port": 443,
		"ip": "10.0.0.2/32", "private-key": private, "public-key": public,
	}, nil)
	require.NoError(t, err)
	require.NotNil(t, proxy)
	require.Equal(t, "node", proxy.Name())
}

// The options carry what a provider is: a proxy parsed for one reports that
// provider, and one parsed outside any reports none.
func TestParseProxyCarriesTheProviderFromOptions(t *testing.T) {
	mapping := map[string]any{
		"name": "node", "type": "ss", "server": "10.0.0.1", "port": 443,
		"cipher": "aes-128-gcm", "password": "password",
	}

	proxy, err := ParseProxy(mapping, &ProxyOptions{ProviderName: "provider-a"})
	require.NoError(t, err)
	require.Equal(t, "provider-a", proxy.ProxyInfo().ProviderName)

	alone, err := ParseProxy(mapping, &ProxyOptions{})
	require.NoError(t, err)
	require.Empty(t, alone.ProxyInfo().ProviderName)
}

func TestParseProxyReportsTheBranchesItCannotBuild(t *testing.T) {
	_, err := ParseProxy(map[string]any{"name": "node"}, nil)
	require.ErrorContains(t, err, "missing type")

	_, err = ParseProxy(map[string]any{"name": "node", "type": "no-such-type"}, nil)
	require.ErrorContains(t, err, "unsupport proxy type: no-such-type")

	_, err = ParseProxy(map[string]any{
		"name": "node", "type": "ss", "server": "10.0.0.1", "port": 443,
		"cipher": "aes-128-gcm", "password": "password", "udp": []any{true},
	}, nil)
	require.Error(t, err)
}
