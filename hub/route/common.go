package route

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	N "github.com/metacubex/mihomo/common/net"

	"github.com/metacubex/chi"
	"github.com/metacubex/http"
)

// When name is composed of a partial escape string, Golang does not unescape it
func getEscapeParam(r *http.Request, paramName string) string {
	param := chi.URLParam(r, paramName)
	if newParam, err := url.PathUnescape(param); err == nil {
		param = newParam
	}
	return param
}

// wsUpgrade upgrades http connection to the websocket connection.
//
// It hijacks net.Conn from w and returns received net.Conn and
// bufio.ReadWriter.
func wsUpgrade(r *http.Request, w http.ResponseWriter) (conn net.Conn, rw *bufio.ReadWriter, err error) {
	// See https://tools.ietf.org/html/rfc6455#section-4.1
	// The method of the request MUST be GET, and the HTTP version MUST be at least 1.1.
	var nonce string
	if r.Method != http.MethodGet {
		err = errors.New("handshake error: bad HTTP request method")
		body := err.Error()
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte(body))
		return nil, nil, err
	} else if r.ProtoMajor < 1 || (r.ProtoMajor == 1 && r.ProtoMinor < 1) {
		err = errors.New("handshake error: bad HTTP protocol version")
		body := err.Error()
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusHTTPVersionNotSupported)
		w.Write([]byte(body))
		return nil, nil, err
	} else if r.Host == "" {
		err = errors.New("handshake error: bad Host header")
		body := err.Error()
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(body))
		return nil, nil, err
	} else if u := r.Header.Get("Upgrade"); u != "websocket" && !strings.EqualFold(u, "websocket") {
		err = errors.New("handshake error: bad Upgrade header")
		body := err.Error()
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(body))
		return nil, nil, err
	} else if c := r.Header.Get("Connection"); c != "Upgrade" && !strings.Contains(strings.ToLower(c), "upgrade") {
		err = errors.New("handshake error: bad Connection header")
		body := err.Error()
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(body))
		return nil, nil, err
	} else if nonce = r.Header.Get("Sec-WebSocket-Key"); len(nonce) != 24 {
		err = errors.New("handshake error: bad Sec-WebSocket-Key header")
		body := err.Error()
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(body))
		return nil, nil, err
	} else if v := r.Header.Get("Sec-WebSocket-Version"); v != "13" {
		err = errors.New("handshake error: bad Sec-WebSocket-Version header")
		body := err.Error()
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		if v != "" {
			// According to RFC6455:
			// If this version does not match a version understood by the server, the
			// server MUST abort the WebSocket handshake described in this section and
			// instead send an appropriate HTTP error code (such as 426 Upgrade Required)
			// and a |Sec-WebSocket-Version| header field indicating the version(s) the
			// server is capable of understanding.
			w.Header().Set("Sec-WebSocket-Version", "13")
			w.WriteHeader(http.StatusUpgradeRequired)
		} else {
			w.WriteHeader(http.StatusBadRequest)
		}
		w.Write([]byte(body))
		return nil, nil, err
	}

	conn, rw, err = http.NewResponseController(w).Hijack()
	if err != nil {
		body := err.Error()
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(body))
		return nil, nil, err
	}

	// Clear deadlines set by server.
	conn.SetDeadline(time.Time{})

	rw.Writer.WriteString("HTTP/1.1 101 Switching Protocols\r\n")
	header := http.Header{}
	header.Set("Upgrade", "websocket")
	header.Set("Connection", "Upgrade")
	header.Set("Sec-WebSocket-Accept", N.GetWebSocketSecAccept(nonce))
	header.Write(rw.Writer)
	rw.Writer.WriteString("\r\n")
	err = rw.Writer.Flush()

	return conn, rw, err
}

const (
	wsOpText               = 0x1
	wsServerMaxFrameHeader = 10
	wsMaxPooledFrame       = 65536
)

var wsFramePool = sync.Pool{New: func() any { return new([wsMaxPooledFrame]byte) }}

// wsFrameHeaderLen returns the length of the server frame header for dataLen.
func wsFrameHeaderLen(dataLen int) int {
	switch {
	case dataLen < 126:
		return 2
	case dataLen < 65536:
		return 4
	default:
		return wsServerMaxFrameHeader
	}
}

// wsEncodeFrameHeader writes the FIN+op server frame header for a payload of
// dataLen bytes into dst and returns its length. dst must hold at least
// wsServerMaxFrameHeader bytes.
func wsEncodeFrameHeader(dst []byte, op byte, dataLen int) int {
	dst[0] = 0x80 | op // FIN, no RSV, opcode
	switch {
	case dataLen < 126:
		dst[1] = byte(dataLen)
		return 2
	case dataLen < 65536:
		dst[1] = 126
		binary.BigEndian.PutUint16(dst[2:4], uint16(dataLen))
		return 4
	default:
		dst[1] = 127
		binary.BigEndian.PutUint64(dst[2:wsServerMaxFrameHeader], uint64(dataLen))
		return wsServerMaxFrameHeader
	}
}

// wsWriteServerMessage writes the header and the payload in one write. Small
// frames ride the frame pool; a payload larger than the pool is the rare
// fallback that keeps the payload untouched behind a stack header.
func wsWriteServerMessage(w io.Writer, op byte, p []byte) error {
	headerLen := wsFrameHeaderLen(len(p))
	total := headerLen + len(p)
	if total <= wsMaxPooledFrame {
		frame := wsFramePool.Get().(*[wsMaxPooledFrame]byte)
		buf := frame[:total]
		wsEncodeFrameHeader(buf, op, len(p))
		copy(buf[headerLen:], p)
		_, err := w.Write(buf)
		wsFramePool.Put(frame)
		return err
	}
	var header [wsServerMaxFrameHeader]byte
	headerLen = wsEncodeFrameHeader(header[:], op, len(p))
	if _, err := w.Write(header[:headerLen]); err != nil {
		return err
	}
	_, err := w.Write(p)
	return err
}

// wsWriteServerText is the same as wsWriteServerMessage with ws.OpText.
func wsWriteServerText(w io.Writer, p []byte) error {
	return wsWriteServerMessage(w, wsOpText, p)
}
