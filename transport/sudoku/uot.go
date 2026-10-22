package sudoku

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/metacubex/mihomo/common/pool"
	"github.com/metacubex/mihomo/log"
)

const (
	maxUoTPayload = 64 * 1024
	// maxUoTIPAddressLen bounds an encoded IP:port: the type tag, the widest
	// address, and the two port bytes.
	maxUoTIPAddressLen = 1 + net.IPv6len + 2
)

// WriteDatagram sends a single UDP datagram frame over a reliable stream.
func WriteDatagram(w io.Writer, addr netip.AddrPort, payload []byte) error {
	if !addr.IsValid() {
		return errors.New("invalid target address")
	}
	if payloadLen := len(payload); payloadLen > maxUoTPayload {
		return fmt.Errorf("payload too large: %d", payloadLen)
	}

	var frame [4 + maxUoTIPAddressLen]byte
	addrLen := putUoTAddress(frame[4:], addr)
	binary.BigEndian.PutUint16(frame[:2], uint16(addrLen))
	binary.BigEndian.PutUint16(frame[2:4], uint16(len(payload)))

	if _, err := w.Write(frame[:4+addrLen]); err != nil {
		return err
	}
	if len(payload) == 0 {
		return nil
	}
	_, err := w.Write(payload)
	return err
}

// putUoTAddress lays an IP:port into dst and reports the bytes it used.
func putUoTAddress(dst []byte, addr netip.AddrPort) int {
	ip := addr.Addr().Unmap()
	if ip.Is4() {
		dst[0] = atypIPv4
		ip4 := ip.As4()
		copy(dst[1:], ip4[:])
		binary.BigEndian.PutUint16(dst[1+net.IPv4len:], addr.Port())
		return 1 + net.IPv4len + 2
	}
	dst[0] = atypIPv6
	ip16 := ip.As16()
	copy(dst[1:], ip16[:])
	binary.BigEndian.PutUint16(dst[1+net.IPv6len:], addr.Port())
	return 1 + net.IPv6len + 2
}

// ReadDatagram parses a single UDP datagram frame from the reliable stream. The
// payload comes from the shared pool, so the caller releases it through the
// packet's Drop.
func ReadDatagram(r io.Reader) (netip.AddrPort, []byte, error) {
	addrLen, payloadLen, err := readDatagramHeader(r)
	if err != nil {
		return netip.AddrPort{}, nil, err
	}
	addr, err := ReadAddrPortField(r, addrLen)
	if err != nil {
		if discardErr := discardBytes(r, payloadLen); discardErr != nil {
			return netip.AddrPort{}, nil, discardErr
		}
		return netip.AddrPort{}, nil, err
	}
	payload := pool.Get(payloadLen)
	if _, err := io.ReadFull(r, payload); err != nil {
		_ = pool.Put(payload)
		return netip.AddrPort{}, nil, err
	}

	return addr, payload, nil
}

// UoTPacketConn adapts a net.Conn with the Sudoku UoT framing to net.PacketConn.
type UoTPacketConn struct {
	conn    net.Conn
	writeMu sync.Mutex
}

func NewUoTPacketConn(conn net.Conn) *UoTPacketConn {
	return &UoTPacketConn{conn: conn}
}

func (c *UoTPacketConn) ReadFrom(p []byte) (int, net.Addr, error) {
	for {
		addrLen, payloadLen, err := readDatagramHeader(c.conn)
		if err != nil {
			return 0, nil, err
		}

		addr, err := ReadAddrPortField(c.conn, addrLen)
		if err != nil {
			if discardErr := discardBytes(c.conn, payloadLen); discardErr != nil {
				return 0, nil, discardErr
			}
			if !errors.Is(err, ErrAddrNotRepresentable) {
				return 0, nil, err
			}
			if log.DebugEnabled() {
				log.Debugln("[Sudoku][UoT] drop datagram with a non-IP target")
			}
			continue
		}
		if payloadLen > len(p) {
			if discardErr := discardBytes(c.conn, payloadLen); discardErr != nil {
				return 0, nil, discardErr
			}
			return 0, nil, io.ErrShortBuffer
		}
		if _, err := io.ReadFull(c.conn, p[:payloadLen]); err != nil {
			return 0, nil, err
		}
		return payloadLen, net.UDPAddrFromAddrPort(addr), nil
	}
}

func (c *UoTPacketConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	udpAddr, ok := addr.(*net.UDPAddr)
	if !ok || udpAddr == nil {
		return 0, fmt.Errorf("unsupported address %T", addr)
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if err := WriteDatagram(c.conn, udpAddr.AddrPort(), p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (c *UoTPacketConn) Close() error {
	return c.conn.Close()
}

func (c *UoTPacketConn) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

func (c *UoTPacketConn) SetDeadline(t time.Time) error {
	return c.conn.SetDeadline(t)
}

func (c *UoTPacketConn) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

func (c *UoTPacketConn) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}

func readDatagramHeader(r io.Reader) (addrLen, payloadLen int, err error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return 0, 0, err
	}

	addrLen = int(binary.BigEndian.Uint16(header[:2]))
	payloadLen = int(binary.BigEndian.Uint16(header[2:]))
	if addrLen <= 0 || addrLen > maxUoTPayload {
		return 0, 0, fmt.Errorf("invalid address length: %d", addrLen)
	}
	if payloadLen > maxUoTPayload {
		return 0, 0, fmt.Errorf("invalid payload length: %d", payloadLen)
	}
	return addrLen, payloadLen, nil
}

func discardBytes(r io.Reader, n int) error {
	if n <= 0 {
		return nil
	}
	_, err := io.CopyN(io.Discard, r, int64(n))
	return err
}
