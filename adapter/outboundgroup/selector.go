package outboundgroup

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	C "github.com/metacubex/mihomo/constant"
	P "github.com/metacubex/mihomo/constant/provider"
)

type SelectorOption struct {
	DefaultSelected string `group:"default-selected,omitempty"`
}

type selectorSelection struct {
	proxy    C.Proxy
	versions []uint32
	version  uint64
}

type Selector struct {
	*GroupBase
	disableUDP      bool
	selectedMu      sync.RWMutex
	selected        string
	selectedVersion atomic.Uint64
	selection       atomic.Pointer[selectorSelection]
	testUrl         string
}

// DialContext implements C.ProxyAdapter
func (s *Selector) DialContext(ctx context.Context, metadata *C.Metadata) (C.Conn, error) {
	c, err := s.selectedProxy(true).DialContext(ctx, metadata)
	if err == nil {
		c.AppendToChains(s)
	}
	return c, err
}

// ListenPacketContext implements C.ProxyAdapter
func (s *Selector) ListenPacketContext(ctx context.Context, metadata *C.Metadata) (C.PacketConn, error) {
	pc, err := s.selectedProxy(true).ListenPacketContext(ctx, metadata)
	if err == nil {
		pc.AppendToChains(s)
	}
	return pc, err
}

// SupportUDP implements C.ProxyAdapter
func (s *Selector) SupportUDP() bool {
	if s.disableUDP {
		return false
	}

	return s.selectedProxy(false).SupportUDP()
}

// IsL3Protocol implements C.ProxyAdapter
func (s *Selector) IsL3Protocol(metadata *C.Metadata) bool {
	return s.selectedProxy(false).IsL3Protocol(metadata)
}

// AppendJSON implements C.ProxyAdapter
func (s *Selector) AppendJSON(dst []byte) []byte {
	dst = append(dst, `{"type":"Selector","name":`...)
	dst = writeJSONString(dst, s.Name())
	dst = append(dst, `,"now":`...)
	dst = writeJSONString(dst, s.Now())
	dst = append(dst, `,"all":`...)
	dst = writeGroupNames(dst, s.GetProxies(false))
	if s.testUrl == C.DefaultTestURL {
		dst = append(dst, `,"testUrl":""`...)
	} else {
		dst = writeStringField(dst, "testUrl", s.testUrl)
	}
	dst = writeBoolField(dst, "hidden", s.Hidden())
	dst = writeStringField(dst, "icon", s.Icon())
	dst = writeStringField(dst, "emptyFallback", s.EmptyFallback().Name())
	return append(dst, '}')
}

// MarshalJSON implements C.ProxyAdapter
func (s *Selector) MarshalJSON() ([]byte, error) {
	return s.AppendJSON(nil), nil
}

func (s *Selector) Now() string {
	return s.selectedProxy(false).Name()
}

func (s *Selector) setSelected(name string) {
	s.selectedMu.Lock()
	s.selected = name
	s.selectedVersion.Add(1)
	s.selectedMu.Unlock()
	s.selection.Store(nil)
}

func (s *Selector) Set(name string) error {
	for _, proxy := range s.GetProxies(false) {
		if proxy.Name() == name {
			s.setSelected(name)
			return nil
		}
	}

	return errors.New("proxy not exist")
}

func (s *Selector) ForceSet(name string) {
	s.setSelected(name)
}

// Unwrap implements C.ProxyAdapter
func (s *Selector) Unwrap(metadata *C.Metadata, touch bool) C.Proxy {
	return s.selectedProxy(touch)
}

func (s *Selector) selectedProxy(touch bool) C.Proxy {
	if touch {
		s.Touch()
		touch = false
	}

	if choice := s.selection.Load(); choice != nil &&
		s.selectedVersion.Load() == choice.version && s.providerVersionsEqual(choice.versions) {
		return choice.proxy
	}

	s.selectedMu.RLock()
	selected, version := s.selected, s.selectedVersion.Load()
	s.selectedMu.RUnlock()

	proxies := s.GetProxies(touch)
	cache := s.providerCache.Load()
	versions, fresh := s.providerVersionsFromCache()
	if !fresh {
		versions = s.providerVersionsNow()
	}
	if cache != nil && s.providerVersionsEqual(cache.versions) {
		proxies = cache.proxies
	}
	proxy := s.selectedFromProxies(proxies, selected)
	s.selection.Store(&selectorSelection{
		proxy:    proxy,
		versions: versions,
		version:  version,
	})
	return proxy
}

func (s *Selector) selectedFromProxies(proxies []C.Proxy, selected string) C.Proxy {
	for _, proxy := range proxies {
		if proxy.Name() == selected {
			return proxy
		}
	}

	return proxies[0]
}

func (s *Selector) Providers() []P.ProxyProvider {
	return s.providers
}

func (s *Selector) Proxies() []C.Proxy {
	return s.GetProxies(false)
}

func NewSelector(option GroupCommonOption, selectorOption SelectorOption, emptyFallback C.Proxy, providers []P.ProxyProvider) (*Selector, error) {
	return &Selector{
		GroupBase: NewGroupBase(GroupBaseOption{
			Name:           option.Name,
			Type:           C.Selector,
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
		selected:   selectorOption.DefaultSelected,
		disableUDP: option.DisableUDP,
		testUrl:    option.URL,
	}, nil
}
