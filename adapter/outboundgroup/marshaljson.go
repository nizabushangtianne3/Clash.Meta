package outboundgroup

import (
	"github.com/metacubex/mihomo/common/jsonutil"
	C "github.com/metacubex/mihomo/constant"
)

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
