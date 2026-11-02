package outboundgroup

import (
	"github.com/metacubex/mihomo/common/jsonutil"
	C "github.com/metacubex/mihomo/constant"
)

// These sizes let a group's MarshalJSON hand AppendJSON a buffer the document
// fits exactly, so appending one never grows. A count that misses because a
// field changed between the two passes only costs a reallocation; the bytes
// written stay the ones the write helpers produce.

const (
	jsonLiteralNowField = len(`,"now":`)
	jsonLiteralAllField = len(`,"all":`)
	jsonLiteralEmptyTestURL = len(`,"testUrl":""`)
)

func groupNamesJSONLen(proxies []C.Proxy) int {
	length := 2 // the brackets
	for _, proxy := range proxies {
		length += jsonutil.StringLen(proxy.Name()) + 1 // and the separator
	}
	return length
}

func stringFieldJSONLen(key, value string) int {
	return 1 + jsonutil.StringLen(key) + 1 + jsonutil.StringLen(value)
}

func boolFieldJSONLen(key string, value bool) int {
	length := 1 + jsonutil.StringLen(key) + 1
	if value {
		return length + 4
	}
	return length + 5
}

func selectorJSONLen(s *Selector) int {
	length := len(`{"type":"Selector","name":`) + jsonutil.StringLen(s.Name()) +
		jsonLiteralNowField + jsonutil.StringLen(s.Now()) +
		jsonLiteralAllField + groupNamesJSONLen(s.GetProxies(false))
	if s.testUrl == C.DefaultTestURL {
		length += jsonLiteralEmptyTestURL
	} else {
		length += stringFieldJSONLen("testUrl", s.testUrl)
	}
	return length + boolFieldJSONLen("hidden", s.Hidden()) +
		stringFieldJSONLen("icon", s.Icon()) +
		stringFieldJSONLen("emptyFallback", s.EmptyFallback().Name()) +
		1 // the closing brace
}

func urlTestJSONLen(u *URLTest) int {
	length := len(`{"type":"URLTest","name":`) + jsonutil.StringLen(u.Name()) +
		jsonLiteralNowField + jsonutil.StringLen(u.Now()) +
		jsonLiteralAllField + groupNamesJSONLen(u.GetProxies(false)) +
		stringFieldJSONLen("testUrl", u.testUrl) +
		stringFieldJSONLen("expectedStatus", u.expectedStatus) +
		stringFieldJSONLen("fixed", u.selectedName()) +
		boolFieldJSONLen("hidden", u.Hidden()) +
		stringFieldJSONLen("icon", u.Icon()) +
		stringFieldJSONLen("emptyFallback", u.EmptyFallback().Name()) +
		1 // the closing brace
	return length
}

func fallbackJSONLen(f *Fallback) int {
	length := len(`{"type":"Fallback","name":`) + jsonutil.StringLen(f.Name()) +
		jsonLiteralNowField + jsonutil.StringLen(f.Now()) +
		jsonLiteralAllField + groupNamesJSONLen(f.GetProxies(false)) +
		stringFieldJSONLen("testUrl", f.testUrl) +
		stringFieldJSONLen("expectedStatus", f.expectedStatus) +
		stringFieldJSONLen("fixed", f.selectedName()) +
		boolFieldJSONLen("hidden", f.Hidden()) +
		stringFieldJSONLen("icon", f.Icon()) +
		stringFieldJSONLen("emptyFallback", f.EmptyFallback().Name()) +
		1 // the closing brace
	return length
}

func loadBalanceJSONLen(lb *LoadBalance) int {
	length := len(`{"type":"LoadBalance","name":`) + jsonutil.StringLen(lb.Name()) +
		jsonLiteralAllField + groupNamesJSONLen(lb.GetProxies(false)) +
		stringFieldJSONLen("testUrl", lb.testUrl) +
		stringFieldJSONLen("expectedStatus", lb.expectedStatus) +
		boolFieldJSONLen("hidden", lb.Hidden()) +
		stringFieldJSONLen("icon", lb.Icon()) +
		stringFieldJSONLen("emptyFallback", lb.EmptyFallback().Name()) +
		1 // the closing brace
	return length
}

func writeJSONString(dst []byte, value string) []byte {
	return jsonutil.AppendString(dst, value)
}

func writeGroupNames(dst []byte, proxies []C.Proxy) []byte {
	dst = append(dst, '[')
	for i, proxy := range proxies {
		if i != 0 {
			dst = append(dst, ',')
		}
		dst = jsonutil.AppendString(dst, proxy.Name())
	}
	return append(dst, ']')
}

func writeStringField(dst []byte, key, value string) []byte {
	dst = append(dst, ',')
	dst = jsonutil.AppendString(dst, key)
	dst = append(dst, ':')
	return jsonutil.AppendString(dst, value)
}

func writeBoolField(dst []byte, key string, value bool) []byte {
	dst = append(dst, ',')
	dst = jsonutil.AppendString(dst, key)
	dst = append(dst, ':')
	return jsonutil.AppendBool(dst, value)
}
