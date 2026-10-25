package config

import (
	"time"

	"github.com/metacubex/mihomo/adapter"
	"github.com/metacubex/mihomo/adapter/inbound"
	"github.com/metacubex/mihomo/component/dialer"
	"github.com/metacubex/mihomo/component/geodata"
	mihomoHttp "github.com/metacubex/mihomo/component/http"
	"github.com/metacubex/mihomo/component/iface"
	"github.com/metacubex/mihomo/component/keepalive"
	"github.com/metacubex/mihomo/component/resolver"
	"github.com/metacubex/mihomo/component/resource"
	"github.com/metacubex/mihomo/log"
	"github.com/metacubex/mihomo/tunnel"
)

// SetGeneral applies a General to the parts of the runtime a config owns.
// logging announces the settings that are only worth announcing when a whole
// config is applied.
func SetGeneral(general *General, logging bool) {
	tunnel.SetMode(general.Mode)
	tunnel.SetFindProcessMode(general.FindProcessMode)
	resolver.DisableIPv6 = !general.IPv6

	dialer.SetTcpConcurrent(general.TCPConcurrent)
	if logging && general.TCPConcurrent {
		log.Infoln("Use tcp concurrent")
	}

	inbound.SetTfo(general.InboundTfo)
	inbound.SetMPTCP(general.InboundMPTCP)

	keepalive.SetKeepAliveIdle(time.Duration(general.KeepAliveIdle) * time.Second)
	keepalive.SetKeepAliveInterval(time.Duration(general.KeepAliveInterval) * time.Second)
	keepalive.SetDisableKeepAlive(general.DisableKeepAlive)

	adapter.UnifiedDelay.Store(general.UnifiedDelay)

	dialer.DefaultInterface.Store(general.Interface)
	dialer.DefaultRoutingMark.Store(int32(general.RoutingMark))
	if logging && general.RoutingMark > 0 {
		log.Infoln("Use routing mark: %#x", general.RoutingMark)
	}

	iface.FlushCache()

	geodata.SetGeodataMode(general.GeodataMode)
	geodata.SetLoader(general.GeodataLoader)
	geodata.SetSiteMatcher(general.GeositeMatcher)
	geodata.SetGeoIpUrl(general.GeoXUrl.GeoIp)
	geodata.SetGeoSiteUrl(general.GeoXUrl.GeoSite)
	geodata.SetMmdbUrl(general.GeoXUrl.Mmdb)
	geodata.SetASNUrl(general.GeoXUrl.ASN)
	mihomoHttp.SetUA(general.GlobalUA)
	resource.SetETag(general.ETagSupport)
}

// TemporarilySetGeneral applies a General and returns a rollback for what was
// applied before it. Rule and provider parsing loads geodata and downloads
// providers, and both read these settings, so a parse pushes them in and takes
// them out again.
func TemporarilySetGeneral(general *General) func() {
	previous := &General{
		Mode:            tunnel.Mode(),
		FindProcessMode: tunnel.FindProcessMode(),
		IPv6:            !resolver.DisableIPv6,
		TCPConcurrent:   dialer.GetTcpConcurrent(),
		Inbound: Inbound{
			InboundTfo:   inbound.Tfo(),
			InboundMPTCP: inbound.MPTCP(),
		},
		KeepAliveIdle:     int(keepalive.KeepAliveIdle() / time.Second),
		KeepAliveInterval: int(keepalive.KeepAliveInterval() / time.Second),
		DisableKeepAlive:  keepalive.DisableKeepAlive(),
		UnifiedDelay:      adapter.UnifiedDelay.Load(),
		Interface:         dialer.DefaultInterface.Load(),
		RoutingMark:       int(dialer.DefaultRoutingMark.Load()),
		GeoXUrl: GeoXUrl{
			GeoIp:   geodata.GeoIpUrl(),
			Mmdb:    geodata.MmdbUrl(),
			ASN:     geodata.ASNUrl(),
			GeoSite: geodata.GeoSiteUrl(),
		},
		GeodataMode:    geodata.GeodataMode(),
		GeodataLoader:  geodata.LoaderName(),
		GeositeMatcher: geodata.SiteMatcherName(),
		GlobalUA:       mihomoHttp.UA(),
		ETagSupport:    resource.ETag(),
	}
	SetGeneral(general, false)
	return func() { SetGeneral(previous, false) }
}
