package adapter

import (
	"fmt"

	"github.com/metacubex/mihomo/adapter/outbound"
	"github.com/metacubex/mihomo/common/structure"
	C "github.com/metacubex/mihomo/constant"
)

// A Decoder only holds its options, so one instance serves every proxy the
// config and its providers carry; a config with a thousand proxies used to
// build a thousand of them.
var proxyDecoder = structure.NewDecoder(structure.Option{
	TagName:          "proxy",
	WeaklyTypedInput: true,
	KeyReplacer:      structure.DefaultKeyReplacer,
})

func ParseProxy(mapping map[string]any, options *ProxyOptions) (C.Proxy, error) {
	proxyType, existType := mapping["type"].(string)
	if !existType {
		return nil, fmt.Errorf("missing type")
	}

	var basicOption outbound.BasicOption
	if options != nil {
		basicOption = outbound.BasicOption{
			DialerForAPI: options.DialerForAPI,
			TunnelForAPI: options.TunnelForAPI,
			ProviderName: options.ProviderName,
		}
	}

	var (
		proxy outbound.ProxyAdapter
		err   error
	)
	switch proxyType {
	case "ss":
		ssOption := &outbound.ShadowSocksOption{BasicOption: basicOption}
		err = proxyDecoder.Decode(mapping, ssOption)
		if err != nil {
			break
		}
		proxy, err = outbound.NewShadowSocks(*ssOption)
	case "ssr":
		ssrOption := &outbound.ShadowSocksROption{BasicOption: basicOption}
		err = proxyDecoder.Decode(mapping, ssrOption)
		if err != nil {
			break
		}
		proxy, err = outbound.NewShadowSocksR(*ssrOption)
	case "socks5":
		socksOption := &outbound.Socks5Option{BasicOption: basicOption}
		err = proxyDecoder.Decode(mapping, socksOption)
		if err != nil {
			break
		}
		proxy, err = outbound.NewSocks5(*socksOption)
	case "http":
		httpOption := &outbound.HttpOption{BasicOption: basicOption}
		err = proxyDecoder.Decode(mapping, httpOption)
		if err != nil {
			break
		}
		proxy, err = outbound.NewHttp(*httpOption)
	case "vmess":
		vmessOption := &outbound.VmessOption{BasicOption: basicOption}
		err = proxyDecoder.Decode(mapping, vmessOption)
		if err != nil {
			break
		}
		proxy, err = outbound.NewVmess(*vmessOption)
	case "vless":
		vlessOption := &outbound.VlessOption{BasicOption: basicOption}
		err = proxyDecoder.Decode(mapping, vlessOption)
		if err != nil {
			break
		}
		proxy, err = outbound.NewVless(*vlessOption)
	case "snell":
		snellOption := &outbound.SnellOption{BasicOption: basicOption}
		err = proxyDecoder.Decode(mapping, snellOption)
		if err != nil {
			break
		}
		proxy, err = outbound.NewSnell(*snellOption)
	case "trojan":
		trojanOption := &outbound.TrojanOption{BasicOption: basicOption}
		err = proxyDecoder.Decode(mapping, trojanOption)
		if err != nil {
			break
		}
		proxy, err = outbound.NewTrojan(*trojanOption)
	case "hysteria":
		hyOption := &outbound.HysteriaOption{BasicOption: basicOption}
		err = proxyDecoder.Decode(mapping, hyOption)
		if err != nil {
			break
		}
		proxy, err = outbound.NewHysteria(*hyOption)
	case "hysteria2":
		hyOption := &outbound.Hysteria2Option{BasicOption: basicOption}
		err = proxyDecoder.Decode(mapping, hyOption)
		if err != nil {
			break
		}
		proxy, err = outbound.NewHysteria2(*hyOption)
	case "wireguard":
		wgOption := &outbound.WireGuardOption{BasicOption: basicOption}
		err = proxyDecoder.Decode(mapping, wgOption)
		if err != nil {
			break
		}
		proxy, err = outbound.NewWireGuard(*wgOption)
	case "tuic":
		tuicOption := &outbound.TuicOption{BasicOption: basicOption}
		err = proxyDecoder.Decode(mapping, tuicOption)
		if err != nil {
			break
		}
		proxy, err = outbound.NewTuic(*tuicOption)
	case "shadowquic":
		shadowQuicOption := &outbound.ShadowQuicOption{BasicOption: basicOption}
		err = proxyDecoder.Decode(mapping, shadowQuicOption)
		if err != nil {
			break
		}
		proxy, err = outbound.NewShadowQuic(*shadowQuicOption)
	case "gost-relay":
		relayOption := &outbound.GostRelayOption{BasicOption: basicOption}
		err = proxyDecoder.Decode(mapping, relayOption)
		if err != nil {
			break
		}
		proxy, err = outbound.NewGostRelay(*relayOption)
	case "direct":
		directOption := &outbound.DirectOption{BasicOption: basicOption}
		err = proxyDecoder.Decode(mapping, directOption)
		if err != nil {
			break
		}
		proxy = outbound.NewDirectWithOption(*directOption)
	case "dns":
		dnsOptions := &outbound.DnsOption{BasicOption: basicOption}
		err = proxyDecoder.Decode(mapping, dnsOptions)
		if err != nil {
			break
		}
		proxy = outbound.NewDnsWithOption(*dnsOptions)
	case "reject":
		rejectOption := &outbound.RejectOption{BasicOption: basicOption}
		err = proxyDecoder.Decode(mapping, rejectOption)
		if err != nil {
			break
		}
		proxy = outbound.NewRejectWithOption(*rejectOption)
	case "rematch":
		rematchOption := &outbound.RematchOption{BasicOption: basicOption}
		err = proxyDecoder.Decode(mapping, rematchOption)
		if err != nil {
			break
		}
		proxy, err = outbound.NewRematch(*rematchOption)
	case "ssh":
		sshOption := &outbound.SshOption{BasicOption: basicOption}
		err = proxyDecoder.Decode(mapping, sshOption)
		if err != nil {
			break
		}
		proxy, err = outbound.NewSsh(*sshOption)
	case "mieru":
		mieruOption := &outbound.MieruOption{BasicOption: basicOption}
		err = proxyDecoder.Decode(mapping, mieruOption)
		if err != nil {
			break
		}
		proxy, err = outbound.NewMieru(*mieruOption)
	case "anytls":
		anytlsOption := &outbound.AnyTLSOption{BasicOption: basicOption}
		err = proxyDecoder.Decode(mapping, anytlsOption)
		if err != nil {
			break
		}
		proxy, err = outbound.NewAnyTLS(*anytlsOption)
	case "sudoku":
		sudokuOption := &outbound.SudokuOption{BasicOption: basicOption}
		err = proxyDecoder.Decode(mapping, sudokuOption)
		if err != nil {
			break
		}
		proxy, err = outbound.NewSudoku(*sudokuOption)
	case "masque":
		masqueOption := &outbound.MasqueOption{BasicOption: basicOption}
		err = proxyDecoder.Decode(mapping, masqueOption)
		if err != nil {
			break
		}
		proxy, err = outbound.NewMasque(*masqueOption)
	case "trusttunnel":
		trustTunnelOption := &outbound.TrustTunnelOption{BasicOption: basicOption}
		err = proxyDecoder.Decode(mapping, trustTunnelOption)
		if err != nil {
			break
		}
		proxy, err = outbound.NewTrustTunnel(*trustTunnelOption)
	case "openvpn":
		openVPNOption := &outbound.OpenVPNOption{BasicOption: basicOption}
		err = proxyDecoder.Decode(mapping, openVPNOption)
		if err != nil {
			break
		}
		proxy, err = outbound.NewOpenVPN(*openVPNOption)
	case "tailscale":
		tailscaleOption := &outbound.TailscaleOption{BasicOption: basicOption}
		err = proxyDecoder.Decode(mapping, tailscaleOption)
		if err != nil {
			break
		}
		proxy, err = outbound.NewTailscale(*tailscaleOption)
	case "zerotier":
		zeroTierOption := &outbound.ZeroTierOption{BasicOption: basicOption}
		err = proxyDecoder.Decode(mapping, zeroTierOption)
		if err != nil {
			break
		}
		proxy, err = outbound.NewZeroTier(*zeroTierOption)
	case "easytier":
		easyTierOption := &outbound.EasyTierOption{BasicOption: basicOption}
		err = proxyDecoder.Decode(mapping, easyTierOption)
		if err != nil {
			break
		}
		proxy, err = outbound.NewEasyTier(*easyTierOption)
	default:
		return nil, fmt.Errorf("unsupport proxy type: %s", proxyType)
	}

	if err != nil {
		return nil, err
	}

	if muxMapping, muxExist := mapping["smux"].(map[string]any); muxExist {
		muxOption := &outbound.SingMuxOption{}
		err = proxyDecoder.Decode(muxMapping, muxOption)
		if err != nil {
			return nil, err
		}
		if muxOption.Enabled {
			proxy, err = outbound.NewSingMux(*muxOption, proxy)
			if err != nil {
				return nil, err
			}
		}
	}

	proxy = outbound.NewAutoCloseProxyAdapter(proxy)
	return NewProxy(proxy), nil
}

type ProxyOptions struct {
	DialerForAPI C.Dialer
	TunnelForAPI C.Tunnel
	ProviderName string
}
