package route

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/metacubex/mihomo/adapter/outboundgroup"
	"github.com/metacubex/mihomo/common/jsonutil"
	"github.com/metacubex/mihomo/common/utils"
	"github.com/metacubex/mihomo/component/profile/cachefile"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/tunnel"

	"github.com/metacubex/chi"
	"github.com/metacubex/chi/render"
	"github.com/metacubex/http"
)

var (
	SwitchProxiesCallback func(sGroup string, sProxy string)

	responseBodyPool = sync.Pool{New: func() any { return new([]byte) }}
	proxyNamesPool   = sync.Pool{New: func() any { return new([]string) }}
)

func writeJSONDocument(w http.ResponseWriter, r *http.Request, document []byte) {
	w.Header().Set("Content-Type", "application/json")
	if status, ok := r.Context().Value(render.StatusCtxKey).(int); ok {
		w.WriteHeader(status)
	}
	_, _ = w.Write(append(document, '\n'))
}

func appendProxyDocument(dst []byte, proxies map[string]C.Proxy) []byte {
	namesPtr := proxyNamesPool.Get().(*[]string)
	names := (*namesPtr)[:0]
	for name := range proxies {
		names = append(names, name)
	}
	sort.Strings(names)

	dst = append(dst, `{"proxies":{`...)
	for index, name := range names {
		if index != 0 {
			dst = append(dst, ',')
		}
		dst = jsonutil.AppendString(dst, name)
		dst = append(dst, ':')
		dst = proxies[name].AppendJSON(dst)
	}
	dst = append(dst, '}', '}')

	*namesPtr = names
	proxyNamesPool.Put(namesPtr)
	return dst
}

func appendGroupDocument(dst []byte, proxies map[string]C.Proxy) []byte {
	namesPtr := proxyNamesPool.Get().(*[]string)
	names := (*namesPtr)[:0]
	for name, proxy := range proxies {
		if _, ok := proxy.Adapter().(outboundgroup.ProxyGroup); ok {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	dst = append(dst, `{"proxies":[`...)
	for index, name := range names {
		if index != 0 {
			dst = append(dst, ',')
		}
		dst = proxies[name].AppendJSON(dst)
	}
	dst = append(dst, ']', '}')

	*namesPtr = names
	proxyNamesPool.Put(namesPtr)
	return dst
}

func proxyRouter() http.Handler {
	r := chi.NewRouter()
	r.Get("/", getProxies)

	r.Route("/{name}", func(r chi.Router) {
		r.Use(parseProxyName, findProxyByName)
		r.Get("/", getProxy)
		r.Get("/delay", getProxyDelay)
		r.Put("/", updateProxy)
		r.Delete("/", unfixedProxy)
	})
	return r
}

func parseProxyName(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := getEscapeParam(r, "name")
		ctx := context.WithValue(r.Context(), CtxKeyProxyName, name)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func findProxyByName(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := r.Context().Value(CtxKeyProxyName).(string)
		proxies := tunnel.Proxies()
		proxy, exist := proxies[name]
		if !exist {
			render.Status(r, http.StatusNotFound)
			render.JSON(w, r, ErrNotFound)
			return
		}

		ctx := context.WithValue(r.Context(), CtxKeyProxy, proxy)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func getProxies(w http.ResponseWriter, r *http.Request) {
	bodyPtr := responseBodyPool.Get().(*[]byte)
	body := appendProxyDocument((*bodyPtr)[:0], tunnel.Proxies())
	writeJSONDocument(w, r, body)
	*bodyPtr = body
	responseBodyPool.Put(bodyPtr)
}

func getProxy(w http.ResponseWriter, r *http.Request) {
	proxy := r.Context().Value(CtxKeyProxy).(C.Proxy)
	bodyPtr := responseBodyPool.Get().(*[]byte)
	body := proxy.AppendJSON((*bodyPtr)[:0])
	writeJSONDocument(w, r, body)
	*bodyPtr = body
	responseBodyPool.Put(bodyPtr)
}

func updateProxy(w http.ResponseWriter, r *http.Request) {
	req := struct {
		Name string `json:"name"`
	}{}
	if err := render.DecodeJSON(r.Body, &req); err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, ErrBadRequest)
		return
	}

	proxy := r.Context().Value(CtxKeyProxy).(C.Proxy)
	selector, ok := proxy.Adapter().(outboundgroup.SelectAble)
	if !ok {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, newError("Must be a Selector"))
		return
	}

	if err := selector.Set(req.Name); err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, newError(fmt.Sprintf("Selector update error: %s", err.Error())))
		return
	}

	cachefile.Cache().SetSelected(proxy.Name(), req.Name)
	if SwitchProxiesCallback != nil {
		// refresh tray menu
		go SwitchProxiesCallback(proxy.Name(), req.Name)
	}
	render.NoContent(w, r)
}

func getProxyDelay(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	url := query.Get("url")
	timeout, err := strconv.ParseInt(query.Get("timeout"), 10, 16)
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, ErrBadRequest)
		return
	}

	expectedStatus, err := utils.NewUnsignedRanges[uint16](query.Get("expected"))
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, ErrBadRequest)
		return
	}

	proxy := r.Context().Value(CtxKeyProxy).(C.Proxy)

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*time.Duration(timeout))
	defer cancel()

	delay, err := proxy.URLTest(ctx, url, expectedStatus)
	if ctx.Err() != nil {
		render.Status(r, http.StatusGatewayTimeout)
		render.JSON(w, r, ErrRequestTimeout)
		return
	}

	if err != nil || delay == 0 {
		render.Status(r, http.StatusServiceUnavailable)
		if err != nil && delay != 0 {
			render.JSON(w, r, err)
		} else {
			render.JSON(w, r, newError("An error occurred in the delay test"))
		}
		return
	}

	render.JSON(w, r, render.M{
		"delay": delay,
	})
}

func unfixedProxy(w http.ResponseWriter, r *http.Request) {
	proxy := r.Context().Value(CtxKeyProxy).(C.Proxy)
	if selectAble, ok := proxy.Adapter().(outboundgroup.SelectAble); ok && proxy.Type() != C.Selector {
		selectAble.ForceSet("")
		cachefile.Cache().SetSelected(proxy.Name(), "")
		render.NoContent(w, r)
		return
	}
	render.Status(r, http.StatusBadRequest)
	render.JSON(w, r, ErrBadRequest)
}
