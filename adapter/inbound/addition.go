package inbound

import (
	"net"

	M "github.com/metacubex/sing/common/metadata"

	C "github.com/metacubex/mihomo/constant"
)

type Addition func(metadata *C.Metadata)

func ApplyAdditions(metadata *C.Metadata, additions ...Addition) {
	for _, addition := range additions {
		addition(metadata)
	}
}

func WithInName(name string) Addition {
	return func(metadata *C.Metadata) {
		metadata.InName = name
	}
}

func WithInUser(user string) Addition {
	return func(metadata *C.Metadata) {
		metadata.InUser = user
	}
}

func WithSpecialRules(specialRules string) Addition {
	return func(metadata *C.Metadata) {
		metadata.SpecialRules = specialRules
	}
}

func WithSpecialProxy(specialProxy string) Addition {
	return func(metadata *C.Metadata) {
		metadata.SpecialProxy = specialProxy
	}
}

func WithDstAddr(addr net.Addr) Addition {
	return func(metadata *C.Metadata) {
		_ = metadata.SetRemoteAddr(addr)
	}
}

func WithSrcAddr(addr net.Addr) Addition {
	return func(metadata *C.Metadata) {
		SetSrcAddr(metadata, addr)
	}
}

// SetSrcAddr applies the same fields as WithSrcAddr without allocating a
// closure per packet.
func SetSrcAddr(metadata *C.Metadata, addr net.Addr) {
	m := C.Metadata{}
	if err := m.SetRemoteAddr(addr); err == nil {
		metadata.SrcIP = m.DstIP
		metadata.SrcPort = m.DstPort
	}
}

// SetSrcSocksaddr applies the source fields from the address a listener stack
// carries. That address arrives as a Socksaddr, and handing it over as a
// net.Addr would box the struct on every connection it sets.
func SetSrcSocksaddr(metadata *C.Metadata, addr M.Socksaddr) {
	if !addr.Addr.IsValid() { // the interface path leaves a name's fields alone
		return
	}
	metadata.SrcIP = addr.Addr.Unmap()
	metadata.SrcPort = addr.Port
}

func WithInAddr(addr net.Addr) Addition {
	return func(metadata *C.Metadata) {
		SetInAddr(metadata, addr)
	}
}

// SetInAddr applies the same fields as WithInAddr without allocating a closure.
func SetInAddr(metadata *C.Metadata, addr net.Addr) {
	m := C.Metadata{}
	if err := m.SetRemoteAddr(addr); err == nil {
		metadata.InIP = m.DstIP
		metadata.InPort = m.DstPort
	}
}

func WithDSCP(dscp uint8) Addition {
	return func(metadata *C.Metadata) {
		metadata.DSCP = dscp
	}
}

func Placeholder(metadata *C.Metadata) {}
