package route

import (
	"context"
	"crypto/subtle"
	"net"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/metacubex/mihomo/adapter/inbound"
	"github.com/metacubex/mihomo/common/utils"
	"github.com/metacubex/mihomo/component/ca"
	"github.com/metacubex/mihomo/component/ech"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/log"
	"github.com/metacubex/mihomo/ntp"
	"github.com/metacubex/mihomo/tunnel/statistic"

	"github.com/metacubex/chi"
	"github.com/metacubex/chi/cors"
	"github.com/metacubex/chi/middleware"
	"github.com/metacubex/chi/render"
	"github.com/metacubex/http"
	"github.com/metacubex/tls"
)

var (
	uiPath = ""

	httpServerMu sync.Mutex
	httpServer   *http.Server
	tlsServer    *http.Server
	unixServer   *http.Server
	pipeServer   *http.Server

	embedMode = false
)

func SetEmbedMode(embed bool) {
	embedMode = embed
}

type Traffic struct {
	Up        int64 `json:"up"`
	Down      int64 `json:"down"`
	UpTotal   int64 `json:"upTotal"`
	DownTotal int64 `json:"downTotal"`
}

type Memory struct {
	Inuse   uint64 `json:"inuse"`
	OSLimit uint64 `json:"oslimit"` // maybe we need it in the future
}

type Config struct {
	Addr           string
	TLSAddr        string
	UnixAddr       string
	PipeAddr       string
	RoutingMark    int
	Secret         string
	Certificate    string
	PrivateKey     string
	ClientAuthType string
	ClientAuthCert string
	EchKey         string
	DohServer      string
	IsDebug        bool
	Cors           Cors
}

type Cors struct {
	AllowOrigins        []string
	AllowPrivateNetwork bool
}

func (c Cors) Apply(r chi.Router) {
	r.Use(cors.New(cors.Options{
		AllowedOrigins:      c.AllowOrigins,
		AllowedMethods:      []string{"GET", "POST", "PUT", "PATCH", "DELETE"},
		AllowedHeaders:      []string{"Content-Type", "Authorization"},
		AllowPrivateNetwork: c.AllowPrivateNetwork,
		MaxAge:              300,
	}).Handler)
}

func ReCreateServer(cfg *Config) {
	go start(cfg)
	go startTLS(cfg)
	go startUnix(cfg)
	if inbound.SupportNamedPipe {
		go startPipe(cfg)
	}
}

func SetUIPath(path string) {
	uiPath = C.Path.Resolve(path)
}

func router(isDebug bool, secret string, dohServer string, cors Cors) *chi.Mux {
	r := chi.NewRouter()
	cors.Apply(r)
	if isDebug {
		r.Mount("/debug", func() http.Handler {
			r := chi.NewRouter()
			r.Put("/gc", func(w http.ResponseWriter, r *http.Request) {
				debug.FreeOSMemory()
			})
			handler := middleware.Profiler
			r.Mount("/", handler())
			return r
		}())
	}
	r.Group(func(r chi.Router) {
		if secret != "" {
			r.Use(authentication(secret))
		}
		r.Get("/", hello)
		r.Get("/logs", getLogs)
		r.Get("/traffic", traffic)
		r.Get("/memory", memory)
		r.Get("/version", version)
		r.Mount("/configs", configRouter())
		r.Mount("/proxies", proxyRouter())
		r.Mount("/group", groupRouter())
		r.Mount("/rules", ruleRouter())
		r.Mount("/connections", connectionRouter())
		r.Mount("/providers/proxies", proxyProviderRouter())
		r.Mount("/providers/rules", ruleProviderRouter())
		r.Mount("/cache", cacheRouter())
		r.Mount("/dns", dnsRouter())
		r.Mount("/storage", storageRouter())
		if !embedMode { // disallow restart in embed mode
			r.Mount("/restart", restartRouter())
		}
		r.Mount("/upgrade", upgradeRouter())
		addExternalRouters(r)

	})

	if uiPath != "" {
		r.Group(func(r chi.Router) {
			fs := http.StripPrefix("/ui", http.FileServer(http.Dir(uiPath)))
			r.Get("/ui", http.RedirectHandler("/ui/", http.StatusTemporaryRedirect).ServeHTTP)
			r.Get("/ui/*", func(w http.ResponseWriter, r *http.Request) {
				fs.ServeHTTP(w, r)
			})
		})
	}
	if len(dohServer) > 0 && dohServer[0] == '/' {
		r.Mount(dohServer, dohRouter())
	}

	return r
}

func start(cfg *Config) {
	httpServerMu.Lock()
	defer httpServerMu.Unlock()

	if httpServer != nil {
		_ = httpServer.Close()
		httpServer = nil
	}

	if len(cfg.Addr) == 0 {
		return
	}

	lc := inbound.NewListenConfig()
	lc.SetRouteMark(cfg.RoutingMark)
	l, err := lc.Listen(context.Background(), "tcp", cfg.Addr)
	if err != nil {
		log.Errorln("External controller listen error: %s", err)
		return
	}
	log.Infoln("RESTful API listening at: %s", l.Addr().String())

	server := &http.Server{
		Handler: router(cfg.IsDebug, cfg.Secret, cfg.DohServer, cfg.Cors),
	}
	httpServer = server
	go func() { _ = server.Serve(l) }()
}

func startTLS(cfg *Config) {
	// first stop existing server
	if tlsServer != nil {
		_ = tlsServer.Close()
		tlsServer = nil
	}

	// handle tlsAddr
	if len(cfg.TLSAddr) > 0 {
		certLoader, err := ca.NewTLSKeyPairLoader(cfg.Certificate, cfg.PrivateKey)
		if err != nil {
			log.Errorln("External controller tls listen error: %s", err)
			return
		}

		lc := inbound.NewListenConfig()
		lc.SetRouteMark(cfg.RoutingMark)
		l, err := lc.Listen(context.Background(), "tcp", cfg.TLSAddr)
		if err != nil {
			log.Errorln("External controller tls listen error: %s", err)
			return
		}

		log.Infoln("RESTful API tls listening at: %s", l.Addr().String())
		tlsConfig := &tls.Config{Time: ntp.Now}
		tlsConfig.NextProtos = []string{"h2", "http/1.1"}
		tlsConfig.GetCertificate = func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
			return certLoader()
		}
		tlsConfig.ClientAuth = ca.ClientAuthTypeFromString(cfg.ClientAuthType)
		if len(cfg.ClientAuthCert) > 0 {
			if tlsConfig.ClientAuth == tls.NoClientCert {
				tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
			}
		}
		if tlsConfig.ClientAuth == tls.VerifyClientCertIfGiven || tlsConfig.ClientAuth == tls.RequireAndVerifyClientCert {
			pool, err := ca.LoadCertificates(cfg.ClientAuthCert)
			if err != nil {
				log.Errorln("External controller tls listen error: %s", err)
				return
			}
			tlsConfig.ClientCAs = pool
		}

		if cfg.EchKey != "" {
			err = ech.LoadECHKey(cfg.EchKey, tlsConfig)
			if err != nil {
				log.Errorln("External controller tls serve error: %s", err)
				return
			}
		}
		server := &http.Server{
			Handler: router(cfg.IsDebug, cfg.Secret, cfg.DohServer, cfg.Cors),
		}
		tlsServer = server
		if err = server.Serve(tls.NewListener(l, tlsConfig)); err != nil {
			log.Errorln("External controller tls serve error: %s", err)
		}
	}
}

func startUnix(cfg *Config) {
	// first stop existing server
	if unixServer != nil {
		_ = unixServer.Close()
		unixServer = nil
	}

	// handle addr
	if len(cfg.UnixAddr) > 0 {
		addr := C.Path.Resolve(cfg.UnixAddr)

		dir := filepath.Dir(addr)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				log.Errorln("External controller unix listen error: %s", err)
				return
			}
		}

		// https://devblogs.microsoft.com/commandline/af_unix-comes-to-windows/
		//
		// Note: As mentioned above in the ‘security’ section, when a socket binds a socket to a valid pathname address,
		// a socket file is created within the filesystem. On Linux, the application is expected to unlink
		// (see the notes section in the man page for AF_UNIX) before any other socket can be bound to the same address.
		// The same applies to Windows unix sockets, except that, DeleteFile (or any other file delete API)
		// should be used to delete the socket file prior to calling bind with the same path.
		_ = syscall.Unlink(addr)

		lc := inbound.NewListenConfig()
		lc.SetRouteMark(0) // don't set route mark for unix socket
		l, err := lc.Listen(context.Background(), "unix", addr)
		if err != nil {
			log.Errorln("External controller unix listen error: %s", err)
			return
		}
		_ = os.Chmod(addr, 0o666)
		log.Infoln("RESTful API unix listening at: %s", l.Addr().String())

		server := &http.Server{
			Handler: router(cfg.IsDebug, "", cfg.DohServer, cfg.Cors),
		}
		unixServer = server
		if err = server.Serve(l); err != nil {
			log.Errorln("External controller unix serve error: %s", err)
		}
	}
}

func startPipe(cfg *Config) {
	// first stop existing server
	if pipeServer != nil {
		_ = pipeServer.Close()
		pipeServer = nil
	}

	// handle addr
	if len(cfg.PipeAddr) > 0 {
		if !strings.HasPrefix(cfg.PipeAddr, "\\\\.\\pipe\\") { // windows namedpipe must start with "\\.\pipe\"
			log.Errorln("External controller pipe listen error: windows namedpipe must start with \"\\\\.\\pipe\\\"")
			return
		}

		l, err := inbound.ListenNamedPipe(cfg.PipeAddr)
		if err != nil {
			log.Errorln("External controller pipe listen error: %s", err)
			return
		}
		log.Infoln("RESTful API pipe listening at: %s", l.Addr().String())

		server := &http.Server{
			Handler: router(cfg.IsDebug, "", cfg.DohServer, cfg.Cors),
		}
		pipeServer = server
		if err = server.Serve(l); err != nil {
			log.Errorln("External controller pipe serve error: %s", err)
		}
	}
}

func safeEqual(a, b string) bool {
	aBuf := utils.ImmutableBytesFromString(a)
	bBuf := utils.ImmutableBytesFromString(b)
	return subtle.ConstantTimeCompare(aBuf, bBuf) == 1
}

func authentication(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			// Browser websocket not support custom header
			if r.Header.Get("Upgrade") == "websocket" && r.URL.Query().Get("token") != "" {
				token := r.URL.Query().Get("token")
				if !safeEqual(token, secret) {
					render.Status(r, http.StatusUnauthorized)
					render.JSON(w, r, ErrUnauthorized)
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			header := r.Header.Get("Authorization")
			bearer, token, found := strings.Cut(header, " ")

			hasInvalidHeader := bearer != "Bearer"
			hasInvalidSecret := !found || !safeEqual(token, secret)
			if hasInvalidHeader || hasInvalidSecret {
				render.Status(r, http.StatusUnauthorized)
				render.JSON(w, r, ErrUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		}
		return http.HandlerFunc(fn)
	}
}

func hello(w http.ResponseWriter, r *http.Request) {
	render.JSON(w, r, render.M{"hello": "mihomo"})
}

func traffic(w http.ResponseWriter, r *http.Request) {
	var wsConn net.Conn
	if r.Header.Get("Upgrade") == "websocket" {
		var err error
		wsConn, _, err = wsUpgrade(r, w)
		if err != nil {
			return
		}
	}

	if wsConn == nil {
		w.Header().Set("Content-Type", "application/json")
		render.Status(r, http.StatusOK)
	}

	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	t := statistic.DefaultManager
	scratch := make([]byte, 0, 128)
	var err error
	for range tick.C {
		up, down := t.Now()
		upTotal, downTotal := t.Total()
		scratch = appendTrafficJSON(scratch[:0], Traffic{
			Up:        up,
			Down:      down,
			UpTotal:   upTotal,
			DownTotal: downTotal,
		})

		if wsConn == nil {
			_, err = w.Write(scratch)
			w.(http.Flusher).Flush()
		} else {
			err = wsWriteServerText(wsConn, scratch)
		}

		if err != nil {
			break
		}
	}
}

func memory(w http.ResponseWriter, r *http.Request) {
	var wsConn net.Conn
	if r.Header.Get("Upgrade") == "websocket" {
		var err error
		wsConn, _, err = wsUpgrade(r, w)
		if err != nil {
			return
		}
	}

	if wsConn == nil {
		w.Header().Set("Content-Type", "application/json")
		render.Status(r, http.StatusOK)
	}

	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	t := statistic.DefaultManager
	scratch := make([]byte, 0, 128)
	var err error
	first := true
	for range tick.C {
		inuse := t.Memory()
		// make chat.js begin with zero
		// this is shit var,but we need output 0 for first time
		if first {
			inuse = 0
			first = false
		}
		scratch = appendMemoryJSON(scratch[:0], Memory{
			Inuse:   inuse,
			OSLimit: 0,
		})
		if wsConn == nil {
			_, err = w.Write(scratch)
			w.(http.Flusher).Flush()
		} else {
			err = wsWriteServerText(wsConn, scratch)
		}

		if err != nil {
			break
		}
	}
}

func getLogs(w http.ResponseWriter, r *http.Request) {
	levelText := r.URL.Query().Get("level")
	if levelText == "" {
		levelText = "info"
	}

	formatText := r.URL.Query().Get("format")
	isStructured := false
	if formatText == "structured" {
		isStructured = true
	}

	level, ok := log.LogLevelMapping[levelText]
	if !ok {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, ErrBadRequest)
		return
	}

	var wsConn net.Conn
	if r.Header.Get("Upgrade") == "websocket" {
		var err error
		wsConn, _, err = wsUpgrade(r, w)
		if err != nil {
			return
		}
	}

	if wsConn == nil {
		w.Header().Set("Content-Type", "application/json")
		render.Status(r, http.StatusOK)
	}

	ch := make(chan log.Event, 1024)
	sub := log.Subscribe()
	defer log.UnSubscribe(sub)
	scratch := make([]byte, 0, 256)

	go func() {
		for logM := range sub {
			select {
			case ch <- logM:
			default:
			}
		}
		close(ch)
	}()

	for logM := range ch {
		if logM.LogLevel < level {
			continue
		}

		if !isStructured {
			scratch = appendLogJSON(scratch[:0], logM)
		} else {
			scratch = appendLogStructuredJSON(scratch[:0], logM, time.Now())
		}

		var err error
		if wsConn == nil {
			_, err = w.Write(scratch)
			w.(http.Flusher).Flush()
		} else {
			err = wsWriteServerText(wsConn, scratch)
		}

		if err != nil {
			break
		}
	}
}

func version(w http.ResponseWriter, r *http.Request) {
	render.JSON(w, r, render.M{"meta": C.Meta, "version": C.Version})
}

// The poll writers append their documents by hand: the reflection encoder
// built its own state, a fresh scratch and the document on every tick, while
// these documents are read once and dropped. The field set, the order and the
// escaping stay pinned by server_json_test.go against documents encoding/json
// produced.

func appendTrafficJSON(dst []byte, t Traffic) []byte {
	dst = append(dst, `{"up":`...)
	dst = strconv.AppendInt(dst, t.Up, 10)
	dst = append(dst, `,"down":`...)
	dst = strconv.AppendInt(dst, t.Down, 10)
	dst = append(dst, `,"upTotal":`...)
	dst = strconv.AppendInt(dst, t.UpTotal, 10)
	dst = append(dst, `,"downTotal":`...)
	dst = strconv.AppendInt(dst, t.DownTotal, 10)
	return append(dst, '}', '\n')
}

func appendMemoryJSON(dst []byte, m Memory) []byte {
	dst = append(dst, `{"inuse":`...)
	dst = strconv.AppendUint(dst, m.Inuse, 10)
	dst = append(dst, `,"oslimit":`...)
	dst = strconv.AppendUint(dst, m.OSLimit, 10)
	return append(dst, '}', '\n')
}

func appendLogJSON(dst []byte, event log.Event) []byte {
	dst = append(dst, `{"type":`...)
	dst = appendJSONString(dst, event.Type())
	dst = append(dst, `,"payload":`...)
	dst = appendJSONString(dst, event.Payload)
	return append(dst, '}', '\n')
}

func appendLogStructuredJSON(dst []byte, event log.Event, now time.Time) []byte {
	level := event.Type()
	if level == "warning" {
		level = "warn"
	}
	dst = append(dst, `{"time":`...)
	dst = now.AppendFormat(dst, `"`+time.TimeOnly+`"`)
	dst = append(dst, `,"level":`...)
	dst = appendJSONString(dst, level)
	dst = append(dst, `,"message":`...)
	dst = appendJSONString(dst, event.Payload)
	dst = append(dst, `,"fields":[]`...)
	return append(dst, '}', '\n')
}

const jsonHexDigits = "0123456789abcdef"

// appendJSONString quotes and escapes the way encoding/json does with HTML
// escaping on, which is what the reflection writer applied.
func appendJSONString(dst []byte, s string) []byte {
	dst = append(dst, '"')
	start := 0
	for i := 0; i < len(s); {
		if b := s[i]; b < utf8.RuneSelf {
			if b >= 0x20 && b != '"' && b != '\\' && b != '<' && b != '>' && b != '&' {
				i++
				continue
			}
			dst = append(dst, s[start:i]...)
			switch b {
			case '\\', '"':
				dst = append(dst, '\\', b)
			case '\b':
				dst = append(dst, '\\', 'b')
			case '\f':
				dst = append(dst, '\\', 'f')
			case '\n':
				dst = append(dst, '\\', 'n')
			case '\r':
				dst = append(dst, '\\', 'r')
			case '\t':
				dst = append(dst, '\\', 't')
			default:
				dst = append(dst, '\\', 'u', '0', '0', jsonHexDigits[b>>4], jsonHexDigits[b&0x0f])
			}
			i++
			start = i
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			dst = append(dst, s[start:i]...)
			dst = utf8.AppendRune(dst, utf8.RuneError)
			i += size
			start = i
			continue
		}
		// json escapes the two unicode separators because they break some
		// hosts that read the document as javascript.
		if r == '\u2028' || r == '\u2029' {
			dst = append(dst, s[start:i]...)
			dst = append(dst, '\\', 'u', '2', '0', '2', jsonHexDigits[r&0x0f])
			i += size
			start = i
			continue
		}
		i += size
	}
	dst = append(dst, s[start:]...)
	return append(dst, '"')
}
