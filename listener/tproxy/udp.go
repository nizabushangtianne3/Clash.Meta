package tproxy

import (
	"net"
	"net/netip"

	"github.com/metacubex/mihomo/adapter/inbound"
	"github.com/metacubex/mihomo/common/lru"
	"github.com/metacubex/mihomo/common/pool"
	C "github.com/metacubex/mihomo/constant"
)

type UDPListener struct {
	packetConn net.PacketConn
	addr       string
	closed     bool
	additions  []inbound.Addition
	flowCache  *lru.LruCache[netip.AddrPort, *tproxyUDPFlow]
}

// A client flow repeats its destination, so the metadata parsed once from the
// original destination is cached per source address and reused while that
// destination matches; the reply reader shares the cache with the same keys.
const (
	tproxyUDPFlowCacheSize = 1024
	tproxyUDPFlowCacheAge  = 300
)

type tproxyUDPFlow struct {
	metadata *C.Metadata
	rAddr    netip.AddrPort
}

// RawAddress implements C.Listener
func (l *UDPListener) RawAddress() string {
	return l.addr
}

// Address implements C.Listener
func (l *UDPListener) Address() string {
	return l.packetConn.LocalAddr().String()
}

// Close implements C.Listener
func (l *UDPListener) Close() error {
	l.closed = true
	return l.packetConn.Close()
}

func NewUDP(addr string, tunnel C.Tunnel, additions ...inbound.Addition) (*UDPListener, error) {
	if len(additions) == 0 {
		additions = []inbound.Addition{
			inbound.WithInName("DEFAULT-TPROXY"),
			inbound.WithSpecialRules(""),
		}
	}
	l, err := net.ListenPacket("udp", addr)
	if err != nil {
		return nil, err
	}

	rl := &UDPListener{
		packetConn: l,
		addr:       addr,
		additions:  additions,
		flowCache: lru.New(
			lru.WithSize[netip.AddrPort, *tproxyUDPFlow](tproxyUDPFlowCacheSize),
			lru.WithAge[netip.AddrPort, *tproxyUDPFlow](tproxyUDPFlowCacheAge),
		),
	}

	c := l.(*net.UDPConn)

	rc, err := c.SyscallConn()
	if err != nil {
		return nil, err
	}

	err = setsockopt(rc, addr)
	if err != nil {
		return nil, err
	}

	go func() {
		oob := make([]byte, 1024)
		for {
			buf := pool.Get(pool.UDPBufferSize)
			n, oobn, _, lAddr, err := c.ReadMsgUDPAddrPort(buf, oob)
			if err != nil {
				pool.Put(buf)
				if rl.closed {
					break
				}
				continue
			}

			rAddr, err := getOrigDst(oob[:oobn])
			if err != nil {
				pool.Put(buf)
				continue
			}

			dscp, _ := getDSCP(oob[:oobn])
			rl.handlePacketConn(l, tunnel, buf[:n], lAddr, rAddr, dscp)
		}
	}()

	return rl, nil
}

func (l *UDPListener) handlePacketConn(pc net.PacketConn, tunnel C.Tunnel, buf []byte, lAddr, rAddr netip.AddrPort, dscp uint8) {
	if rAddr.Addr().Is4() {
		// try to unmap 4in6 address
		lAddr = netip.AddrPortFrom(lAddr.Addr().Unmap(), lAddr.Port())
	}
	pkt := newTproxyPacket(pc, lAddr, buf, tunnel, l)
	flow, hit := l.flowCache.Get(lAddr)
	if !hit || flow.rAddr != rAddr {
		_, metadata := inbound.NewPacketFromAddrPort(rAddr, pkt, C.TPROXY, l.additions...)
		metadata.DSCP = dscp
		l.flowCache.Set(lAddr, &tproxyUDPFlow{metadata: metadata, rAddr: rAddr})
		tunnel.HandleUDPPacket(pkt, metadata)
		return
	}
	tunnel.HandleUDPPacket(pkt, flow.metadata)
}
