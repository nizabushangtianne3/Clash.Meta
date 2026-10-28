package sudoku

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"strconv"
	"strings"

	"github.com/metacubex/mihomo/transport/socks5"
)

// Address type tags of the KIP address form.
const (
	atypIPv4   byte = 0x01
	atypDomain byte = 0x03
	atypIPv6   byte = 0x04
)

// ErrAddrNotRepresentable reports an address field that is not an IP:port,
// which is a domain target. The UoT relay is IP only because the client
// resolves the destination before it writes.
var ErrAddrNotRepresentable = errors.New("address is not an ip:port")

// ReadAddrPortField reads one length-delimited address field as an AddrPort,
// so the datagram path never formats a host:port string only to parse it back.
// A field with no AddrPort form is consumed whole and reported as
// ErrAddrNotRepresentable, which leaves the reader on the next frame.
func ReadAddrPortField(r io.Reader, fieldLen int) (netip.AddrPort, error) {
	var atyp [1]byte
	if _, err := io.ReadFull(r, atyp[:]); err != nil {
		return netip.AddrPort{}, err
	}

	var buf [net.IPv6len + 2]byte
	size := -1
	switch atyp[0] {
	case atypIPv4:
		size = net.IPv4len + 2
	case atypIPv6:
		size = net.IPv6len + 2
	}
	if size < 0 || fieldLen != 1+size {
		if err := discardBytes(r, fieldLen-1); err != nil {
			return netip.AddrPort{}, err
		}
		return netip.AddrPort{}, ErrAddrNotRepresentable
	}
	if _, err := io.ReadFull(r, buf[:size]); err != nil {
		return netip.AddrPort{}, err
	}
	port := binary.BigEndian.Uint16(buf[size-2:])
	if atyp[0] == atypIPv4 {
		return netip.AddrPortFrom(netip.AddrFrom4([net.IPv4len]byte(buf[:net.IPv4len])), port), nil
	}
	return netip.AddrPortFrom(netip.AddrFrom16([net.IPv6len]byte(buf[:net.IPv6len])).Unmap(), port), nil
}

func EncodeAddress(rawAddr string) ([]byte, error) {
	return AppendAddress(nil, rawAddr)
}

// AppendAddress appends the SOCKS5 encoding of rawAddr ("host:port") to dst; dst is returned
// unchanged on error. IPv4-mapped IPv6 literals are encoded as IPv4 and IPv6 zones are dropped.
func AppendAddress(dst []byte, rawAddr string) ([]byte, error) {
	host, portStr, err := net.SplitHostPort(rawAddr)
	if err != nil {
		return dst, err
	}

	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return dst, err
	}

	if i := strings.IndexByte(host, '%'); i >= 0 {
		// Zone identifiers are not representable in SOCKS5 IPv6 address encoding.
		host = host[:i]
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		return AppendAddrPort(dst, netip.AddrPortFrom(ip, uint16(port))), nil
	}
	if len(host) > 255 {
		return dst, errors.New("domain too long")
	}
	dst = append(dst, socks5.AtypDomainName, byte(len(host)))
	dst = append(dst, host...)
	return append(dst, byte(port>>8), byte(port)), nil
}

// AppendAddrPort appends the SOCKS5 encoding of an IP destination to dst. IPv4-mapped IPv6
// addresses are encoded as IPv4 and zones are dropped.
func AppendAddrPort(dst []byte, ap netip.AddrPort) []byte {
	if ip := ap.Addr().Unmap(); ip.Is4() {
		v := ip.As4()
		dst = append(dst, socks5.AtypIPv4)
		dst = append(dst, v[:]...)
	} else {
		v := ip.As16()
		dst = append(dst, socks5.AtypIPv6)
		dst = append(dst, v[:]...)
	}
	return append(dst, byte(ap.Port()>>8), byte(ap.Port()))
}

// DecodeAddress parses the SOCKS5 address at the start of b and returns it as "host:port";
// bytes after the address are ignored.
func DecodeAddress(b []byte) (string, error) {
	addr := socks5.SplitAddr(b)
	if addr == nil {
		if len(b) == 0 {
			return "", io.EOF
		}
		return "", fmt.Errorf("invalid address: type %d, %d bytes", b[0], len(b))
	}
	return addr.String(), nil
}
