package route

import (
	"strconv"

	"github.com/metacubex/mihomo/common/jsonutil"
	"github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/tunnel"

	"github.com/metacubex/chi"
	"github.com/metacubex/chi/render"
	"github.com/metacubex/http"
)

func ruleRouter() http.Handler {
	r := chi.NewRouter()
	r.Get("/", getRules)
	if !embedMode { // disallow update/patch rules in embed mode
		r.Patch("/disable", disableRules)
	}
	return r
}

func appendRulesDocument(dst []byte, rawRules []constant.Rule) []byte {
	dst = append(dst, `{"rules":[`...)
	for index, wrappedRule := range rawRules {
		if index != 0 {
			dst = append(dst, ',')
		}
		rule := wrappedRule
		dst = append(dst, `{"index":`...)
		dst = strconv.AppendInt(dst, int64(index), 10)
		dst = append(dst, `,"type":`...)
		dst = jsonutil.AppendString(dst, rule.RuleType().String())
		dst = append(dst, `,"payload":`...)
		dst = jsonutil.AppendString(dst, rule.Payload())
		dst = append(dst, `,"proxy":`...)
		dst = jsonutil.AppendString(dst, rule.Adapter())
		dst = append(dst, `,"size":-1`...)

		if ruleWrapper, ok := rule.(constant.RuleWrapper); ok {
			dst = append(dst, `,"extra":{"disabled":`...)
			dst = jsonutil.AppendBool(dst, ruleWrapper.IsDisabled())
			dst = append(dst, `,"hitCount":`...)
			dst = strconv.AppendUint(dst, ruleWrapper.HitCount(), 10)
			dst = append(dst, `,"hitAt":`...)
			dst = jsonutil.AppendTime(dst, ruleWrapper.HitAt())
			dst = append(dst, `,"missCount":`...)
			dst = strconv.AppendUint(dst, ruleWrapper.MissCount(), 10)
			dst = append(dst, `,"missAt":`...)
			dst = jsonutil.AppendTime(dst, ruleWrapper.MissAt())
			dst = append(dst, '}')
			rule = ruleWrapper.Unwrap()
		}

		if rule.RuleType() == constant.GEOIP || rule.RuleType() == constant.GEOSITE {
			dst = append(dst, `,"size":`...)
			dst = strconv.AppendInt(dst, int64(rule.(constant.RuleGroup).GetRecodeSize()), 10)
		}
		dst = append(dst, '}')
	}
	return append(dst, ']', '}')
}

func getRules(w http.ResponseWriter, r *http.Request) {
	bodyPtr := responseBodyPool.Get().(*[]byte)
	body := appendRulesDocument((*bodyPtr)[:0], tunnel.Rules())
	writeJSONDocument(w, r, body)
	*bodyPtr = body
	responseBodyPool.Put(bodyPtr)
}

// disableRules disable or enable rules by their indexes.
func disableRules(w http.ResponseWriter, r *http.Request) {
	// key: rule index, value: disabled
	var payload map[int]bool
	if err := render.DecodeJSON(r.Body, &payload); err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, ErrBadRequest)
		return
	}

	if len(payload) != 0 {
		rules := tunnel.Rules()
		for index, disabled := range payload {
			if index < 0 || index >= len(rules) {
				continue
			}
			rule := rules[index]
			if ruleWrapper, ok := rule.(constant.RuleWrapper); ok {
				ruleWrapper.SetDisabled(disabled)
			}
		}
	}

	render.NoContent(w, r)
}
