package adapter

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/metacubex/mihomo/common/atomic"
	"github.com/metacubex/mihomo/common/jsonutil"
	"github.com/metacubex/mihomo/common/queue"
	"github.com/metacubex/mihomo/common/utils"
	"github.com/metacubex/mihomo/common/xsync"
	"github.com/metacubex/mihomo/component/ca"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/log"

	"github.com/metacubex/http"
	"github.com/metacubex/tls"
)

var UnifiedDelay = atomic.NewBool(false)

const (
	defaultHistoriesNum = 10
)

type internalProxyState struct {
	alive   atomic.Bool
	history *queue.Queue[C.DelayHistory]
}

type Proxy struct {
	C.ProxyAdapter
	alive    atomic.Bool
	history  *queue.Queue[C.DelayHistory]
	extra    xsync.Map[string, *internalProxyState]
	hasExtra atomic.Bool
}

// hasExtraState reports whether this proxy carries per-URL test state. A proxy
// that only ever gets read owns no table for it: the map builds its table on
// first use, which for a subscription of a thousand nodes is a table per node
// that never holds anything.
func (p *Proxy) hasExtraState() bool {
	return p.hasExtra.Load()
}

// Adapter implements C.Proxy
func (p *Proxy) Adapter() C.ProxyAdapter {
	return p.ProxyAdapter
}

// AliveForTestUrl implements C.Proxy
func (p *Proxy) AliveForTestUrl(url string) bool {
	if p.hasExtraState() {
		if state, ok := p.extra.Load(url); ok {
			return state.alive.Load()
		}
	}

	return p.alive.Load()
}

func (p *Proxy) testState(url string, alive bool) *internalProxyState {
	// Published before the table is touched, so a reader that sees the flag
	// clear knows nothing has ever been stored and never has to build one.
	p.hasExtra.Store(true)
	if state, ok := p.extra.Load(url); ok {
		return state
	}
	state, _ := p.extra.LoadOrStore(url, &internalProxyState{
		history: queue.NewWithCapacity[C.DelayHistory](defaultHistoriesNum),
		alive:   atomic.NewBool(alive),
	})
	return state
}

// SetAliveForTestUrl records a locally observed dial result for the test URL.
// A URL-specific result must not overwrite the global fallback state used by
// groups that have not initialized their own URL-specific state yet.
func (p *Proxy) SetAliveForTestUrl(url string, alive bool) {
	if url == "" {
		p.alive.Store(alive)
		return
	}

	state := p.testState(url, alive)
	state.alive.Store(alive)
}

// DialContext implements C.ProxyAdapter
func (p *Proxy) DialContext(ctx context.Context, metadata *C.Metadata) (C.Conn, error) {
	conn, err := p.ProxyAdapter.DialContext(ctx, metadata)
	return conn, err
}

// ListenPacketContext implements C.ProxyAdapter
func (p *Proxy) ListenPacketContext(ctx context.Context, metadata *C.Metadata) (C.PacketConn, error) {
	pc, err := p.ProxyAdapter.ListenPacketContext(ctx, metadata)
	return pc, err
}

// DelayHistory implements C.Proxy
func (p *Proxy) DelayHistory() []C.DelayHistory {
	return p.history.Copy()
}

// DelayHistoryForTestUrl implements C.Proxy
func (p *Proxy) DelayHistoryForTestUrl(url string) []C.DelayHistory {
	if p.hasExtraState() {
		if state, ok := p.extra.Load(url); ok {
			return state.history.Copy()
		}
	}
	return nil
}

// ExtraDelayHistories return all delay histories for each test URL
// implements C.Proxy
func (p *Proxy) ExtraDelayHistories() map[string]C.ProxyState {
	histories := map[string]C.ProxyState{}
	if !p.hasExtraState() {
		return histories
	}

	p.extra.Range(func(k string, v *internalProxyState) bool {
		testUrl := k
		state := v

		history := state.history.Copy()

		histories[testUrl] = C.ProxyState{
			Alive:   state.alive.Load(),
			History: history,
		}
		return true
	})
	return histories
}

// LastDelayForTestUrl return last history record of the specified URL. if proxy is not alive, return the max value of uint16.
// implements C.Proxy
func (p *Proxy) LastDelayForTestUrl(url string) (delay uint16) {
	alive := false
	var history C.DelayHistory

	if p.hasExtraState() {
		if state, ok := p.extra.Load(url); ok {
			alive = state.alive.Load()
			history = state.history.Last()
		}
	}

	if !alive || history.Delay == 0 {
		return 0xffff
	}
	return history.Delay
}

// AppendJSON implements C.ProxyAdapter
func (p *Proxy) AppendJSON(dst []byte) []byte {
	dst = p.ProxyAdapter.AppendJSON(dst)
	dst = dst[:len(dst)-1]

	dst = append(dst, `,"alive":`...)
	dst = jsonutil.AppendBool(dst, p.alive.Load())
	dst = append(dst, `,"udp":`...)
	dst = jsonutil.AppendBool(dst, p.SupportUDP())
	dst = append(dst, `,"uot":`...)
	dst = jsonutil.AppendBool(dst, p.SupportUOT())

	dst = append(dst, `,"history":[`...)
	first := true
	p.history.Range(func(item C.DelayHistory) bool {
		if !first {
			dst = append(dst, ',')
		}
		first = false
		dst = append(dst, `{"time":`...)
		dst = append(dst, '"')
		dst = item.Time.AppendFormat(dst, time.RFC3339Nano)
		dst = append(dst, '"')
		dst = append(dst, `,"delay":`...)
		dst = strconv.AppendUint(dst, uint64(item.Delay), 10)
		dst = append(dst, '}')
		return true
	})
	dst = append(dst, ']')

	dst = append(dst, `,"extra":{`...)
	if p.hasExtraState() {
		size := p.extra.Size()
		urls := make([]string, 0, size)
		p.extra.RangeRelaxed(func(url string, _ *internalProxyState) bool {
			urls = append(urls, url)
			return true
		})
		sort.Strings(urls)
		for index, url := range urls {
			if index != 0 {
				dst = append(dst, ',')
			}
			dst = jsonutil.AppendString(dst, url)
			dst = append(dst, ':')
			alive := false
			var history *queue.Queue[C.DelayHistory]
			if state, ok := p.extra.Load(url); ok {
				alive = state.alive.Load()
				history = state.history
			}
			dst = append(dst, `{"alive":`...)
			dst = jsonutil.AppendBool(dst, alive)
			dst = append(dst, `,"history":[`...)
			if history != nil {
				firstHistory := true
				history.Range(func(item C.DelayHistory) bool {
					if !firstHistory {
						dst = append(dst, ',')
					}
					firstHistory = false
					dst = append(dst, `{"time":`...)
					dst = append(dst, '"')
					dst = item.Time.AppendFormat(dst, time.RFC3339Nano)
					dst = append(dst, '"')
					dst = append(dst, `,"delay":`...)
					dst = strconv.AppendUint(dst, uint64(item.Delay), 10)
					dst = append(dst, '}')
					return true
				})
			}
			dst = append(dst, ']')
			dst = append(dst, '}')
		}
	}
	dst = append(dst, '}')

	info := p.ProxyInfo()
	dst = append(dst, `,"xudp":`...)
	dst = jsonutil.AppendBool(dst, info.XUDP)
	dst = append(dst, `,"tfo":`...)
	dst = jsonutil.AppendBool(dst, info.TFO)
	dst = append(dst, `,"mptcp":`...)
	dst = jsonutil.AppendBool(dst, info.MPTCP)
	dst = append(dst, `,"smux":`...)
	dst = jsonutil.AppendBool(dst, info.SMUX)
	dst = append(dst, `,"interface":`...)
	dst = jsonutil.AppendString(dst, info.Interface)
	dst = append(dst, `,"routing-mark":`...)
	dst = strconv.AppendInt(dst, int64(info.RoutingMark), 10)
	dst = append(dst, `,"provider-name":`...)
	dst = jsonutil.AppendString(dst, info.ProviderName)
	dst = append(dst, `,"dialer-proxy":`...)
	dst = jsonutil.AppendString(dst, info.DialerProxy)
	return append(dst, '}')
}

// MarshalJSON implements C.ProxyAdapter
func (p *Proxy) MarshalJSON() ([]byte, error) {
	return p.AppendJSON(make([]byte, 0, 512+jsonutil.StringLen(p.Name()))), nil
}

type urlTestExchange struct {
	conn    net.Conn
	buffers *urlTestBufio
}

type urlTestBufio struct {
	writer *bufio.Writer
	reader *bufio.Reader
}

var urlTestBufioPool = sync.Pool{
	New: func() any {
		return &urlTestBufio{
			writer: bufio.NewWriterSize(nil, 512),
			reader: bufio.NewReaderSize(nil, 4096),
		}
	},
}

func acquireURLTestBufio(conn net.Conn) *urlTestBufio {
	buffers := urlTestBufioPool.Get().(*urlTestBufio)
	buffers.writer.Reset(conn)
	buffers.reader.Reset(conn)
	return buffers
}

func (b *urlTestBufio) release() {
	b.writer.Reset(nil)
	b.reader.Reset(nil)
	urlTestBufioPool.Put(b)
}

func newURLTestExchange(ctx context.Context, conn net.Conn, serverName string, secure bool, tlsConfig *tls.Config) (*urlTestExchange, error) {
	if err := setURLTestDeadline(ctx, conn); err != nil {
		return nil, err
	}
	if secure {
		if tlsConfig.ServerName == "" {
			tlsConfig.ServerName = serverName
		}
		tlsConn := tls.Client(conn, tlsConfig)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return nil, err
		}
		conn = tlsConn
	}
	return &urlTestExchange{conn: conn, buffers: acquireURLTestBufio(conn)}, nil
}

func (e *urlTestExchange) release() {
	e.buffers.release()
	e.conn = nil
}

func (e *urlTestExchange) Do(req *http.Request) (*http.Response, error) {
	if err := setURLTestDeadline(req.Context(), e.conn); err != nil {
		return nil, err
	}
	if err := req.Write(e.buffers.writer); err != nil {
		return nil, err
	}
	if err := e.buffers.writer.Flush(); err != nil {
		return nil, err
	}
	response, err := http.ReadResponse(e.buffers.reader, req)
	if err != nil && response != nil {
		_ = response.Body.Close()
	}
	return response, err
}

func setURLTestDeadline(ctx context.Context, conn net.Conn) error {
	deadline := time.Now().Add(30 * time.Second)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	return conn.SetDeadline(deadline)
}

// URLTest get the delay for the specified URL
// implements C.Proxy
func (p *Proxy) URLTest(ctx context.Context, url string, expectedStatus utils.IntRanges[uint16]) (t uint16, err error) {
	var satisfied bool

	defer func() {
		if UrlTestHook != nil {
			UrlTestHook(url, p.Name(), t, ctx.Err() != nil)
		}

		alive := err == nil
		record := C.DelayHistory{Time: time.Now()}
		if alive {
			record.Delay = t
		}
		if url == "" {
			p.alive.Store(alive)
		}

		p.history.PutBounded(defaultHistoriesNum, record)
		if p.history.Len() > defaultHistoriesNum {
			p.history.Pop()
		}

		state := p.testState(url, true)

		if !satisfied {
			record.Delay = 0
			alive = false
		}

		state.alive.Store(alive)
		state.history.PutBounded(defaultHistoriesNum, record)
		if state.history.Len() > defaultHistoriesNum {
			state.history.Pop()
		}

	}()

	unifiedDelay := UnifiedDelay.Load()

	addr, err := urlToMetadata(url)
	if err != nil {
		return
	}

	start := time.Now()
	instance, err := p.DialContext(ctx, &addr)
	if err != nil {
		return
	}
	defer func() {
		_ = instance.Close()
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return
	}

	secure := req.URL.Scheme == "https"
	var tlsConfig *tls.Config
	if secure {
		tlsConfig, err = ca.GetTLSConfig(ca.Option{})
		if err != nil {
			return
		}
	}

	exchange, err := newURLTestExchange(ctx, instance, req.URL.Hostname(), req.URL.Scheme == "https", tlsConfig)
	if err != nil {
		return
	}
	defer exchange.release()
	resp, err := exchange.Do(req)

	if err != nil {
		return
	}

	_ = resp.Body.Close()

	if unifiedDelay {
		second := time.Now()
		var ignoredErr error
		var secondResp *http.Response
		secondResp, ignoredErr = exchange.Do(req)
		if ignoredErr == nil {
			resp = secondResp
			_ = resp.Body.Close()
			start = second
		} else {
			if strings.HasPrefix(url, "http://") {
				log.Errorln("%s failed to get the second response from %s: %v", p.Name(), url, ignoredErr)
				log.Warnln("It is recommended to use HTTPS for provider.health-check.url and group.url to ensure better reliability. Due to some proxy providers hijacking test addresses and not being compatible with repeated HEAD requests, using HTTP may result in failed tests.")
			}
		}
	}

	satisfied = resp != nil && (expectedStatus == nil || expectedStatus.Check(uint16(resp.StatusCode)))
	t = uint16(time.Since(start) / time.Millisecond)
	if t == 0 {
		// a zero delay means a failed test to callers, so report a test
		// that completed in under a millisecond as 1 ms
		t = 1
	}
	return
}

func NewProxy(adapter C.ProxyAdapter) *Proxy {
	return &Proxy{
		ProxyAdapter: adapter,
		history:      queue.NewWithCapacity[C.DelayHistory](defaultHistoriesNum),
		alive:        atomic.NewBool(true),
	}
}

func urlToMetadata(rawURL string) (addr C.Metadata, err error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return
	}

	port := u.Port()
	if port == "" {
		switch u.Scheme {
		case "https":
			port = "443"
		case "http":
			port = "80"
		default:
			err = fmt.Errorf("%s scheme not Support", rawURL)
			return
		}
	}

	err = addr.SetRemoteAddress(net.JoinHostPort(u.Hostname(), port))
	return
}
