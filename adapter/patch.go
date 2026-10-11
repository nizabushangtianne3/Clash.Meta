package adapter

import C "github.com/metacubex/mihomo/constant"

type UrlTestCheck func(url string, proxy C.Proxy, delay uint16)

var UrlTestHook UrlTestCheck
