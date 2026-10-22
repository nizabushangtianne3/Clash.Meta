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
	host, portStr, err := net.SplitHostPort(rawAddr)
	if err != nil {
		return nil, err
	}

	portInt, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return nil, err
	}

	var buf []byte
	if i := strings.IndexByte(host, '%'); i >= 0 {
		// Zone identifiers are not representable in SOCKS5 IPv6 address encoding.
		host = host[:i]
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			buf = append(buf, atypIPv4)
			buf = append(buf, ip4...)
		} else {
			buf = append(buf, atypIPv6)
			ip16 := ip.To16()
			if ip16 == nil {
				return nil, fmt.Errorf("invalid ipv6: %q", host)
			}
			buf = append(buf, ip16...)
		}
	} else {
		if len(host) > 255 {
			return nil, fmt.Errorf("domain too long")
		}
		buf = append(buf, atypDomain)
		buf = append(buf, byte(len(host)))
		buf = append(buf, host...)
	}

	var portBytes [2]byte
	binary.BigEndian.PutUint16(portBytes[:], uint16(portInt))
	buf = append(buf, portBytes[:]...)
	return buf, nil
}

func DecodeAddress(r io.Reader) (string, error) {
	var atyp [1]byte
	if _, err := io.ReadFull(r, atyp[:]); err != nil {
		return "", err
	}

	switch atyp[0] {
	case atypIPv4:
		var ipBuf [net.IPv4len]byte
		if _, err := io.ReadFull(r, ipBuf[:]); err != nil {
			return "", err
		}
		var portBuf [2]byte
		if _, err := io.ReadFull(r, portBuf[:]); err != nil {
			return "", err
		}
		return net.JoinHostPort(net.IP(ipBuf[:]).String(), fmt.Sprint(binary.BigEndian.Uint16(portBuf[:]))), nil
	case atypIPv6:
		var ipBuf [net.IPv6len]byte
		if _, err := io.ReadFull(r, ipBuf[:]); err != nil {
			return "", err
		}
		var portBuf [2]byte
		if _, err := io.ReadFull(r, portBuf[:]); err != nil {
			return "", err
		}
		return net.JoinHostPort(net.IP(ipBuf[:]).String(), fmt.Sprint(binary.BigEndian.Uint16(portBuf[:]))), nil
	case atypDomain:
		var lengthBuf [1]byte
		if _, err := io.ReadFull(r, lengthBuf[:]); err != nil {
			return "", err
		}
		l := int(lengthBuf[0])
		hostBuf := make([]byte, l)
		if _, err := io.ReadFull(r, hostBuf); err != nil {
			return "", err
		}
		var portBuf [2]byte
		if _, err := io.ReadFull(r, portBuf[:]); err != nil {
			return "", err
		}
		return net.JoinHostPort(string(hostBuf), fmt.Sprint(binary.BigEndian.Uint16(portBuf[:]))), nil
	default:
		return "", fmt.Errorf("unknown address type: %d", atyp[0])
	}
}
