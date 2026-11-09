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
	// AnswerVersion counts every move the group's answer names: its pinned
	// selection and the pick it answers with. Both can move without the caller
	// asking, so a caller that caches the answer reads this one counter instead
	// of resolving either. The members the answer names ride the provider list
	// generation, which such a caller already reads. A group whose answer names
	// neither reports zero.
	AnswerVersion() uint64
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
