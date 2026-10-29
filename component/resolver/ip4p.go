package resolver

import (
	"net"
	"net/netip"
	"strconv"

	"github.com/metacubex/mihomo/log"
)

var (
	ip4PEnable bool
)

func GetIP4PEnable() bool {
	return ip4PEnable
}

func SetIP4PEnable(enableIP4PConvert bool) {
	ip4PEnable = enableIP4PConvert
}

// kanged from https://github.com/heiher/frp/blob/ip4p/client/ip4p.go

func LookupIP4P(addr netip.Addr, port string) (netip.Addr, string) {
	if ip4PEnable {
		if converted, ok := LookupIP4PAddrPort(netip.AddrPortFrom(addr, 0)); ok {
			port = strconv.Itoa(int(converted.Port()))
			log.Debugln("Convert IP4P address %s to %s", addr, net.JoinHostPort(converted.Addr().String(), port))
			return converted.Addr(), port
		}
	}
	return addr, port
}

// LookupIP4PAddrPort converts a NAT64 address without materialising a host:port
// string.
func LookupIP4PAddrPort(addrPort netip.AddrPort) (netip.AddrPort, bool) {
	if !ip4PEnable {
		return addrPort, false
	}
	ip := addrPort.Addr()
	if !ip.Is6() || ip.As16()[0] != 0x20 || ip.As16()[1] != 0x01 ||
		ip.As16()[2] != 0x00 || ip.As16()[3] != 0x00 {
		return addrPort, false
	}
	bytes := ip.As16()
	port := uint16(bytes[10])<<8 + uint16(bytes[11])
	return netip.AddrPortFrom(netip.AddrFrom4([4]byte{bytes[12], bytes[13], bytes[14], bytes[15]}), port), true
}
