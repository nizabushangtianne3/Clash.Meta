package tunnel

import (
	"context"
	"fmt"
	"net"
	"net/netip"

	"github.com/metacubex/mihomo/adapter/inbound"
	N "github.com/metacubex/mihomo/common/net"
	"github.com/metacubex/mihomo/common/pool"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/transport/socks5"
)

type tunnelWriteBackPacketConn interface {
	net.PacketConn
	WriteToUDPAddrPort(b []byte, addr netip.AddrPort) (int, error)
}

type PacketConn struct {
	conn   net.PacketConn
	addr   string
	target socks5.Addr
	proxy  string
}

// RawAddress implements C.Listener
func (l *PacketConn) RawAddress() string {
	return l.addr
}

// Address implements C.Listener
func (l *PacketConn) Address() string {
	return l.conn.LocalAddr().String()
}

// Close implements C.Listener
func (l *PacketConn) Close() error {
	return l.conn.Close()
}

func NewUDP(addr, target, proxy string, lc C.InboundListenConfig, tunnel C.Tunnel, additions ...inbound.Addition) (*PacketConn, error) {
	l, err := lc.ListenPacket(context.Background(), "udp", addr)
	if err != nil {
		return nil, err
	}

	targetAddr := socks5.ParseAddr(target)
	if targetAddr == nil {
		return nil, fmt.Errorf("invalid target address %s", target)
	}

	sl := &PacketConn{
		conn:   l,
		target: targetAddr,
		proxy:  proxy,
		addr:   addr,
	}

	if proxy != "" {
		additions = append([]inbound.Addition{inbound.WithSpecialProxy(proxy)}, additions...)
	}

	conn := N.NewEnhancePacketConn(l)
	go sl.serveUDP(conn, tunnel, additions...)

	return sl, nil
}

func (l *PacketConn) serveUDP(conn N.EnhancePacketConn, tunnel C.Tunnel, additions ...inbound.Addition) {
	for {
		data, remoteAddr, err := conn.WaitReadFrom()
		if err != nil {
			if data != nil {
				_ = pool.Put(data)
			}
			return
		}
		l.handleUDP(conn, tunnel, data, remoteAddr, additions...)
	}
}

func (l *PacketConn) handleUDP(pc net.PacketConn, tunnel C.Tunnel, buf []byte, source netip.AddrPort, additions ...inbound.Addition) {
	cPacket := newTunnelUDPPacket(pc, source, buf)

	tunnel.HandleUDPPacket(inbound.NewPacket(l.target, cPacket, C.TUNNEL, additions...))
}

type tunnelUDPPacket struct {
	pc      net.PacketConn
	source  netip.AddrPort
	payload []byte
}

func newTunnelUDPPacket(pc net.PacketConn, source netip.AddrPort, payload []byte) *tunnelUDPPacket {
	return &tunnelUDPPacket{pc: pc, source: source, payload: payload}
}

func (c *tunnelUDPPacket) Data() []byte {
	return c.payload
}

// FlowKey keeps the packet adapter on the zero-materialisation source path.
func (c *tunnelUDPPacket) FlowKey() netip.AddrPort {
	return c.source
}

func (c *tunnelUDPPacket) WriteBack(b []byte, _ netip.AddrPort) (n int, err error) {
	if conn, ok := c.pc.(tunnelWriteBackPacketConn); ok {
		return conn.WriteToUDPAddrPort(b, c.source)
	}
	return c.pc.WriteTo(b, net.UDPAddrFromAddrPort(c.source))
}

func (c *tunnelUDPPacket) LocalAddr() net.Addr {
	return net.UDPAddrFromAddrPort(c.source)
}

func (c *tunnelUDPPacket) Drop() {
	if c.payload == nil {
		return
	}
	_ = pool.Put(c.payload)
	c.payload = nil
}

func (c *tunnelUDPPacket) InAddr() net.Addr {
	return c.pc.LocalAddr()
}
