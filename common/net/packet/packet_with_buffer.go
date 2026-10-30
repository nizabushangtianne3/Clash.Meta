package packet

import (
	"net"
	"net/netip"

	"github.com/metacubex/mihomo/common/pool"
)

type withBufferPacketConn interface {
	net.PacketConn
	ReadFromWithBuffer(func(sizeHint int) []byte) (int, net.Addr, error)
}

type enhanceWithBufferPacketConn struct {
	withBufferPacketConn
	readBuf         []byte
	getReadBufferFn func(sizeHint int) []byte
}

func newEnhanceWithBufferPacketConn(pc withBufferPacketConn) *enhanceWithBufferPacketConn {
	c := &enhanceWithBufferPacketConn{withBufferPacketConn: pc}
	c.getReadBufferFn = c.getReadBuffer
	return c
}

func (c *enhanceWithBufferPacketConn) getReadBuffer(sizeHint int) []byte {
	c.readBuf = pool.Get(sizeHint)
	return c.readBuf
}

func (c *enhanceWithBufferPacketConn) WaitReadFrom() (data []byte, addr netip.AddrPort, err error) {
	var (
		readN   int
		rawAddr net.Addr
	)
	readN, rawAddr, err = c.ReadFromWithBuffer(c.getReadBufferFn)
	addr = AddrPortFromNetAddr(rawAddr)
	if err != nil && c.readBuf != nil {
		_ = pool.Put(c.readBuf)
		c.readBuf = nil
		return nil, addr, err
	}
	if readN > 0 && c.readBuf != nil {
		data, c.readBuf = c.readBuf[:readN], nil
		return data, addr, nil
	}
	_ = pool.Put(c.readBuf)
	c.readBuf = nil
	return nil, addr, nil
}

func (c *enhanceWithBufferPacketConn) Upstream() any {
	return c.withBufferPacketConn
}

func (c *enhanceWithBufferPacketConn) WriterReplaceable() bool {
	return true
}

func (c *enhanceWithBufferPacketConn) ReaderReplaceable() bool {
	return true
}
