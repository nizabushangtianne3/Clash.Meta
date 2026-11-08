package adapter

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"net"
	stdhttp "net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/metacubex/mihomo/adapter/outbound"
	"github.com/metacubex/mihomo/common/utils"
	"github.com/metacubex/mihomo/component/ca"
	C "github.com/metacubex/mihomo/constant"
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
	assert.Empty(t, mapping["extra"])
}

// A proxy that is only read owns no map table: the table is built on first
// use, and a subscription of a thousand nodes would build one per node that
// never holds anything. Writing still publishes before it stores, so a reader
// that sees the flag clear knows the table is empty rather than unseen.
func TestProxyExtraStateIsBuiltOnlyByAWrite(t *testing.T) {
	proxy := testProxy()
	require.False(t, proxy.hasExtraState())

	// An unseen url falls back to the global state, which a fresh proxy carries.
	assert.True(t, proxy.AliveForTestUrl("http://a/"))
	assert.Empty(t, proxy.DelayHistoryForTestUrl("http://a/"))
	assert.Equal(t, uint16(0xffff), proxy.LastDelayForTestUrl("http://a/"))
	require.False(t, proxy.extra.Initialized(), "reads must not build the table")
	require.False(t, proxy.hasExtraState())

	proxy.SetAliveForTestUrl("http://a/", true)
	require.True(t, proxy.hasExtraState())
	require.True(t, proxy.extra.Initialized())
	assert.True(t, proxy.AliveForTestUrl("http://a/"))
	assert.True(t, proxy.AliveForTestUrl(""), "a url state must not touch the global one")
	assert.True(t, proxy.AliveForTestUrl("http://unseen/"), "an unseen url falls back to the global state")

	empty := testProxy()
	assert.Empty(t, empty.ExtraDelayHistories())
	require.False(t, empty.extra.Initialized())
	document, err := empty.MarshalJSON()
	require.NoError(t, err)
	assert.Contains(t, string(document), `"extra":{}`)
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

// pipeListener and pipeProxyAdapter keep the URL-test benchmark entirely in
// process. Windows can otherwise allocate an outbound ephemeral port equal to
// the httptest listener port, making benchmark iterations depend on kernel port
// allocation rather than the URL-test implementation.
type pipeListener struct {
	conns chan net.Conn
	done  chan struct{}
	once  sync.Once
	addr  net.Addr
}

func newPipeListener() *pipeListener {
	return &pipeListener{
		conns: make(chan net.Conn, 128),
		done:  make(chan struct{}),
		addr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 80},
	}
}

func (l *pipeListener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.conns:
		return conn, nil
	case <-l.done:
		return nil, net.ErrClosed
	}
}

func (l *pipeListener) enqueue(conn net.Conn) error {
	select {
	case l.conns <- conn:
		return nil
	case <-l.done:
		_ = conn.Close()
		return net.ErrClosed
	}
}

func (l *pipeListener) Close() error {
	l.once.Do(func() { close(l.done) })
	return nil
}

func (*pipeListener) Addr() net.Addr { return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 80} }

type pipeProxyAdapter struct {
	*outbound.Base
	listener *pipeListener
}

func (p *pipeProxyAdapter) DialContext(ctx context.Context, _ *C.Metadata) (C.Conn, error) {
	client, server := net.Pipe()
	if err := p.listener.enqueue(server); err != nil {
		return nil, err
	}
	return outbound.NewConn(client, p), nil
}

func BenchmarkProxyURLTestHTTP(b *testing.B) {
	listener := newPipeListener()
	server := &stdhttp.Server{Handler: stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		w.WriteHeader(stdhttp.StatusNoContent)
	})}
	serverError := make(chan error, 1)
	go func() { serverError <- server.Serve(listener) }()
	b.Cleanup(func() {
		_ = server.Close()
		_ = listener.Close()
		<-serverError
	})

	proxy := NewProxy(&pipeProxyAdapter{
		Base:     outbound.NewBase(outbound.BaseOption{Name: "pipe-bench", Type: C.Direct, Prefer: C.DualStack}),
		listener: listener,
	})
	ctx := context.Background()
	const url = "http://benchmark.test/delay"

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := proxy.URLTest(ctx, url, nil); err != nil {
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

func BenchmarkSetAliveExistingURL(b *testing.B) {
	proxy := testProxy()
	const url = "http://bench.example/"
	proxy.SetAliveForTestUrl(url, true)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		proxy.SetAliveForTestUrl(url, true)
	}
}
