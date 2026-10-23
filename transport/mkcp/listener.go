package mkcp

import (
	"context"
	"io"
	"net"
	"net/netip"
	"sync"
)

type listenerCore struct {
	cfg            Config
	reader         packetReader
	securityWriter func(ioWriter) packetWriter
	localAddr      net.Addr
	closer         io.Closer
	mu             sync.Mutex
	accept         chan net.Conn
	closed         chan struct{}
	once           sync.Once
}

type ioWriter interface {
	Write([]byte) (int, error)
}

func newListenerCore(cfg Config, localAddr net.Addr, closer io.Closer) (listenerCore, error) {
	security, err := cfg.security()
	if err != nil {
		return listenerCore{}, err
	}
	return listenerCore{
		cfg:    cfg,
		reader: packetReader{security: security, header: cfg.packetHeader()},
		securityWriter: func(w ioWriter) packetWriter {
			return packetWriter{security: security, header: cfg.packetHeader(), writer: w}
		},
		localAddr: localAddr,
		closer:    closer,
		accept:    make(chan net.Conn, 1024),
		closed:    make(chan struct{}),
	}, nil
}

func (l *listenerCore) watch(ctx context.Context, closeListener func() error) {
	select {
	case <-ctx.Done():
		_ = closeListener()
	case <-l.closed:
	}
}

func (l *listenerCore) announce(conn *Conn) bool {
	select {
	case l.accept <- conn:
		return true
	case <-l.closed:
		_ = conn.Close()
		return false
	}
}

func (l *listenerCore) Accept() (net.Conn, error) {
	select {
	case conn := <-l.accept:
		return conn, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

func (l *listenerCore) Addr() net.Addr {
	return l.localAddr
}

func (l *listenerCore) close() bool {
	closed := false
	l.once.Do(func() {
		close(l.closed)
		_ = l.closer.Close()
		closed = true
	})
	return closed
}

func (l *listenerCore) terminate(conns []*Conn) {
	for _, conn := range conns {
		conn.terminate()
	}
}

type udpPacket struct {
	payload []byte
	addr    netip.AddrPort
}

type udpSessionID struct {
	addr netip.AddrPort
	conv uint16
}

type UDPListener struct {
	*listenerCore
	udp      *net.UDPConn
	sessions map[udpSessionID]*Conn
	packets  chan udpPacket
}

func ListenUDP(ctx context.Context, udp *net.UDPConn, cfg Config) (*UDPListener, error) {
	core, err := newListenerCore(cfg, udp.LocalAddr(), udp)
	if err != nil {
		return nil, err
	}
	l := &UDPListener{
		listenerCore: &core,
		udp:          udp,
		sessions:     make(map[udpSessionID]*Conn),
		packets:      make(chan udpPacket, 1024),
	}
	go l.readLoop()
	go l.packetLoop()
	go l.watch(ctx, l.Close)
	return l, nil
}

func (l *UDPListener) readLoop() {
	buf := make([]byte, 64*1024)
	for {
		n, addr, err := l.udp.ReadFromUDPAddrPort(buf)
		if err != nil {
			_ = l.Close()
			return
		}
		payload := append([]byte(nil), buf[:n]...)
		select {
		case l.packets <- udpPacket{payload: payload, addr: addr}:
		case <-l.closed:
			return
		default:
		}
	}
}

func (l *UDPListener) packetLoop() {
	for {
		select {
		case packet := <-l.packets:
			l.onPacket(packet)
		case <-l.closed:
			return
		}
	}
}

func (l *UDPListener) onPacket(packet udpPacket) {
	segments := l.reader.read(packet.payload)
	if len(segments) == 0 {
		return
	}
	addr := netip.AddrPortFrom(packet.addr.Addr().Unmap(), packet.addr.Port())
	conv := segments[0].conversation()
	id := udpSessionID{addr: addr, conv: conv}

	l.mu.Lock()
	if l.sessions == nil {
		l.mu.Unlock()
		return
	}
	conn := l.sessions[id]
	if conn == nil {
		if segments[0].command() == commandTerminate {
			l.mu.Unlock()
			return
		}
		writer := &udpPacketConnWriter{udp: l.udp, addr: addr, remove: func() {
			l.remove(id)
		}}
		remoteAddr := net.UDPAddrFromAddrPort(addr)
		conn = newConn(l.localAddr, remoteAddr, conv, l.securityWriter(writer), writer, l.cfg)
		l.sessions[id] = conn
		if !l.announce(conn) {
			l.mu.Unlock()
			return
		}
	}
	l.mu.Unlock()
	conn.Input(segments)
}

func (l *UDPListener) remove(id udpSessionID) {
	l.mu.Lock()
	delete(l.sessions, id)
	l.mu.Unlock()
}

func (l *UDPListener) Close() error {
	if !l.close() {
		return nil
	}
	l.mu.Lock()
	conns := make([]*Conn, 0, len(l.sessions))
	for _, conn := range l.sessions {
		conns = append(conns, conn)
	}
	l.sessions = nil
	l.mu.Unlock()
	l.terminate(conns)
	return nil
}

type udpPacketConnWriter struct {
	udp    *net.UDPConn
	addr   netip.AddrPort
	remove func()
}

func (w *udpPacketConnWriter) Write(b []byte) (int, error) {
	return w.udp.WriteToUDPAddrPort(b, w.addr)
}

func (w *udpPacketConnWriter) Close() error {
	w.remove()
	return nil
}

type virtualPacket struct {
	payload []byte
	addr    net.Addr
}

type virtualSessionID struct {
	addr net.Addr
	conv uint16
}

type VirtualListener struct {
	*listenerCore
	pc       net.PacketConn
	sessions map[virtualSessionID]*Conn
	packets  chan virtualPacket
}

func ListenVirtual(ctx context.Context, pc net.PacketConn, cfg Config) (*VirtualListener, error) {
	core, err := newListenerCore(cfg, pc.LocalAddr(), pc)
	if err != nil {
		return nil, err
	}
	l := &VirtualListener{
		listenerCore: &core,
		pc:           pc,
		sessions:     make(map[virtualSessionID]*Conn),
		packets:      make(chan virtualPacket, 1024),
	}
	go l.readLoop()
	go l.packetLoop()
	go l.watch(ctx, l.Close)
	return l, nil
}

func (l *VirtualListener) readLoop() {
	buf := make([]byte, 64*1024)
	for {
		n, addr, err := l.pc.ReadFrom(buf)
		if err != nil {
			_ = l.Close()
			return
		}
		payload := append([]byte(nil), buf[:n]...)
		select {
		case l.packets <- virtualPacket{payload: payload, addr: addr}:
		case <-l.closed:
			return
		default:
		}
	}
}

func (l *VirtualListener) packetLoop() {
	for {
		select {
		case packet := <-l.packets:
			l.onPacket(packet)
		case <-l.closed:
			return
		}
	}
}

func (l *VirtualListener) onPacket(packet virtualPacket) {
	segments := l.reader.read(packet.payload)
	if len(segments) == 0 {
		return
	}
	conv := segments[0].conversation()
	id := virtualSessionID{addr: packet.addr, conv: conv}

	l.mu.Lock()
	if l.sessions == nil {
		l.mu.Unlock()
		return
	}
	conn := l.sessions[id]
	if conn == nil {
		if segments[0].command() == commandTerminate {
			l.mu.Unlock()
			return
		}
		writer := &virtualPacketConnWriter{pc: l.pc, addr: packet.addr, remove: func() {
			l.remove(id)
		}}
		conn = newConn(l.localAddr, packet.addr, conv, l.securityWriter(writer), writer, l.cfg)
		l.sessions[id] = conn
		if !l.announce(conn) {
			l.mu.Unlock()
			return
		}
	}
	l.mu.Unlock()
	conn.Input(segments)
}

func (l *VirtualListener) remove(id virtualSessionID) {
	l.mu.Lock()
	delete(l.sessions, id)
	l.mu.Unlock()
}

func (l *VirtualListener) Close() error {
	if !l.close() {
		return nil
	}
	l.mu.Lock()
	conns := make([]*Conn, 0, len(l.sessions))
	for _, conn := range l.sessions {
		conns = append(conns, conn)
	}
	l.sessions = nil
	l.mu.Unlock()
	l.terminate(conns)
	return nil
}

type virtualPacketConnWriter struct {
	pc     net.PacketConn
	addr   net.Addr
	remove func()
}

func (w *virtualPacketConnWriter) Write(b []byte) (int, error) {
	return w.pc.WriteTo(b, w.addr)
}

func (w *virtualPacketConnWriter) Close() error {
	w.remove()
	return nil
}

var (
	_ net.Listener = (*UDPListener)(nil)
	_ net.Listener = (*VirtualListener)(nil)
	_ net.Conn     = (*Conn)(nil)
)
