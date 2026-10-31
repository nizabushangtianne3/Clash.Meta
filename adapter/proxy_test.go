package adapter

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"net"
	stdhttp "net/http"
	"net/http/httptest"
	"testing"

	"github.com/metacubex/mihomo/adapter/outbound"
	"github.com/metacubex/mihomo/common/utils"
	"github.com/metacubex/mihomo/component/ca"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testStatusServer(tb testing.TB, status int) *httptest.Server {
	tb.Helper()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		w.WriteHeader(status)
	}))
	tb.Cleanup(server.Close)
	return server
}

func testProxy() *Proxy {
	return NewProxy(outbound.NewDirectWithOption(outbound.DirectOption{Name: "test"}))
}

// The delay a proxy reports is what the proxies page shows, and the direct
// dialer makes the round trip real without leaving the machine.
func TestProxyURLTestRecordsDelayForTheTestedUrl(t *testing.T) {
	server := testStatusServer(t, stdhttp.StatusNoContent)
	proxy := testProxy()

	delay, err := proxy.URLTest(context.Background(), server.URL, nil)
	require.NoError(t, err)

	assert.True(t, proxy.AliveForTestUrl(server.URL))
	history := proxy.DelayHistoryForTestUrl(server.URL)
	require.Len(t, history, 1)
	assert.Equal(t, delay, history[0].Delay)

	if delay > 0 {
		assert.Equal(t, delay, proxy.LastDelayForTestUrl(server.URL))
	} else {
		// a record of zero delay is not a usable delay
		assert.Equal(t, uint16(0xffff), proxy.LastDelayForTestUrl(server.URL))
	}
}

// Each test URL keeps its own record, so a provider's health check and a
// group's test do not overwrite one another.
func TestProxyURLTestKeepsEachUrlApart(t *testing.T) {
	first := testStatusServer(t, stdhttp.StatusNoContent)
	second := testStatusServer(t, stdhttp.StatusNoContent)
	proxy := testProxy()

	_, err := proxy.URLTest(context.Background(), first.URL, nil)
	require.NoError(t, err)
	_, err = proxy.URLTest(context.Background(), second.URL, nil)
	require.NoError(t, err)

	assert.Len(t, proxy.DelayHistoryForTestUrl(first.URL), 1)
	assert.Len(t, proxy.DelayHistoryForTestUrl(second.URL), 1)
	assert.True(t, proxy.AliveForTestUrl(first.URL))
	assert.True(t, proxy.AliveForTestUrl(second.URL))

	states := proxy.ExtraDelayHistories()
	assert.Len(t, states, 2)

	data, err := proxy.MarshalJSON()
	require.NoError(t, err)
	var view struct {
		Extra map[string]struct {
			Alive   bool `json:"alive"`
			History []struct {
				Delay uint16 `json:"delay"`
			} `json:"history"`
		} `json:"extra"`
	}
	require.NoError(t, json.Unmarshal(data, &view))
	require.Len(t, view.Extra, 2)
	require.True(t, view.Extra[first.URL].Alive)
	require.True(t, view.Extra[second.URL].Alive)
	require.Len(t, view.Extra[first.URL].History, 1)
	require.Len(t, view.Extra[second.URL].History, 1)
}

func TestProxyURLTestRejectsUnexpectedStatus(t *testing.T) {
	server := testStatusServer(t, stdhttp.StatusInternalServerError)
	proxy := testProxy()

	_, err := proxy.URLTest(context.Background(), server.URL, utils.IntRanges[uint16]{utils.NewRange[uint16](200, 299)})
	require.NoError(t, err)

	assert.False(t, proxy.AliveForTestUrl(server.URL))
	assert.Equal(t, uint16(0xffff), proxy.LastDelayForTestUrl(server.URL))
	history := proxy.DelayHistoryForTestUrl(server.URL)
	require.Len(t, history, 1)
	assert.Zero(t, history[0].Delay)
}

func TestProxyURLTestReportsAFailedDial(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	url := "http://" + listener.Addr().String() + "/"
	require.NoError(t, listener.Close())

	proxy := testProxy()
	_, err = proxy.URLTest(context.Background(), url, nil)
	require.Error(t, err)

	assert.False(t, proxy.AliveForTestUrl(url))
	assert.Equal(t, uint16(0xffff), proxy.LastDelayForTestUrl(url))
}

func TestProxyAliveForTestUrl(t *testing.T) {
	proxy := testProxy()

	assert.True(t, proxy.AliveForTestUrl(""))

	proxy.SetAliveForTestUrl("http://a/", false)
	assert.False(t, proxy.AliveForTestUrl("http://a/"))
	assert.True(t, proxy.AliveForTestUrl(""), "a url state must not touch the global one")

	proxy.SetAliveForTestUrl("", false)
	assert.False(t, proxy.AliveForTestUrl(""))
	assert.False(t, proxy.AliveForTestUrl("http://b/"), "an unseen url falls back to the global state")
}

func TestProxyMarshalJSONCarriesDelayState(t *testing.T) {
	proxy := testProxy()
	data, err := proxy.MarshalJSON()
	require.NoError(t, err)

	var mapping map[string]any
	require.NoError(t, json.Unmarshal(data, &mapping))
	assert.Equal(t, "test", mapping["name"])
	assert.Equal(t, true, mapping["alive"])
	assert.Contains(t, mapping, "history")
	assert.Contains(t, mapping, "extra")
}

func BenchmarkProxyMarshalJSON(b *testing.B) {
	proxy := testProxy()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := proxy.MarshalJSON(); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkProxyURLTestHTTP(b *testing.B) {
	server := testStatusServer(b, stdhttp.StatusNoContent)
	proxy := testProxy()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := proxy.URLTest(ctx, server.URL, nil); err != nil {
			b.Fatal(err)
		}
	}
}

func TestProxyURLTestUsesTLSAndSendsTheUnifiedDelaySecondRequest(t *testing.T) {
	requests := make(chan *stdhttp.Request, 2)
	server := httptest.NewTLSServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		requests <- r
		w.WriteHeader(stdhttp.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	certificate := server.Certificate()
	require.NotNil(t, certificate)
	certificatePEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw})
	require.NoError(t, ca.AddCertificate(string(certificatePEM)))
	t.Cleanup(ca.ResetCertificate)

	previousUnifiedDelay := UnifiedDelay.Load()
	UnifiedDelay.Store(true)
	t.Cleanup(func() { UnifiedDelay.Store(previousUnifiedDelay) })

	proxy := testProxy()
	_, err := proxy.URLTest(context.Background(), server.URL, nil)
	require.NoError(t, err)
	assert.True(t, proxy.AliveForTestUrl(server.URL))
	assert.Len(t, requests, 2)
}
