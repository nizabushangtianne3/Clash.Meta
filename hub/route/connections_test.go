package route

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"

	N "github.com/metacubex/mihomo/common/net"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/tunnel/statistic"
)

type statisticPipeConn struct {
	N.ExtendedConn
}

func (c *statisticPipeConn) Chains() C.Chain         { return nil }
func (c *statisticPipeConn) ProviderChains() C.Chain { return nil }
func (c *statisticPipeConn) AppendToChains(C.ProxyAdapter) {}
func (c *statisticPipeConn) RemoteDestination() string        { return "10.0.0.3:443" }

// The host reads this view on a timer, so the document it receives is pinned
// against the values the tracker carries.
func TestRouterConnectionsView(test *testing.T) {
	local, remote := net.Pipe()
	defer remote.Close()

	metadata := &C.Metadata{
		NetWork: C.TCP,
		Type:    C.HTTP,
		SrcIP:   netip.MustParseAddr("10.0.0.2"),
		SrcPort: 40000,
		DstIP:   netip.MustParseAddr("10.0.0.3"),
		DstPort: 443,
		Host:    "connections.example.com",
	}
	tracker := statistic.NewTCPTracker(
		&statisticPipeConn{ExtendedConn: N.NewExtendedConn(local)},
		statistic.DefaultManager,
		metadata,
		nil,
		7,
		9,
		false,
	)
	statistic.DefaultManager.Join(tracker)
	defer func() {
		statistic.DefaultManager.Leave(tracker)
		_ = tracker.Close()
	}()

	base := newTestServer(test, "")
	request, err := http.NewRequest(http.MethodGet, base+"/connections", nil)
	require.NoError(test, err)
	response, err := http.DefaultClient.Do(request)
	require.NoError(test, err)
	body, err := io.ReadAll(response.Body)
	require.NoError(test, err)
	_ = response.Body.Close()

	require.Equal(test, http.StatusOK, response.StatusCode)
	require.Equal(test, "application/json", response.Header.Get("Content-Type"))

	var document struct {
		DownloadTotal int64 `json:"downloadTotal"`
		UploadTotal   int64 `json:"uploadTotal"`
		Memory        uint64 `json:"memory"`
		Connections []struct {
			ID             string `json:"id"`
			Upload         int64  `json:"upload"`
			Download       int64  `json:"download"`
			Start          string `json:"start"`
			Chains         []string `json:"chains"`
			ProviderChains []string `json:"providerChains"`
			Rule           string   `json:"rule"`
			RulePayload    string   `json:"rulePayload"`
			Metadata struct {
				Network          string `json:"network"`
				Type             string `json:"type"`
				SourceIP         string `json:"sourceIP"`
				SourcePort       string `json:"sourcePort"`
				DestinationIP    string `json:"destinationIP"`
				DestinationPort  string `json:"destinationPort"`
				Host             string `json:"host"`
				RemoteDestination string `json:"remoteDestination"`
			} `json:"metadata"`
		} `json:"connections"`
	}
	require.NoError(test, json.Unmarshal(body, &document))

	matching := 0
	for _, connection := range document.Connections {
		if connection.ID != tracker.ID() {
			continue
		}
		matching++
		require.Equal(test, int64(7), connection.Upload)
		require.Equal(test, int64(9), connection.Download)
		require.NotEmpty(test, connection.Start)
		require.Equal(test, "tcp", connection.Metadata.Network)
		require.Equal(test, "HTTP", connection.Metadata.Type)
		require.Equal(test, "10.0.0.2", connection.Metadata.SourceIP)
		require.Equal(test, "40000", connection.Metadata.SourcePort)
		require.Equal(test, "10.0.0.3", connection.Metadata.DestinationIP)
		require.Equal(test, "443", connection.Metadata.DestinationPort)
		require.Equal(test, "connections.example.com", connection.Metadata.Host)
		require.Equal(test, "10.0.0.3:443", connection.Metadata.RemoteDestination)
	}
	require.Equal(test, 1, matching)
}
