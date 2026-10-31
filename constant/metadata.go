package constant

import (
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"

	M "github.com/metacubex/sing/common/metadata"
)

// SOCKS address types as defined in RFC 1928 section 5.
const (
	AtypIPv4       AddrType = 1
	AtypDomainName AddrType = 3
	AtypIPv6       AddrType = 4
)

const (
	TCP NetWork = iota
	UDP
	ALLNet
	InvalidNet = 0xff
)

const (
	HTTP Type = iota
	HTTPS
	SOCKS4
	SOCKS5
	SHADOWSOCKS
	SNELL
	VMESS
	VLESS
	REDIR
	TPROXY
	TROJAN
	TUNNEL
	TUN
	TUIC
	HYSTERIA2
	ANYTLS
	MIERU
	SUDOKU
	TRUSTTUNNEL
	SHADOWQUIC
	INNER
)

type AddrType byte

func (a AddrType) String() string {
	switch a {
	case AtypIPv4:
		return "IPv4"
	case AtypDomainName:
		return "DomainName"
	case AtypIPv6:
		return "IPv6"
	default:
		return "Unknown"
	}
}

type NetWork int

func (n NetWork) String() string {
	switch n {
	case TCP:
		return "tcp"
	case UDP:
		return "udp"
	case ALLNet:
		return "all"
	default:
		return "invalid"
	}
}

func (n NetWork) MarshalJSON() ([]byte, error) {
	return strconv.AppendQuote(nil, n.String()), nil
}

type Type int

func (t Type) String() string {
	switch t {
	case HTTP:
		return "HTTP"
	case HTTPS:
		return "HTTPS"
	case SOCKS4:
		return "Socks4"
	case SOCKS5:
		return "Socks5"
	case SHADOWSOCKS:
		return "ShadowSocks"
	case SNELL:
		return "Snell"
	case VMESS:
		return "Vmess"
	case VLESS:
		return "Vless"
	case REDIR:
		return "Redir"
	case TPROXY:
		return "TProxy"
	case TROJAN:
		return "Trojan"
	case TUNNEL:
		return "Tunnel"
	case TUN:
		return "Tun"
	case TUIC:
		return "Tuic"
	case HYSTERIA2:
		return "Hysteria2"
	case ANYTLS:
		return "AnyTLS"
	case MIERU:
		return "Mieru"
	case SUDOKU:
		return "Sudoku"
	case TRUSTTUNNEL:
		return "TrustTunnel"
	case SHADOWQUIC:
		return "ShadowQuic"
	case INNER:
		return "Inner"
	default:
		return "Unknown"
	}
}

func ParseType(t string) (*Type, error) {
	var res Type
	switch t {
	case "HTTP":
		res = HTTP
	case "HTTPS":
		res = HTTPS
	case "SOCKS4":
		res = SOCKS4
	case "SOCKS5":
		res = SOCKS5
	case "SHADOWSOCKS":
		res = SHADOWSOCKS
	case "SNELL":
		res = SNELL
	case "VMESS":
		res = VMESS
	case "VLESS":
		res = VLESS
	case "REDIR":
		res = REDIR
	case "TPROXY":
		res = TPROXY
	case "TROJAN":
		res = TROJAN
	case "TUNNEL":
		res = TUNNEL
	case "TUN":
		res = TUN
	case "TUIC":
		res = TUIC
	case "HYSTERIA2":
		res = HYSTERIA2
	case "ANYTLS":
		res = ANYTLS
	case "MIERU":
		res = MIERU
	case "SUDOKU":
		res = SUDOKU
	case "TRUSTTUNNEL":
		res = TRUSTTUNNEL
	case "SHADOWQUIC":
		res = SHADOWQUIC
	case "INNER":
		res = INNER
	default:
		return nil, fmt.Errorf("unknown type: %s", t)
	}
	return &res, nil
}

func (t Type) MarshalJSON() ([]byte, error) {
	return strconv.AppendQuote(nil, t.String()), nil
}

// Metadata is used to store connection address
type Metadata struct {
	NetWork      NetWork    `json:"network"`
	Type         Type       `json:"type"`
	SrcIP        netip.Addr `json:"sourceIP"`
	DstIP        netip.Addr `json:"destinationIP"`
	SrcGeoIP     []string   `json:"sourceGeoIP"`      // can be nil if never queried, empty slice if got no result
	DstGeoIP     []string   `json:"destinationGeoIP"` // can be nil if never queried, empty slice if got no result
	SrcIPASN     string     `json:"sourceIPASN"`
	DstIPASN     string     `json:"destinationIPASN"`
	SrcPort      uint16     `json:"sourcePort,string"`      // `,string` is used to compatible with old version json output
	DstPort      uint16     `json:"destinationPort,string"` // `,string` is used to compatible with old version json output
	InIP         netip.Addr `json:"inboundIP"`
	InPort       uint16     `json:"inboundPort,string"` // `,string` is used to compatible with old version json output
	InName       string     `json:"inboundName"`
	InUser       string     `json:"inboundUser"`
	RematchName  string     `json:"rematchName"`
	Host         string     `json:"host"`
	DNSMode      DNSMode    `json:"dnsMode"`
	Uid          uint32     `json:"uid"`
	Process      string     `json:"process"`
	ProcessPath  string     `json:"processPath"`
	SpecialProxy string     `json:"specialProxy"`
	SpecialRules string     `json:"specialRules"`
	RemoteDst    string     `json:"remoteDestination"`
	DSCP         uint8      `json:"dscp"`

	RawSrcAddr net.Addr `json:"-"`
	RawDstAddr net.Addr `json:"-"`
	// Only domain rule
	SniffHost string `json:"sniffHost"`
}

func (m *Metadata) RemoteAddress() string {
	if m.Host == "" && m.DstIP.IsValid() {
		return netip.AddrPortFrom(m.DstIP.Unmap(), m.DstPort).String()
	}

	host := m.String()
	buf := make([]byte, 0, len(host)+6)
	if strings.Contains(host, ":") {
		buf = append(buf, '[')
		buf = append(buf, host...)
		buf = append(buf, ']', ':')
	} else {
		buf = append(buf, host...)
		buf = append(buf, ':')
	}
	buf = strconv.AppendUint(buf, uint64(m.DstPort), 10)
	return string(buf)
}

func (m *Metadata) appendSourceAddress(dst []byte) []byte {
	if !m.SrcIP.IsValid() {
		dst = append(dst, "invalid IP"...)
	} else {
		if m.SrcIP.Is6() {
			dst = append(dst, '[')
		}
		dst, _ = m.SrcIP.AppendText(dst)
		if m.SrcIP.Is6() {
			dst = append(dst, ']')
		}
	}
	dst = append(dst, ':')
	return strconv.AppendUint(dst, uint64(m.SrcPort), 10)
}

func (m *Metadata) SourceAddress() string {
	return string(m.appendSourceAddress(make([]byte, 0, 64)))
}

func (m *Metadata) SourceAddrPort() netip.AddrPort {
	return netip.AddrPortFrom(m.SrcIP.Unmap(), m.SrcPort)
}

func (m *Metadata) appendSourceDetail(dst []byte) []byte {
	if m.Type == INNER {
		return append(dst, MihomoName...)
	}

	dst = m.appendSourceAddress(dst)
	switch {
	case m.Process != "" && m.Uid != 0:
		dst = append(dst, '(')
		dst = append(dst, m.Process...)
		dst = append(dst, ',', ' ', 'u', 'i', 'd', '=')
		dst = strconv.AppendUint(dst, uint64(m.Uid), 10)
		return append(dst, ')')
	case m.Uid != 0:
		dst = append(dst, '(', 'u', 'i', 'd', '=')
		dst = strconv.AppendUint(dst, uint64(m.Uid), 10)
		return append(dst, ')')
	case m.Process != "":
		dst = append(dst, '(')
		dst = append(dst, m.Process...)
		return append(dst, ')')
	default:
		return dst
	}
}

func (m *Metadata) SourceDetail() string {
	if m.Type == INNER {
		return MihomoName
	}

	var source [64]byte
	sourceBytes := m.appendSourceAddress(source[:0])

	var builder strings.Builder
	builder.Grow(len(sourceBytes) + len(m.Process) + 24)
	builder.Write(sourceBytes)
	switch {
	case m.Process != "" && m.Uid != 0:
		builder.WriteByte('(')
		builder.WriteString(m.Process)
		builder.WriteString(", uid=")
		var uid [20]byte
		builder.Write(strconv.AppendUint(uid[:0], uint64(m.Uid), 10))
		builder.WriteByte(')')
	case m.Uid != 0:
		builder.WriteString("(uid=")
		var uid [20]byte
		builder.Write(strconv.AppendUint(uid[:0], uint64(m.Uid), 10))
		builder.WriteByte(')')
	case m.Process != "":
		builder.WriteByte('(')
		builder.WriteString(m.Process)
		builder.WriteByte(')')
	}
	return builder.String()
}

func (m *Metadata) SourceValid() bool {
	return m.SrcPort != 0 && m.SrcIP.IsValid()
}

func (m *Metadata) AddrType() AddrType {
	switch true {
	case m.Host != "" || !m.DstIP.IsValid():
		return AtypDomainName
	case m.DstIP.Is4():
		return AtypIPv4
	default:
		return AtypIPv6
	}
}

func (m *Metadata) Resolved() bool {
	return m.DstIP.IsValid()
}

func (m *Metadata) RuleHost() string {
	if len(m.SniffHost) == 0 {
		return m.Host
	} else {
		return m.SniffHost
	}
}

// Pure is used to solve unexpected behavior
// when dialing proxy connection in DNSMapping mode.
func (m *Metadata) Pure() *Metadata {
	if (m.DNSMode == DNSMapping || m.DNSMode == DNSHosts) && m.DstIP.IsValid() {
		copyM := *m
		copyM.Host = ""
		return &copyM
	}

	return m
}

func (m *Metadata) Clone() *Metadata {
	copyM := *m
	return &copyM
}

func (m *Metadata) AddrPort() netip.AddrPort {
	return netip.AddrPortFrom(m.DstIP.Unmap(), m.DstPort)
}

// Socksaddr returns the destination in sing's address form. A name keeps its
// name and an IP target comes from the AddrPort, so the destination is never
// formatted into a host:port string only to be parsed back.
func (m *Metadata) Socksaddr() M.Socksaddr {
	if m.Host != "" {
		return M.ParseSocksaddrHostPort(m.Host, m.DstPort)
	}
	return M.SocksaddrFromNetIP(m.AddrPort())
}

func (m *Metadata) UDPAddr() *net.UDPAddr {
	if m.NetWork != UDP || !m.DstIP.IsValid() {
		return nil
	}
	return net.UDPAddrFromAddrPort(m.AddrPort())
}

func (m *Metadata) String() string {
	if m.Host != "" {
		return m.Host
	} else if m.DstIP.IsValid() {
		return m.DstIP.String()
	} else {
		return "<nil>"
	}
}

func (m *Metadata) Valid() bool {
	return m.Host != "" || m.DstIP.IsValid()
}

func (m *Metadata) SetRemoteAddr(addr net.Addr) error {
	if addr == nil {
		return nil
	}
	if rawAddr, ok := addr.(interface{ RawAddr() net.Addr }); ok {
		if rawAddr := rawAddr.RawAddr(); rawAddr != nil {
			if err := m.SetRemoteAddr(rawAddr); err == nil {
				return nil
			}
		}
	}
	if addr, ok := addr.(interface{ AddrPort() netip.AddrPort }); ok { // *net.TCPAddr, *net.UDPAddr, M.Socksaddr
		if addrPort := addr.AddrPort(); addrPort.Port() != 0 {
			m.DstPort = addrPort.Port()
			if addrPort.IsValid() { // sing's M.Socksaddr maybe return an invalid AddrPort if it's a DomainName
				m.DstIP = addrPort.Addr().Unmap()
				return nil
			} else {
				if addr, ok := addr.(interface{ AddrString() string }); ok { // must be sing's M.Socksaddr
					m.Host = addr.AddrString() // actually is M.Socksaddr.Fqdn
					return nil
				}
			}
		}
	}
	return m.SetRemoteAddress(addr.String())
}

func (m *Metadata) SetRemoteAddress(rawAddress string) error {
	host, port, err := net.SplitHostPort(rawAddress)
	if err != nil {
		return err
	}

	uint64Port, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return err
	}
	uint16Port := uint16(uint64Port)

	if ip, err := netip.ParseAddr(host); err != nil {
		m.Host = host
		m.DstIP = netip.Addr{}
	} else {
		m.Host = ""
		m.DstIP = ip.Unmap()
	}
	m.DstPort = uint16Port

	return nil
}

func (m *Metadata) SwapSrcDst() {
	m.SrcIP, m.DstIP = m.DstIP, m.SrcIP
	m.SrcPort, m.DstPort = m.DstPort, m.SrcPort
	m.SrcIPASN, m.DstIPASN = m.DstIPASN, m.SrcIPASN
	m.SrcGeoIP, m.DstGeoIP = m.DstGeoIP, m.SrcGeoIP
}
