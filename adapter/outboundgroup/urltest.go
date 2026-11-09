package outboundgroup

import (
	"context"
	"errors"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/metacubex/mihomo/adapter/provider"
	"github.com/metacubex/mihomo/common/callback"
	N "github.com/metacubex/mihomo/common/net"
	"github.com/metacubex/mihomo/common/singledo"
	"github.com/metacubex/mihomo/common/utils"
	C "github.com/metacubex/mihomo/constant"
	P "github.com/metacubex/mihomo/constant/provider"
)

const unmeasuredPickHold = time.Second

type URLTestOption struct {
	Tolerance uint16 `group:"tolerance,omitempty"`
}

type URLTest struct {
	*GroupBase
	selected           atomic.Pointer[string]
	testUrl            string
	expectedStatus     string
	tolerance          uint16
	disableUDP         bool
	fastMu             sync.Mutex
	fastNode           C.Proxy
	fastSingle         *singledo.Single[C.Proxy]
	fastHasAlternative atomic.Bool
	rotateProbe        atomic.Bool
	probeCursor        atomic.Uint32
	versionsMu         sync.Mutex
	knownGeneration    atomic.Uint64
	answeredPick       atomic.Pointer[answeredPick]
}

func (u *URLTest) Now() string {
	return u.fast(false).Name()
}

func (u *URLTest) Set(name string) error {
	var p C.Proxy
	for _, proxy := range u.GetProxies(false) {
		if proxy.Name() == name {
			p = proxy
			break
		}
	}
	if p == nil {
		return errors.New("proxy not exist")
	}
	u.ForceSet(name)
	return nil
}

func (u *URLTest) selectedName() string {
	if selected := u.selected.Load(); selected != nil {
		return *selected
	}
	return ""
}

// AnswerVersion counts every move this group's answer names: the pinned
// selection, which setSelected writes, and the pick this group answers with,
// which it re-elects on its own. A caller that caches the answer reads this
// one counter instead of resolving either.
func (u *URLTest) AnswerVersion() uint64 {
	return answerVersionOf(&u.answeredPick)
}

func (u *URLTest) setSelected(name string) {
	u.selected.Store(&name)
	bumpAnswer(&u.answeredPick)
}

func (u *URLTest) ForceSet(name string) {
	u.setSelected(name)
	u.resetFastNode()
}

// markProxyFailed stops selecting a proxy after a failed dial, then lets the
// group re-check it soon so a transient failure does not park it until the
// periodic health check.
func (u *URLTest) markProxyFailed(proxy C.Proxy, err error) {
	if !shouldMarkProxyFailed(proxy, err) || !markProxyUnavailable(proxy, u.testUrl) {
		return
	}
	u.resetFastNode()
	u.rotateProbe.Store(true)
	notifyRouteChange()
	u.scheduleRecoveryProbe(proxy)
}

func (u *URLTest) scheduleRecoveryProbe(proxy C.Proxy) {
	expectedStatus, err := utils.NewUnsignedRanges[uint16](u.expectedStatus)
	if err != nil {
		return
	}
	scheduleFailedProxyProbe(
		proxy,
		u.testUrl,
		expectedStatus,
		time.Duration(u.testTimeout)*time.Millisecond,
		func() {
			u.resetFastNode()
			notifyRouteChange()
		},
	)
}

// DialContext implements C.ProxyAdapter
func (u *URLTest) DialContext(ctx context.Context, metadata *C.Metadata) (c C.Conn, err error) {
	proxy := u.fast(true)
	ctx, cancel := failoverContext(ctx, u.fastHasAlternative.Load())
	defer cancel()
	c, err = proxy.DialContext(ctx, metadata)
	if err == nil {
		c.AppendToChains(u)
	} else {
		u.markProxyFailed(proxy, err)
		u.onDialFailed(proxy.Type(), err, u.healthCheck)
	}

	if N.NeedHandshake(c) {
		c = callback.NewFirstWriteCallBackConn(c, func(err error) {
			if err == nil {
				u.onDialSuccess()
			} else {
				u.markProxyFailed(proxy, err)
				u.onDialFailed(proxy.Type(), err, u.healthCheck)
			}
		})
	}

	return c, err
}

// ListenPacketContext implements C.ProxyAdapter
func (u *URLTest) ListenPacketContext(ctx context.Context, metadata *C.Metadata) (C.PacketConn, error) {
	proxy := u.fast(true)
	ctx, cancel := failoverContext(ctx, u.fastHasAlternative.Load())
	defer cancel()
	pc, err := proxy.ListenPacketContext(ctx, metadata)
	if err == nil {
		pc.AppendToChains(u)
	} else {
		u.markProxyFailed(proxy, err)
		u.onDialFailed(proxy.Type(), err, u.healthCheck)
	}

	return pc, err
}

// Unwrap implements C.ProxyAdapter
func (u *URLTest) Unwrap(metadata *C.Metadata, touch bool) C.Proxy {
	return u.fast(touch)
}

func (u *URLTest) healthCheck() {
	u.GroupBase.healthCheck()
	u.resetFastNode()
	notifyRouteChange()
}

func (u *URLTest) shouldReplaceFastNode(fast C.Proxy, fastNotExist bool) bool {
	if u.fastNode == nil || fastNotExist || !u.fastNode.AliveForTestUrl(u.testUrl) {
		return true
	}
	return improvesOnCachedDelay(
		u.fastNode.LastDelayForTestUrl(u.testUrl),
		fast.LastDelayForTestUrl(u.testUrl),
		u.tolerance,
	)
}

func hasOtherAliveProxy(proxies []C.Proxy, selected C.Proxy, testURL string) bool {
	for _, proxy := range proxies {
		if proxy != selected && proxy.AliveForTestUrl(testURL) {
			return true
		}
	}
	return false
}

func (u *URLTest) fast(touch bool) C.Proxy {
	u.dropCacheForProviderSwap()
	// A rebuilt group starts on its first member, which subscriptions often fill
	// with a traffic notice; held for the whole cache window, that pick outlasts
	// the first health check and fails every connection meanwhile. The hold keeps
	// a group with no usable member from walking them all on every connection.
	if cached, ok := u.fastSingle.CachedValid(unmeasuredPickHold, func(pick C.Proxy) bool {
		return pick.LastDelayForTestUrl(u.testUrl) == math.MaxUint16
	}); ok {
		if touch { // the same recency the cached Do result reports as shared
			u.Touch()
		}
		return cached
	}
	elm, _, shared := u.fastSingle.Do(func() (C.Proxy, error) {
		u.fastMu.Lock()
		defer u.fastMu.Unlock()
		proxies := u.GetProxies(touch)
		if len(proxies) == 0 {
			u.fastHasAlternative.Store(false)
			return u.EmptyFallback(), nil
		}

		selectedName := u.selectedName()
		var selectedProxy C.Proxy
		if selectedName != "" {
			if proxy, ok := u.indexedProxyByName(selectedName); ok && proxy.AliveForTestUrl(u.testUrl) {
				u.fastHasAlternative.Store(hasOtherAliveProxy(proxies, proxy, u.testUrl))
				u.rotateProbe.Store(false)
				u.fastNode = proxy
				return proxy, nil
			}
		}
		var fast C.Proxy
		var minDelay uint16
		fastNotExist := true
		aliveCount := int32(0)

		for _, proxy := range proxies {
			if u.fastNode != nil && proxy == u.fastNode {
				fastNotExist = false
			}
			if !proxy.AliveForTestUrl(u.testUrl) {
				continue
			}
			aliveCount++
			if proxy.Name() == selectedName {
				selectedProxy = proxy
			}
			if selectedProxy != nil && aliveCount >= 2 {
				break
			}
			if selectedProxy != nil {
				continue
			}
			delay := proxy.LastDelayForTestUrl(u.testUrl)
			if fast == nil || delay < minDelay {
				fast = proxy
				minDelay = delay
			}
		}
		u.fastHasAlternative.Store(aliveCount > 1)
		if aliveCount > 0 {
			u.rotateProbe.Store(false)
		}

		if selectedProxy != nil {
			u.fastNode = selectedProxy
			return selectedProxy, nil
		}

		if fast == nil {
			index := 0
			if u.rotateProbe.Swap(false) {
				index = int(u.probeCursor.Load()) % len(proxies)
			}
			fast = proxies[index]
			u.probeCursor.Store(uint32((index + 1) % len(proxies)))
		}
		if u.shouldReplaceFastNode(fast, fastNotExist) {
			u.fastNode = fast
		}
		return u.fastNode, nil
	})
	if shared && touch { // a shared fastSingle.Do() may cause providers untouched, so we touch them again
		u.Touch()
	}

	announceAnswer(&u.answeredPick, elm)
	return elm
}

// listGeneration reads the generation the providers' lists carry. A variable so
// a test can stand in for a provider swapping its list, which only the core's
// own providers do.
var listGeneration = provider.ListGeneration

// dropCacheForProviderSwap forgets the cached fast node when a provider has
// replaced its list underneath it: the cached node may no longer be a member,
// and its delay says nothing about the members that took its place.
func (u *URLTest) dropCacheForProviderSwap() {
	generation := listGeneration()
	if u.knownGeneration.Load() == generation {
		return
	}
	u.versionsMu.Lock()
	defer u.versionsMu.Unlock()
	if u.knownGeneration.Load() == generation {
		return
	}
	u.knownGeneration.Store(generation)
	u.resetFastNode()
}

func (u *URLTest) resetFastNode() {
	u.fastMu.Lock()
	defer u.fastMu.Unlock()
	u.fastNode = nil
	u.fastSingle.Reset()
}

// SupportUDP implements C.ProxyAdapter
func (u *URLTest) SupportUDP() bool {
	if u.disableUDP {
		return false
	}
	return u.fast(false).SupportUDP()
}

// IsL3Protocol implements C.ProxyAdapter
func (u *URLTest) IsL3Protocol(metadata *C.Metadata) bool {
	return u.fast(false).IsL3Protocol(metadata)
}

// AppendJSON implements C.ProxyAdapter
func (u *URLTest) AppendJSON(dst []byte) []byte {
	dst = append(dst, `{"type":"URLTest","name":`...)
	dst = writeJSONString(dst, u.Name())
	dst = append(dst, `,"now":`...)
	dst = writeJSONString(dst, u.Now())
	dst = append(dst, `,"all":`...)
	dst = writeGroupNames(dst, u.GetProxies(false))
	dst = writeStringField(dst, "testUrl", u.testUrl)
	dst = writeStringField(dst, "expectedStatus", u.expectedStatus)
	dst = writeStringField(dst, "fixed", u.selectedName())
	dst = writeBoolField(dst, "hidden", u.Hidden())
	dst = writeStringField(dst, "icon", u.Icon())
	dst = writeStringField(dst, "emptyFallback", u.EmptyFallback().Name())
	return append(dst, '}')
}

// MarshalJSON implements C.ProxyAdapter
func (u *URLTest) MarshalJSON() ([]byte, error) {
	return u.AppendJSON(make([]byte, 0, urlTestJSONLen(u))), nil
}

func (u *URLTest) Providers() []P.ProxyProvider {
	return u.providers
}

func (u *URLTest) Proxies() []C.Proxy {
	return u.GetProxies(false)
}

func (u *URLTest) URLTest(ctx context.Context, url string, expectedStatus utils.IntRanges[uint16]) (map[string]uint16, error) {
	return u.GroupBase.URLTest(ctx, u.testUrl, expectedStatus)
}

func NewURLTest(option GroupCommonOption, urlTestOption URLTestOption, emptyFallback C.Proxy, providers []P.ProxyProvider) (*URLTest, error) {
	if emptyFallback == nil {
		return nil, errors.New("empty fallback proxy not exist")
	}
	urlTest := &URLTest{
		GroupBase: NewGroupBase(GroupBaseOption{
			Name:           option.Name,
			Type:           C.URLTest,
			Hidden:         option.Hidden,
			Icon:           option.Icon,
			Filter:         option.Filter,
			ExcludeFilter:  option.ExcludeFilter,
			ExcludeType:    option.ExcludeType,
			TestTimeout:    option.TestTimeout,
			MaxFailedTimes: option.MaxFailedTimes,
			EmptyFallback:  emptyFallback,
			Providers:      providers,
			UsePosition:    option.UsePosition,
		}),
		fastSingle:     singledo.NewSingle[C.Proxy](time.Second * 10),
		disableUDP:     option.DisableUDP,
		testUrl:        option.URL,
		expectedStatus: option.ExpectedStatus,
		tolerance:      urlTestOption.Tolerance,
	}

	return urlTest, nil
}
