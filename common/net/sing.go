package net

import (
	"io"
	"net"

	"github.com/metacubex/mihomo/common/net/deadline"

	"github.com/metacubex/sing/common"
	"github.com/metacubex/sing/common/buf"
	"github.com/metacubex/sing/common/bufio"
	"github.com/metacubex/sing/common/network"
)

var NewExtendedConn = bufio.NewExtendedConn
var NewExtendedWriter = bufio.NewExtendedWriter
var NewExtendedReader = bufio.NewExtendedReader

type ExtendedConn = network.ExtendedConn
type ExtendedWriter = network.ExtendedWriter
type ExtendedReader = network.ExtendedReader

var WriteBuffer = bufio.WriteBuffer

type ReadWaitOptions = network.ReadWaitOptions

var NewReadWaitOptions = network.NewReadWaitOptions
var CalculateFrontHeadroom = network.CalculateFrontHeadroom
var CalculateRearHeadroom = network.CalculateRearHeadroom

type ReaderWithUpstream = network.ReaderWithUpstream
type WithUpstreamReader = network.WithUpstreamReader
type WriterWithUpstream = network.WriterWithUpstream
type WithUpstreamWriter = network.WithUpstreamWriter
type WithUpstream = common.WithUpstream

var UnwrapReader = network.UnwrapReader
var UnwrapWriter = network.UnwrapWriter

func NewDeadlineConn(conn net.Conn) ExtendedConn {
	if deadline.IsPipe(conn) || deadline.IsPipe(UnwrapReader(conn)) {
		return NewExtendedConn(conn) // pipe always have correctly deadline implement
	}
	if deadline.IsConn(conn) || deadline.IsConn(UnwrapReader(conn)) {
		return NewExtendedConn(conn) // was a *deadline.Conn
	}
	return deadline.NewConn(conn)
}

func NeedHandshake(conn any) bool {
	if earlyConn, isEarlyConn := common.Cast[network.EarlyConn](conn); isEarlyConn && earlyConn.NeedHandshake() {
		return true
	}
	return false
}

type CountFunc = network.CountFunc

var Pipe = deadline.Pipe

func closeWrite(writer io.Closer) error {
	if c, ok := common.Cast[network.WriteCloser](writer); ok {
		return c.CloseWrite()
	}
	return writer.Close()
}

// Relay copies between left and right bidirectionally.
// like [bufio.CopyConn] but remove unneeded [context.Context] handle and the cost of [task.Group]
//
// Each direction holds one buffer for the connection's lifetime: the copy loop
// took a fresh 32KB buffer per chunk, so every chunk of a long transfer paid a
// struct allocation and a pool round trip for it.
//
// relayReader re-arms that reused buffer before every read. The readers under
// the copy loop append into the buffer they are handed instead of overwriting
// it, so without a Reset the next chunk would stack behind the previous one.
// The headroom values belong to the destination, and Reset restores the
// capacity Reserve trims, so both can be applied again per chunk.
type relayReader struct {
	ExtendedReader
	frontHeadroom int
	rearHeadroom  int
}

func (r *relayReader) ReadBuffer(buffer *buf.Buffer) error {
	buffer.Reset()
	if r.frontHeadroom > 0 {
		buffer.Resize(r.frontHeadroom, 0)
	}
	if r.rearHeadroom > 0 {
		buffer.Reserve(r.rearHeadroom)
	}
	return r.ExtendedReader.ReadBuffer(buffer)
}

func relayCopy(leftConn, rightConn net.Conn) error {
	source := &relayReader{
		ExtendedReader: NewExtendedReader(rightConn),
		frontHeadroom:  CalculateFrontHeadroom(leftConn),
		rearHeadroom:   CalculateRearHeadroom(leftConn),
	}
	// A cached conn carries bytes that were read off the wire before the relay
	// started, a recorded handshake flight on a fallback path. Its ReadBuffer
	// skips them, so they go first, the way the copy loop's cached branch did.
	if cached, ok := source.ExtendedReader.(*CachedConn); ok {
		if data := cached.ReadCached(); data != nil {
			if _, err := WriteBuffer(NewExtendedWriter(leftConn), data); err != nil {
				return err
			}
		}
	}
	buffer := buf.NewSize(buf.BufferSize)
	_, err := bufio.CopyExtendedBuffer(leftConn, NewExtendedWriter(leftConn), source, buffer, nil, nil)
	if err == nil {
		// A failed write may still hold the buffer, so only a clean finish
		// hands its array back to the pool.
		buffer.Release()
	}
	return err
}

func Relay(leftConn, rightConn net.Conn) {
	defer func() {
		_ = leftConn.Close()
		_ = rightConn.Close()
	}()

	ch := make(chan struct{})
	go func() {
		err := relayCopy(leftConn, rightConn)
		if err == nil {
			_ = closeWrite(leftConn)
		} else {
			_ = leftConn.Close()
		}
		close(ch)
	}()

	err := relayCopy(rightConn, leftConn)
	if err == nil {
		_ = closeWrite(rightConn)
	} else {
		_ = rightConn.Close()
	}
	<-ch
}
