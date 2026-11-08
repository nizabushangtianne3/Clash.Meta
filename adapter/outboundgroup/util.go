package outboundgroup

import (
	"context"

	"github.com/metacubex/mihomo/common/utils"
	C "github.com/metacubex/mihomo/constant"
	P "github.com/metacubex/mihomo/constant/provider"
)

type ProxyGroup interface {
	C.ProxyAdapter

	Providers() []P.ProxyProvider
	Proxies() []C.Proxy
	Now() string
	// SelectionVersion counts how many times this group's selection moved, so
	// a caller that caches a group's answer can key it on the count beside the
	// provider versions instead of resolving the selection on every read. A
	// group with no selection of its own reports zero.
	SelectionVersion() uint64
	Touch()

	Hidden() bool
	Icon() string

	URLTest(ctx context.Context, url string, expectedStatus utils.IntRanges[uint16]) (mp map[string]uint16, err error)
}

var _ ProxyGroup = (*Fallback)(nil)
var _ ProxyGroup = (*LoadBalance)(nil)
var _ ProxyGroup = (*URLTest)(nil)
var _ ProxyGroup = (*Selector)(nil)

type SelectAble interface {
	Set(string) error
	ForceSet(name string)
}

var _ SelectAble = (*Fallback)(nil)
var _ SelectAble = (*URLTest)(nil)
var _ SelectAble = (*Selector)(nil)
