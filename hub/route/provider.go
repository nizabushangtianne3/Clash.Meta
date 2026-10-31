package route

import (
	"context"
	"sort"

	"github.com/metacubex/mihomo/common/jsonutil"
	C "github.com/metacubex/mihomo/constant"
	P "github.com/metacubex/mihomo/constant/provider"
	"github.com/metacubex/mihomo/tunnel"

	"github.com/metacubex/chi"
	"github.com/metacubex/chi/render"
	"github.com/metacubex/http"
	"github.com/samber/lo"
)

func proxyProviderRouter() http.Handler {
	r := chi.NewRouter()
	r.Get("/", getProviders)

	r.Route("/{providerName}", func(r chi.Router) {
		r.Use(parseProviderName, findProviderByName)
		r.Get("/", getProvider)
		r.Put("/", updateProvider)
		r.Get("/healthcheck", healthCheckProvider)
		r.Mount("/", proxyProviderProxyRouter())
	})
	return r
}

func proxyProviderProxyRouter() http.Handler {
	r := chi.NewRouter()
	r.Route("/{name}", func(r chi.Router) {
		r.Use(parseProxyName, findProviderProxyByName)
		r.Get("/", getProxy)
		r.Get("/healthcheck", getProxyDelay)
	})
	return r
}

func appendProxyProvidersDocument(dst []byte, providers map[string]P.ProxyProvider) []byte {
	namesPtr := proxyNamesPool.Get().(*[]string)
	names := (*namesPtr)[:0]
	for name := range providers {
		names = append(names, name)
	}
	sort.Strings(names)

	dst = append(dst, `{"providers":{`...)
	for index, name := range names {
		if index != 0 {
			dst = append(dst, ',')
		}
		dst = jsonutil.AppendString(dst, name)
		dst = append(dst, ':')
		dst = providers[name].AppendJSON(dst)
	}
	dst = append(dst, '}', '}')

	*namesPtr = names
	proxyNamesPool.Put(namesPtr)
	return dst
}

func getProviders(w http.ResponseWriter, r *http.Request) {
	bodyPtr := responseBodyPool.Get().(*[]byte)
	body := appendProxyProvidersDocument((*bodyPtr)[:0], tunnel.Providers())
	writeJSONDocument(w, r, body)
	*bodyPtr = body
	responseBodyPool.Put(bodyPtr)
}

func getProvider(w http.ResponseWriter, r *http.Request) {
	provider := r.Context().Value(CtxKeyProvider).(P.ProxyProvider)
	bodyPtr := responseBodyPool.Get().(*[]byte)
	body := provider.AppendJSON((*bodyPtr)[:0])
	writeJSONDocument(w, r, body)
	*bodyPtr = body
	responseBodyPool.Put(bodyPtr)
}

func updateProvider(w http.ResponseWriter, r *http.Request) {
	provider := r.Context().Value(CtxKeyProvider).(P.ProxyProvider)
	if err := provider.Update(); err != nil {
		render.Status(r, http.StatusServiceUnavailable)
		render.JSON(w, r, newError(err.Error()))
		return
	}
	render.NoContent(w, r)
}

func healthCheckProvider(w http.ResponseWriter, r *http.Request) {
	provider := r.Context().Value(CtxKeyProvider).(P.ProxyProvider)
	provider.HealthCheck()
	render.NoContent(w, r)
}

func parseProviderName(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := getEscapeParam(r, "providerName")
		ctx := context.WithValue(r.Context(), CtxKeyProviderName, name)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func findProviderByName(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := r.Context().Value(CtxKeyProviderName).(string)
		providers := tunnel.Providers()
		provider, exist := providers[name]
		if !exist {
			render.Status(r, http.StatusNotFound)
			render.JSON(w, r, ErrNotFound)
			return
		}

		ctx := context.WithValue(r.Context(), CtxKeyProvider, provider)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func findProviderProxyByName(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var (
			name = r.Context().Value(CtxKeyProxyName).(string)
			pd   = r.Context().Value(CtxKeyProvider).(P.ProxyProvider)
		)
		proxy, exist := lo.Find(pd.Proxies(), func(proxy C.Proxy) bool {
			return proxy.Name() == name
		})

		if !exist {
			render.Status(r, http.StatusNotFound)
			render.JSON(w, r, ErrNotFound)
			return
		}

		ctx := context.WithValue(r.Context(), CtxKeyProxy, proxy)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func ruleProviderRouter() http.Handler {
	r := chi.NewRouter()
	r.Get("/", getRuleProviders)
	r.Route("/{name}", func(r chi.Router) {
		r.Use(parseRuleProviderName, findRuleProviderByName)
		r.Put("/", updateRuleProvider)
	})
	return r
}

func appendRuleProvidersDocument(dst []byte, providers map[string]P.RuleProvider) []byte {
	namesPtr := proxyNamesPool.Get().(*[]string)
	names := (*namesPtr)[:0]
	for name := range providers {
		names = append(names, name)
	}
	sort.Strings(names)

	dst = append(dst, `{"providers":{`...)
	for index, name := range names {
		if index != 0 {
			dst = append(dst, ',')
		}
		dst = jsonutil.AppendString(dst, name)
		dst = append(dst, ':')
		dst = providers[name].AppendJSON(dst)
	}
	dst = append(dst, '}', '}')

	*namesPtr = names
	proxyNamesPool.Put(namesPtr)
	return dst
}

func getRuleProviders(w http.ResponseWriter, r *http.Request) {
	bodyPtr := responseBodyPool.Get().(*[]byte)
	body := appendRuleProvidersDocument((*bodyPtr)[:0], tunnel.RuleProviders())
	writeJSONDocument(w, r, body)
	*bodyPtr = body
	responseBodyPool.Put(bodyPtr)
}

func updateRuleProvider(w http.ResponseWriter, r *http.Request) {
	provider := r.Context().Value(CtxKeyProvider).(P.RuleProvider)
	if err := provider.Update(); err != nil {
		render.Status(r, http.StatusServiceUnavailable)
		render.JSON(w, r, newError(err.Error()))
		return
	}
	render.NoContent(w, r)
}

func parseRuleProviderName(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := getEscapeParam(r, "name")
		ctx := context.WithValue(r.Context(), CtxKeyProviderName, name)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func findRuleProviderByName(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := r.Context().Value(CtxKeyProviderName).(string)
		providers := tunnel.RuleProviders()
		provider, exist := providers[name]
		if !exist {
			render.Status(r, http.StatusNotFound)
			render.JSON(w, r, ErrNotFound)
			return
		}

		ctx := context.WithValue(r.Context(), CtxKeyProvider, provider)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
