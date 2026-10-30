package v4

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/netip"
	"strconv"

	"github.com/metacubex/blake3"
	"github.com/metacubex/quic-go"

	N "github.com/metacubex/mihomo/common/net"
	"github.com/metacubex/mihomo/common/pool"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/transport/socks5"
)

type BufferedReader interface {
	io.Reader
	io.ByteReader
}

type BufferedWriter interface {
	io.Writer
	io.ByteWriter
}

// writeUint16 writes a big-endian uint16 through WriteByte. Handing the writer
// a scratch slice would escape through the interface and allocate per packet.
func writeUint16(writer BufferedWriter, v uint16) error {
	if err := writer.WriteByte(byte(v >> 8)); err != nil {
		return err
	}
	return writer.WriteByte(byte(v))
}

// readUint16 is writeUint16 for the read side.
func readUint16(reader BufferedReader) (uint16, error) {
	hi, err := reader.ReadByte()
	if err != nil {
		return 0, err
	}
	lo, err := reader.ReadByte()
	if err != nil {
		return 0, err
	}
	return uint16(hi)<<8 | uint16(lo), nil
}

// writeUint32 is writeUint16 for the four-byte association id.
func writeUint32(writer BufferedWriter, v uint32) error {
	for shift := 24; shift >= 0; shift -= 8 {
		if err := writer.WriteByte(byte(v >> shift)); err != nil {
			return err
		}
	}
	return nil
}

// readUint32 is writeUint32 for the read side.
func readUint32(reader BufferedReader) (uint32, error) {
	var v uint32
	for range 4 {
		b, err := reader.ReadByte()
		if err != nil {
			return 0, err
		}
		v = v<<8 | uint32(b)
	}
	return v, nil
}

type CommandType byte

const (
	AuthenticateType = CommandType(0x00)
	ConnectType      = CommandType(0x01)
	PacketType       = CommandType(0x02)
	DissociateType   = CommandType(0x03)
	HeartbeatType    = CommandType(0x04)
	ResponseType     = CommandType(0xff)
)

const VER byte = 0x04

func (c CommandType) String() string {
	switch c {
	case AuthenticateType:
		return "Authenticate"
	case ConnectType:
		return "Connect"
	case PacketType:
		return "Packet"
	case DissociateType:
		return "Dissociate"
	case HeartbeatType:
		return "Heartbeat"
	case ResponseType:
		return "Response"
	default:
		return fmt.Sprintf("UnknowCommand: %#x", byte(c))
	}
}

func (c CommandType) BytesLen() int {
	return 1
}

type CommandHead struct {
	VER  byte
	TYPE CommandType
}

func NewCommandHead(TYPE CommandType) CommandHead {
	return CommandHead{
		VER:  VER,
		TYPE: TYPE,
	}
}

func ReadCommandHead(reader BufferedReader) (c CommandHead, err error) {
	c.VER, err = reader.ReadByte()
	if err != nil {
		return
	}
	TYPE, err := reader.ReadByte()
	if err != nil {
		return
	}
	c.TYPE = CommandType(TYPE)
	return
}

func (c CommandHead) WriteTo(writer BufferedWriter) (err error) {
	err = writer.WriteByte(c.VER)
	if err != nil {
		return
	}
	err = writer.WriteByte(byte(c.TYPE))
	if err != nil {
		return
	}
	return
}

func (c CommandHead) BytesLen() int {
	return 1 + c.TYPE.BytesLen()
}

type Authenticate struct {
	CommandHead
	TKN [32]byte
}

func NewAuthenticate(TKN [32]byte) Authenticate {
	return Authenticate{
		CommandHead: NewCommandHead(AuthenticateType),
		TKN:         TKN,
	}
}

func ReadAuthenticateWithHead(head CommandHead, reader BufferedReader) (c Authenticate, err error) {
	c.CommandHead = head
	if c.CommandHead.TYPE != AuthenticateType {
		err = fmt.Errorf("error command type: %s", c.CommandHead.TYPE)
		return
	}
	_, err = io.ReadFull(reader, c.TKN[:])
	if err != nil {
		return
	}
	return
}

func ReadAuthenticate(reader BufferedReader) (c Authenticate, err error) {
	head, err := ReadCommandHead(reader)
	if err != nil {
		return
	}
	return ReadAuthenticateWithHead(head, reader)
}

func GenTKN(token string) [32]byte {
	return blake3.Sum256([]byte(token))
}

func (c Authenticate) WriteTo(writer BufferedWriter) (err error) {
	err = c.CommandHead.WriteTo(writer)
	if err != nil {
		return
	}
	_, err = writer.Write(c.TKN[:])
	if err != nil {
		return
	}
	return
}

func (c Authenticate) BytesLen() int {
	return c.CommandHead.BytesLen() + 32
}

type Connect struct {
	CommandHead
	ADDR Address
}

func NewConnect(ADDR Address) Connect {
	return Connect{
		CommandHead: NewCommandHead(ConnectType),
		ADDR:        ADDR,
	}
}

func ReadConnectWithHead(head CommandHead, reader BufferedReader) (c Connect, err error) {
	c.CommandHead = head
	if c.CommandHead.TYPE != ConnectType {
		err = fmt.Errorf("error command type: %s", c.CommandHead.TYPE)
		return
	}
	c.ADDR, err = ReadAddress(reader)
	if err != nil {
		return
	}
	return
}

func ReadConnect(reader BufferedReader) (c Connect, err error) {
	head, err := ReadCommandHead(reader)
	if err != nil {
		return
	}
	return ReadConnectWithHead(head, reader)
}

func (c Connect) WriteTo(writer BufferedWriter) (err error) {
	err = c.CommandHead.WriteTo(writer)
	if err != nil {
		return
	}
	err = c.ADDR.WriteTo(writer)
	if err != nil {
		return
	}
	return
}

func (c Connect) BytesLen() int {
	return c.CommandHead.BytesLen() + c.ADDR.BytesLen()
}

type Packet struct {
	CommandHead
	ASSOC_ID uint32
	LEN      uint16
	ADDR     Address
	DATA     []byte
}

func NewPacket(ASSOC_ID uint32, LEN uint16, ADDR Address, DATA []byte) Packet {
	return Packet{
		CommandHead: NewCommandHead(PacketType),
		ASSOC_ID:    ASSOC_ID,
		LEN:         LEN,
		ADDR:        ADDR,
		DATA:        DATA,
	}
}

func ReadPacketWithHead(head CommandHead, reader BufferedReader) (c Packet, err error) {
	return readPacketWithHead(head, reader)
}

func readPacketInto(reader BufferedReader) (c Packet, err error) {
	head, err := ReadCommandHead(reader)
	if err != nil {
		return
	}
	return readPacketWithHead(head, reader)
}

func readPacketWithHead(head CommandHead, reader BufferedReader) (c Packet, err error) {
	c.CommandHead = head
	if c.CommandHead.TYPE != PacketType {
		err = fmt.Errorf("error command type: %s", c.CommandHead.TYPE)
		return
	}
	if c.ASSOC_ID, err = readUint32(reader); err != nil {
		return
	}
	if c.LEN, err = readUint16(reader); err != nil {
		return
	}
	c.ADDR, err = ReadAddress(reader)
	if err != nil {
		return
	}
	// one owner: the reader hands this buffer up and returns it to the pool
	// once the datagram is written out or dropped
	c.DATA = pool.Get(int(c.LEN))
	_, err = io.ReadFull(reader, c.DATA)
	if err != nil {
		return
	}
	return
}

func ReadPacket(reader BufferedReader) (c Packet, err error) {
	return readPacketInto(reader)
}

func (c Packet) WriteTo(writer BufferedWriter) (err error) {
	err = c.CommandHead.WriteTo(writer)
	if err != nil {
		return
	}
	if err = writeUint32(writer, c.ASSOC_ID); err != nil {
		return
	}
	if err = writeUint16(writer, c.LEN); err != nil {
		return
	}
	err = c.ADDR.WriteTo(writer)
	if err != nil {
		return
	}
	_, err = writer.Write(c.DATA)
	if err != nil {
		return
	}
	return
}

func (c Packet) BytesLen() int {
	return c.CommandHead.BytesLen() + 4 + 2 + c.ADDR.BytesLen() + len(c.DATA)
}

var PacketOverHead = NewPacket(0, 0, NewAddressAddrPort(netip.AddrPortFrom(netip.IPv6Unspecified(), 0)), nil).BytesLen()

type Dissociate struct {
	CommandHead
	ASSOC_ID uint32
}

func NewDissociate(ASSOC_ID uint32) Dissociate {
	return Dissociate{
		CommandHead: NewCommandHead(DissociateType),
		ASSOC_ID:    ASSOC_ID,
	}
}

func ReadDissociateWithHead(head CommandHead, reader BufferedReader) (c Dissociate, err error) {
	c.CommandHead = head
	if c.CommandHead.TYPE != DissociateType {
		err = fmt.Errorf("error command type: %s", c.CommandHead.TYPE)
		return
	}
	if c.ASSOC_ID, err = readUint32(reader); err != nil {
		return
	}
	return
}

func ReadDissociate(reader BufferedReader) (c Dissociate, err error) {
	head, err := ReadCommandHead(reader)
	if err != nil {
		return
	}
	return ReadDissociateWithHead(head, reader)
}

func (c Dissociate) WriteTo(writer BufferedWriter) (err error) {
	err = c.CommandHead.WriteTo(writer)
	if err != nil {
		return
	}
	if err = writeUint32(writer, c.ASSOC_ID); err != nil {
		return
	}
	return
}

func (c Dissociate) BytesLen() int {
	return c.CommandHead.BytesLen() + 4
}

type Heartbeat struct {
	CommandHead
}

func NewHeartbeat() Heartbeat {
	return Heartbeat{
		CommandHead: NewCommandHead(HeartbeatType),
	}
}

func ReadHeartbeatWithHead(head CommandHead, reader BufferedReader) (c Heartbeat, err error) {
	c.CommandHead = head
	if c.CommandHead.TYPE != HeartbeatType {
		err = fmt.Errorf("error command type: %s", c.CommandHead.TYPE)
		return
	}
	return
}

func ReadHeartbeat(reader BufferedReader) (c Heartbeat, err error) {
	head, err := ReadCommandHead(reader)
	if err != nil {
		return
	}
	return ReadHeartbeatWithHead(head, reader)
}

type Response struct {
	CommandHead
	REP byte
}

func NewResponse(REP byte) Response {
	return Response{
		CommandHead: NewCommandHead(ResponseType),
		REP:         REP,
	}
}

func NewResponseSucceed() Response {
	return NewResponse(0x00)
}

func NewResponseFailed() Response {
	return NewResponse(0xff)
}

func ReadResponseWithHead(head CommandHead, reader BufferedReader) (c Response, err error) {
	c.CommandHead = head
	if c.CommandHead.TYPE != ResponseType {
		err = fmt.Errorf("error command type: %s", c.CommandHead.TYPE)
		return
	}
	c.REP, err = reader.ReadByte()
	if err != nil {
		return
	}
	return
}

func ReadResponse(reader BufferedReader) (c Response, err error) {
	head, err := ReadCommandHead(reader)
	if err != nil {
		return
	}
	return ReadResponseWithHead(head, reader)
}

func (c Response) WriteTo(writer BufferedWriter) (err error) {
	err = c.CommandHead.WriteTo(writer)
	if err != nil {
		return
	}
	err = writer.WriteByte(c.REP)
	if err != nil {
		return
	}
	return
}

func (c Response) IsSucceed() bool {
	return c.REP == 0x00
}

func (c Response) IsFailed() bool {
	return c.REP == 0xff
}

func (c Response) BytesLen() int {
	return c.CommandHead.BytesLen() + 1
}

// Addr types
const (
	AtypDomainName byte = 0
	AtypIPv4       byte = 1
	AtypIPv6       byte = 2
)

type Address struct {
	typ    byte
	ip     netip.Addr
	domain []byte // length-prefixed on the wire
	port   uint16
}

func NewAddress(metadata *C.Metadata) Address {
	addr := Address{port: metadata.DstPort}
	switch metadata.AddrType() {
	case C.AtypIPv4:
		addr.typ = AtypIPv4
		addr.ip = metadata.DstIP.Unmap()
	case C.AtypIPv6:
		addr.typ = AtypIPv6
		addr.ip = metadata.DstIP
	case C.AtypDomainName:
		addr.typ = AtypDomainName
		addr.domain = make([]byte, len(metadata.Host)+1)
		addr.domain[0] = byte(len(metadata.Host))
		copy(addr.domain[1:], metadata.Host)
	}
	return addr
}

func NewAddressNetAddr(addr net.Addr) (Address, error) {
	if addr, ok := addr.(interface{ AddrPort() netip.AddrPort }); ok {
		if addrPort := addr.AddrPort(); addrPort.IsValid() { // sing's M.Socksaddr may return an invalid AddrPort for a domain
			return NewAddressAddrPort(addrPort), nil
		}
	}
	addrStr := addr.String()
	if addrPort, err := netip.ParseAddrPort(addrStr); err == nil {
		return NewAddressAddrPort(addrPort), nil
	}
	metadata := &C.Metadata{}
	if err := metadata.SetRemoteAddress(addrStr); err != nil {
		return Address{}, err
	}
	return NewAddress(metadata), nil
}

func NewAddressAddrPort(addrPort netip.AddrPort) Address {
	addr := addrPort.Addr().Unmap()
	typ := AtypIPv6
	if addr.Is4() {
		typ = AtypIPv4
	}
	return Address{typ: typ, ip: addr, port: addrPort.Port()}
}

func readFixedAddress(reader BufferedReader, dst []byte) (err error) {
	switch src := reader.(type) {
	case *bufio.Reader:
		var wire []byte
		wire, err = src.Peek(len(dst))
		if err != nil {
			return
		}
		copy(dst, wire)
		_, err = src.Discard(len(dst))
	case *N.BufferedConn:
		var wire []byte
		wire, err = src.Peek(len(dst))
		if err != nil {
			return
		}
		copy(dst, wire)
		_, err = src.Discard(len(dst))
	default:
		for i := range dst {
			if dst[i], err = reader.ReadByte(); err != nil {
				return
			}
		}
	}
	return
}

func ReadAddress(reader BufferedReader) (c Address, err error) {
	c.typ, err = reader.ReadByte()
	if err != nil {
		return
	}
	switch c.typ {
	case AtypIPv4:
		var ip [4]byte
		if err = readFixedAddress(reader, ip[:]); err != nil {
			return
		}
		c.ip = netip.AddrFrom4(ip)
	case AtypIPv6:
		var ip [16]byte
		if err = readFixedAddress(reader, ip[:]); err != nil {
			return
		}
		c.ip = netip.AddrFrom16(ip)
	case AtypDomainName:
		var addrLen byte
		if addrLen, err = reader.ReadByte(); err != nil {
			return
		}
		c.domain = make([]byte, int(addrLen)+1)
		c.domain[0] = addrLen
		if _, err = io.ReadFull(reader, c.domain[1:]); err != nil {
			return
		}
	default:
		err = fmt.Errorf("unknown address type: %#x", c.typ)
		return
	}
	c.port, err = readUint16(reader)
	return
}

func (a Address) WriteTo(writer BufferedWriter) (err error) {
	if err = writer.WriteByte(a.typ); err != nil {
		return
	}
	if a.typ == AtypDomainName {
		_, err = writer.Write(a.domain)
	} else if buffer, ok := writer.(*bytes.Buffer); ok {
		// The concrete path keeps fixed-width address arrays on the stack.
		if a.ip.Is4() {
			ip4 := a.ip.As4()
			_, err = buffer.Write(ip4[:])
		} else {
			ip16 := a.ip.As16()
			_, err = buffer.Write(ip16[:])
		}
	} else if a.ip.Is4() {
		for _, b := range a.ip.As4() {
			if err = writer.WriteByte(b); err != nil {
				return
			}
		}
	} else {
		for _, b := range a.ip.As16() {
			if err = writer.WriteByte(b); err != nil {
				return
			}
		}
	}
	return writeUint16(writer, a.port)
}

func (a Address) String() string {
	if a.typ == AtypDomainName {
		return net.JoinHostPort(string(a.domain[1:]), strconv.Itoa(int(a.port)))
	}
	return netip.AddrPortFrom(a.ip.Unmap(), a.port).String()
}

func (a Address) SocksAddr() socks5.Addr {
	var body []byte
	if a.typ == AtypDomainName {
		body = a.domain
	} else if a.ip.Is4() {
		ip4 := a.ip.As4()
		body = ip4[:]
	} else {
		ip16 := a.ip.As16()
		body = ip16[:]
	}
	addr := make([]byte, 1+len(body)+2)
	switch a.typ {
	case AtypIPv4:
		addr[0] = socks5.AtypIPv4
	case AtypIPv6:
		addr[0] = socks5.AtypIPv6
	case AtypDomainName:
		addr[0] = socks5.AtypDomainName
	}
	copy(addr[1:], body)
	binary.BigEndian.PutUint16(addr[len(addr)-2:], a.port)
	return addr
}

func (a Address) UDPAddr() *net.UDPAddr {
	return &net.UDPAddr{IP: a.ip.AsSlice(), Port: int(a.port), Zone: ""}
}

func (a Address) AddrPort() netip.AddrPort {
	switch a.typ {
	case AtypIPv4, AtypIPv6:
		return netip.AddrPortFrom(a.ip.Unmap(), a.port)
	default:
		return netip.AddrPort{}
	}
}

func (a Address) BytesLen() int {
	if a.typ == AtypDomainName {
		return 1 + len(a.domain) + 2
	}
	return 1 + a.ip.BitLen()/8 + 2
}

const (
	ProtocolError         = quic.ApplicationErrorCode(0xfffffff0)
	AuthenticationFailed  = quic.ApplicationErrorCode(0xfffffff1)
	AuthenticationTimeout = quic.ApplicationErrorCode(0xfffffff2)
	BadCommand            = quic.ApplicationErrorCode(0xfffffff3)
)
