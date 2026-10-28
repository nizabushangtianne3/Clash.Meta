package trusttunnel

import (
	"math"
	"net"

	"github.com/metacubex/sing/common"
	"github.com/metacubex/sing/common/buf"
	E "github.com/metacubex/sing/common/exceptions"
	M "github.com/metacubex/sing/common/metadata"
	N "github.com/metacubex/sing/common/network"
	"github.com/metacubex/sing/common/rw"
)

func writeUint16(buffer *buf.Buffer, v uint16) error {
	if err := buffer.WriteByte(byte(v >> 8)); err != nil {
		return err
	}
	return buffer.WriteByte(byte(v))
}

func writeUint32(buffer *buf.Buffer, v uint32) error {
	for shift := 24; shift >= 0; shift -= 8 {
		if err := buffer.WriteByte(byte(v >> shift)); err != nil {
			return err
		}
	}
	return nil
}

func readUint16(buffer *buf.Buffer) (uint16, error) {
	hi, err := buffer.ReadByte()
	if err != nil {
		return 0, err
	}
	lo, err := buffer.ReadByte()
	if err != nil {
		return 0, err
	}
	return uint16(hi)<<8 | uint16(lo), nil
}

func readUint32(buffer *buf.Buffer) (uint32, error) {
	var v uint32
	for range 4 {
		b, err := buffer.ReadByte()
		if err != nil {
			return 0, err
		}
		v = v<<8 | uint32(b)
	}
	return v, nil
}

type packetConn struct {
	httpConn
	readWaitOptions N.ReadWaitOptions
}

func (c *packetConn) InitializeReadWaiter(options N.ReadWaitOptions) (needCopy bool) {
	c.readWaitOptions = options
	return false
}

var (
	_ N.NetPacketConn    = (*clientPacketConn)(nil)
	_ N.FrontHeadroom    = (*clientPacketConn)(nil)
	_ N.PacketReadWaiter = (*clientPacketConn)(nil)
)

type clientPacketConn struct {
	packetConn
}

func (u *clientPacketConn) FrontHeadroom() int {
	return 4 + 16 + 2 + 16 + 2 + 1 + math.MaxUint8
}

func (u *clientPacketConn) WaitReadPacket() (buffer *buf.Buffer, destination M.Socksaddr, err error) {
	buffer = u.readWaitOptions.NewPacketBuffer()
	destination, err = u.ReadPacket(buffer)
	if err != nil {
		buffer.Release()
		return nil, M.Socksaddr{}, err
	}
	u.readWaitOptions.PostReturn(buffer)
	return buffer, destination, nil
}

func (u *clientPacketConn) ReadPacket(buffer *buf.Buffer) (destination M.Socksaddr, err error) {
	err = u.waitCreated()
	if err != nil {
		return M.Socksaddr{}, err
	}
	return u.readPacketFromServer(buffer)
}

func (u *clientPacketConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	buffer := buf.With(p)
	destination, err := u.ReadPacket(buffer)
	if err != nil {
		return 0, nil, err
	}
	return buffer.Len(), destination.UDPAddr(), nil
}

func (u *clientPacketConn) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	return u.writePacketToServer(buffer, destination)
}

func (u *clientPacketConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	err = u.WritePacket(buf.As(p), M.SocksaddrFromNet(addr))
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func (u *clientPacketConn) readPacketFromServer(buffer *buf.Buffer) (destination M.Socksaddr, err error) {
	header := buf.NewSize(4 + 16 + 2 + 16 + 2)
	defer header.Release()
	_, err = header.ReadFullFrom(u.body, header.Cap())
	if err != nil {
		return
	}
	length := common.Must1(readUint32(header))
	var sourceAddressBuffer [16]byte
	common.Must1(header.Read(sourceAddressBuffer[:]))
	destination.Addr = parse16BytesIP(sourceAddressBuffer)
	destination.Port = common.Must1(readUint16(header))
	common.Must(rw.SkipN(header, 16+2)) // To local address:port
	payloadLen := int(length) - (16 + 2 + 16 + 2)
	if payloadLen < 0 {
		return M.Socksaddr{}, E.New("invalid udp length: ", length)
	}
	_, err = buffer.ReadFullFrom(u.body, payloadLen)
	return
}

func (u *clientPacketConn) writePacketToServer(buffer *buf.Buffer, source M.Socksaddr) error {
	defer buffer.Release()
	if !source.IsIP() {
		return E.New("only support IP")
	}
	appName := AppName
	if len(appName) > math.MaxUint8 {
		appName = appName[:math.MaxUint8]
	}
	payloadLen := buffer.Len()
	headerLen := 4 + 16 + 2 + 16 + 2 + 1 + len(appName)
	lengthField := uint32(16 + 2 + 16 + 2 + 1 + len(appName) + payloadLen)
	destinationAddress := buildPaddingIP(source.Addr)

	var (
		header         *buf.Buffer
		headerInBuffer bool
	)
	if buffer.Start() >= headerLen {
		headerBytes := buffer.ExtendHeader(headerLen)
		header = buf.With(headerBytes)
		headerInBuffer = true
	} else {
		header = buf.NewSize(headerLen)
		defer header.Release()
	}
	common.Must(writeUint32(header, lengthField))
	common.Must(header.WriteZeroN(16 + 2)) // Source address:port (unknown)
	common.Must1(header.Write(destinationAddress[:]))
	common.Must(writeUint16(header, source.Port))
	common.Must(header.WriteByte(byte(len(appName))))
	common.Must1(header.WriteString(appName))
	if !headerInBuffer {
		_, err := u.writer.Write(header.Bytes())
		if err != nil {
			return err
		}
	}
	_, err := u.writer.Write(buffer.Bytes())
	if err != nil {
		return err
	}
	if u.flusher != nil {
		u.flusher.Flush()
	}
	return nil
}

var (
	_ N.NetPacketConn    = (*serverPacketConn)(nil)
	_ N.FrontHeadroom    = (*serverPacketConn)(nil)
	_ N.PacketReadWaiter = (*serverPacketConn)(nil)
)

type serverPacketConn struct {
	packetConn
}

func (u *serverPacketConn) FrontHeadroom() int {
	return 4 + 16 + 2 + 16 + 2
}

func (u *serverPacketConn) WaitReadPacket() (buffer *buf.Buffer, destination M.Socksaddr, err error) {
	buffer = u.readWaitOptions.NewPacketBuffer()
	destination, err = u.ReadPacket(buffer)
	if err != nil {
		buffer.Release()
		return nil, M.Socksaddr{}, err
	}
	u.readWaitOptions.PostReturn(buffer)
	return buffer, destination, nil
}

func (u *serverPacketConn) ReadPacket(buffer *buf.Buffer) (destination M.Socksaddr, err error) {
	err = u.waitCreated()
	if err != nil {
		return M.Socksaddr{}, err
	}
	return u.readPacketFromClient(buffer)
}

func (u *serverPacketConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	buffer := buf.With(p)
	destination, err := u.ReadPacket(buffer)
	if err != nil {
		return 0, nil, err
	}
	return buffer.Len(), destination.UDPAddr(), nil
}

func (u *serverPacketConn) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	return u.writePacketToClient(buffer, destination)
}

func (u *serverPacketConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	err = u.WritePacket(buf.As(p), M.SocksaddrFromNet(addr))
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func (u *serverPacketConn) readPacketFromClient(buffer *buf.Buffer) (destination M.Socksaddr, err error) {
	header := buf.NewSize(4 + 16 + 2 + 16 + 2 + 1)
	defer header.Release()
	_, err = header.ReadFullFrom(u.body, header.Cap())
	if err != nil {
		return
	}
	length := common.Must1(readUint32(header))
	var sourceAddressBuffer [16]byte
	common.Must1(header.Read(sourceAddressBuffer[:]))
	_ = common.Must1(readUint16(header))
	var destinationAddressBuffer [16]byte
	common.Must1(header.Read(destinationAddressBuffer[:]))
	destination.Addr = parse16BytesIP(destinationAddressBuffer)
	destination.Port = common.Must1(readUint16(header))
	appNameLen := common.Must1(header.ReadByte())
	if appNameLen > 0 {
		err = rw.SkipN(u.body, int(appNameLen))
		if err != nil {
			return M.Socksaddr{}, err
		}
	}
	payloadLen := int(length) - (16 + 2 + 16 + 2 + 1 + int(appNameLen))
	if payloadLen < 0 {
		return M.Socksaddr{}, E.New("invalid udp length: ", length)
	}
	_, err = buffer.ReadFullFrom(u.body, payloadLen)
	return
}

func (u *serverPacketConn) writePacketToClient(buffer *buf.Buffer, source M.Socksaddr) error {
	defer buffer.Release()
	if !source.IsIP() {
		return E.New("only support IP")
	}
	payloadLen := buffer.Len()
	headerLen := 4 + 16 + 2 + 16 + 2
	lengthField := uint32(16 + 2 + 16 + 2 + payloadLen)
	sourceAddress := buildPaddingIP(source.Addr)
	var destinationAddress [16]byte
	var destinationPort uint16
	var (
		header         *buf.Buffer
		headerInBuffer bool
	)
	if buffer.Start() >= headerLen {
		headerBytes := buffer.ExtendHeader(headerLen)
		header = buf.With(headerBytes)
		headerInBuffer = true
	} else {
		header = buf.NewSize(headerLen)
		defer header.Release()
	}
	common.Must(writeUint32(header, lengthField))
	common.Must1(header.Write(sourceAddress[:]))
	common.Must(writeUint16(header, source.Port))
	common.Must1(header.Write(destinationAddress[:]))
	common.Must(writeUint16(header, destinationPort))
	if !headerInBuffer {
		_, err := u.writer.Write(header.Bytes())
		if err != nil {
			return err
		}
	}
	_, err := u.writer.Write(buffer.Bytes())
	if err != nil {
		return err
	}
	if u.flusher != nil {
		u.flusher.Flush()
	}
	return nil
}
