package sudoku

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/metacubex/mihomo/common/pool"
	"github.com/metacubex/mihomo/log"
	"github.com/metacubex/mihomo/transport/socks5"
)

// uotHeaderSize is u16 address length | u16 payload length.
const uotHeaderSize = 4

// WriteDatagram sends a single UDP datagram frame over a reliable stream. The
// target is an AddrPort because the UoT relay is IP only: the client resolves
// the destination before it writes.
func WriteDatagram(w io.Writer, addr netip.AddrPort, payload []byte) error {
	if !addr.IsValid() {
		return errors.New("invalid target address")
	}
	return writeDatagram(w, payload, func(b []byte) ([]byte, error) { return AppendAddrPort(b, addr), nil })
}

// writeDatagram sends the whole frame with one write, so it is sealed as one record.
func writeDatagram(w io.Writer, payload []byte, appendAddr func([]byte) ([]byte, error)) error {
	if len(payload) > math.MaxUint16 {
		return fmt.Errorf("payload too large: %d", len(payload))
	}
	buf := pool.Get(uotHeaderSize + socks5.MaxAddrLen + len(payload))
	defer pool.Put(buf)

	frame, err := appendAddr(buf[:uotHeaderSize])
	if err != nil {
		return fmt.Errorf("encode address: %w", err)
	}
	binary.BigEndian.PutUint16(frame, uint16(len(frame)-uotHeaderSize))
	binary.BigEndian.PutUint16(frame[2:], uint16(len(payload)))
	_, err = w.Write(append(frame, payload...))
	return err
}

// ReadDatagram reads the next frame off a bare reader, for callers that hold no
// long-lived reader of their own.
func ReadDatagram(r io.Reader) (netip.AddrPort, []byte, error) {
	return NewUoTReader(r).ReadDatagram()
}

// UoTReader reads datagram frames from a UoT stream. The address is read straight into an
// AddrPort, so the datagram path never formats a host:port string only to parse it back.
type UoTReader struct {
	r   io.Reader
	buf [uotHeaderSize]byte
}

func NewUoTReader(r io.Reader) *UoTReader {
	return &UoTReader{r: r}
}

// readHeader reads the next frame's header and address and leaves its payload (payloadLen bytes)
// unread. A non-IP address field is consumed whole and reported as ErrAddrNotRepresentable,
// which leaves the reader on the next frame.
func (u *UoTReader) readHeader() (netip.AddrPort, int, error) {
	if _, err := io.ReadFull(u.r, u.buf[:]); err != nil {
		return netip.AddrPort{}, 0, err
	}
	addrLen := int(binary.BigEndian.Uint16(u.buf[:2]))
	payloadLen := int(binary.BigEndian.Uint16(u.buf[2:]))
	if addrLen == 0 {
		return netip.AddrPort{}, 0, fmt.Errorf("invalid address length: %d", addrLen)
	}
	addr, err := ReadAddrPortField(u.r, addrLen)
	if err != nil {
		if errors.Is(err, ErrAddrNotRepresentable) {
			// Consume this frame's payload too, so a caller that drops the frame
			// stays aligned for the next one.
			if discardErr := discardBytes(u.r, payloadLen); discardErr != nil {
				return netip.AddrPort{}, 0, discardErr
			}
		}
		return netip.AddrPort{}, 0, err
	}
	return addr, payloadLen, nil
}

// ReadDatagram reads the next frame. payload comes from common/pool and must be released
// with pool.Put.
func (u *UoTReader) ReadDatagram() (netip.AddrPort, []byte, error) {
	addr, payloadLen, err := u.readHeader()
	if err != nil {
		return netip.AddrPort{}, nil, err
	}
	payload := pool.Get(payloadLen)
	if _, err := io.ReadFull(u.r, payload); err != nil {
		_ = pool.Put(payload)
		return netip.AddrPort{}, nil, err
	}
	return addr, payload, nil
}

// UoTPacketConn adapts a net.Conn with the Sudoku UoT framing to net.PacketConn.
type UoTPacketConn struct {
	conn    net.Conn
	reader  *UoTReader
	writeMu sync.Mutex
}

func NewUoTPacketConn(conn net.Conn) *UoTPacketConn {
	return &UoTPacketConn{conn: conn, reader: NewUoTReader(conn)}
}

func (c *UoTPacketConn) ReadFrom(p []byte) (int, net.Addr, error) {
	for {
		addrPort, payloadLen, err := c.reader.readHeader()
		if err != nil {
			if errors.Is(err, ErrAddrNotRepresentable) {
				// A domain target cannot name a UDP flow; readHeader already consumed
				// the whole frame, so stay on the next one.
				log.Debugln("[Sudoku][UoT] discard datagram with a non-IP target")
				continue
			}
			return 0, nil, err
		}
		udpAddr := net.UDPAddrFromAddrPort(addrPort)
		if payloadLen > len(p) {
			if discardErr := discardBytes(c.conn, payloadLen); discardErr != nil {
				return 0, nil, discardErr
			}
			return 0, nil, io.ErrShortBuffer
		}
		if _, err := io.ReadFull(c.conn, p[:payloadLen]); err != nil {
			return 0, nil, err
		}
		return payloadLen, udpAddr, nil
	}
}

func (c *UoTPacketConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	if addr == nil {
		return 0, errors.New("address is nil")
	}
	appendAddr := func(b []byte) ([]byte, error) { return AppendAddress(b, addr.String()) }
	if udpAddr, ok := addr.(*net.UDPAddr); ok {
		if ap := udpAddr.AddrPort(); ap.Addr().IsValid() {
			appendAddr = func(b []byte) ([]byte, error) { return AppendAddrPort(b, ap), nil }
		}
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if err := writeDatagram(c.conn, p, appendAddr); err != nil {
		return 0, err
	}
	return len(p), nil
}

// WriteToAddrPort writes one datagram to an AddrPort destination. The UDP write-back
// contract hands its address as a netip.AddrPort, so going straight to AppendAddrPort
// spares the net.Addr the interface would allocate.
func (c *UoTPacketConn) WriteToAddrPort(p []byte, addr netip.AddrPort) (int, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if err := writeDatagram(c.conn, p, func(b []byte) ([]byte, error) { return AppendAddrPort(b, addr), nil }); err != nil {
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

func discardBytes(r io.Reader, n int) error {
	if n <= 0 {
		return nil
	}
	_, err := io.CopyN(io.Discard, r, int64(n))
	return err
}
