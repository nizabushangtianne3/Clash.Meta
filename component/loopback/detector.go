package loopback

import (
	"errors"
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"sync"

	"github.com/metacubex/mihomo/component/iface"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/constant/features"
)

var disableLoopBackDetector, _ = strconv.ParseBool(os.Getenv("DISABLE_LOOPBACK_DETECTOR"))

func init() {
	if features.Android {
		disableLoopBackDetector = true
	}
}

var ErrReject = errors.New("reject loopback connection")

type Detector struct {
	connMu        sync.RWMutex
	connMap       map[netip.AddrPort]struct{}
	packetMu      sync.RWMutex
	packetConnMap map[uint16]struct{}
}

func NewDetector() *Detector {
	if disableLoopBackDetector {
		return nil
	}
	return &Detector{
		connMap:       make(map[netip.AddrPort]struct{}),
		packetConnMap: make(map[uint16]struct{}),
	}
}

func (l *Detector) NewConn(conn C.Conn) C.Conn {
	if l == nil {
		return conn
	}
	metadata := C.Metadata{}
	if metadata.SetRemoteAddr(conn.LocalAddr()) != nil {
		return conn
	}
	connAddr := metadata.AddrPort()
	if !connAddr.IsValid() {
		return conn
	}
	l.connMu.Lock()
	l.connMap[connAddr] = struct{}{}
	l.connMu.Unlock()
	return &trackedConn{Conn: conn, detector: l, source: connAddr}
}

type trackedConn struct {
	C.Conn
	detector *Detector
	source   netip.AddrPort
}

func (c *trackedConn) Close() error {
	c.detector.connMu.Lock()
	delete(c.detector.connMap, c.source)
	c.detector.connMu.Unlock()
	return c.Conn.Close()
}

func (l *Detector) NewPacketConn(conn C.PacketConn) C.PacketConn {
	if l == nil {
		return conn
	}
	metadata := C.Metadata{}
	if metadata.SetRemoteAddr(conn.LocalAddr()) != nil {
		return conn
	}
	connAddr := metadata.AddrPort()
	if !connAddr.IsValid() {
		return conn
	}
	port := connAddr.Port()
	l.packetMu.Lock()
	l.packetConnMap[port] = struct{}{}
	l.packetMu.Unlock()
	return &trackedPacketConn{PacketConn: conn, detector: l, source: port}
}

type trackedPacketConn struct {
	C.PacketConn
	detector *Detector
	source   uint16
}

func (c *trackedPacketConn) Close() error {
	c.detector.packetMu.Lock()
	delete(c.detector.packetConnMap, c.source)
	c.detector.packetMu.Unlock()
	return c.PacketConn.Close()
}

func (l *Detector) CheckConn(metadata *C.Metadata) error {
	if l == nil {
		return nil
	}
	connAddr := metadata.SourceAddrPort()
	if !connAddr.IsValid() {
		return nil
	}
	l.connMu.RLock()
	_, blocked := l.connMap[connAddr]
	l.connMu.RUnlock()
	if blocked {
		return fmt.Errorf("%w to: %s", ErrReject, metadata.RemoteAddress())
	}
	return nil
}

func (l *Detector) CheckPacketConn(metadata *C.Metadata) error {
	if l == nil {
		return nil
	}
	connAddr := metadata.SourceAddrPort()
	if !connAddr.IsValid() {
		return nil
	}

	isLocalIp, err := iface.IsLocalIp(connAddr.Addr())
	if err != nil {
		return err
	}
	if !isLocalIp && !connAddr.Addr().IsLoopback() {
		return nil
	}

	l.packetMu.RLock()
	_, blocked := l.packetConnMap[connAddr.Port()]
	l.packetMu.RUnlock()
	if blocked {
		return fmt.Errorf("%w to: %s", ErrReject, metadata.RemoteAddress())
	}
	return nil
}
