package route

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"net/netip"
	"net/textproto"
	"strings"
	"testing"
	"time"

	mhttp "github.com/metacubex/http"
	"github.com/stretchr/testify/require"

	N "github.com/metacubex/mihomo/common/net"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/log"
	"github.com/metacubex/mihomo/tunnel/statistic"
)

// The poll documents were produced by encoding/json before the hand written
// appends replaced it, so every shape here is compared against what the
// reflection encoder still produces.

func TestAppendTrafficJSONMatchesEncodingJSON(t *testing.T) {
	values := []Traffic{
		{},
		{Up: 1, Down: 2, UpTotal: 3, DownTotal: 4},
		{Up: 1<<63 - 1, Down: -1, UpTotal: 1<<63 - 1, DownTotal: -1 << 63},
	}
	for _, value := range values {
		expected := &bytes.Buffer{}
		require.NoError(t, json.NewEncoder(expected).Encode(value))
		require.Equal(t, expected.String(), string(appendTrafficJSON(nil, value)))
	}
}

func TestAppendMemoryJSONMatchesEncodingJSON(t *testing.T) {
	values := []Memory{
		{},
		{Inuse: 1, OSLimit: 2},
		{Inuse: 1<<64 - 1},
	}
	for _, value := range values {
		expected := &bytes.Buffer{}
		require.NoError(t, json.NewEncoder(expected).Encode(value))
		require.Equal(t, expected.String(), string(appendMemoryJSON(nil, value)))
	}
}

func TestAppendLogJSONMatchesEncodingJSON(t *testing.T) {
	type logDocument struct {
		Type    string `json:"type"`
		Payload string `json:"payload"`
	}
	payloads := []string{
		"", "plain", `quotes"and\backslash`, "tab\tnewline\nreturn\r",
		"<script>&</script>", "control\u0001\u001fchars", "del\u007fchar",
		"separators\u2028and\u2029too", "emoji\U0001F600ok", "latin1\u00e9accent",
		"invalidutf8\xffend", "\xff\xfe\xfd", "trailingtruncated\xe4\xb8",
	}
	for _, payload := range payloads {
		event := log.Event{LogLevel: log.WARNING, Payload: payload}
		expected := &bytes.Buffer{}
		require.NoError(t, json.NewEncoder(expected).Encode(logDocument{
			Type:    event.Type(),
			Payload: payload,
		}))
		require.Equal(t, expected.String(), string(appendLogJSON(nil, event)), payload)
	}
}

func mappedLogLevel(level string) string {
	if level == "warning" {
		return "warn"
	}
	return level
}

func TestAppendLogStructuredJSONMatchesEncodingJSON(t *testing.T) {
	type structuredDocument struct {
		Time    string     `json:"time"`
		Level   string     `json:"level"`
		Message string     `json:"message"`
		Fields  []struct{} `json:"fields"`
	}
	when := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	events := []log.Event{
		{LogLevel: log.INFO, Payload: "hello"},
		{LogLevel: log.WARNING, Payload: "careful<script>&"},
		{LogLevel: log.ERROR, Payload: "failed \"hard\"\u2028"},
	}
	for _, event := range events {
		expected := &bytes.Buffer{}
		require.NoError(t, json.NewEncoder(expected).Encode(structuredDocument{
			Time:    when.Format(time.TimeOnly),
			Level:   mappedLogLevel(event.Type()),
			Message: event.Payload,
			Fields:  []struct{}{},
		}))
		require.Equal(t, expected.String(), string(appendLogStructuredJSON(nil, event, when)), event.Payload)
	}
}

func TestAppendPollJSONAllocatesNothing(t *testing.T) {
	event := log.Event{LogLevel: log.WARNING, Payload: "warm"}
	now := time.Now()
	scratch := make([]byte, 0, 256)
	allocations := testing.AllocsPerRun(100, func() {
		scratch = appendTrafficJSON(scratch[:0], Traffic{Up: 1, Down: 2})
		scratch = appendMemoryJSON(scratch[:0], Memory{Inuse: 3})
		scratch = appendLogJSON(scratch[:0], event)
		scratch = appendLogStructuredJSON(scratch[:0], event, now)
	})
	require.Zero(t, allocations)
}

func TestWsFrameHeaderGolden(t *testing.T) {
	require.Equal(t, 2, wsFrameHeaderLen(0))
	require.Equal(t, 2, wsFrameHeaderLen(125))
	require.Equal(t, 4, wsFrameHeaderLen(126))
	require.Equal(t, 4, wsFrameHeaderLen(65535))
	require.Equal(t, 10, wsFrameHeaderLen(65536))

	var header [wsServerMaxFrameHeader]byte
	require.Equal(t, 2, wsEncodeFrameHeader(header[:], wsOpText, 5))
	require.Equal(t, []byte{0x81, 0x05}, header[:2])
	require.Equal(t, 4, wsEncodeFrameHeader(header[:], wsOpText, 200))
	require.Equal(t, []byte{0x81, 0x7e, 0x00, 0xc8}, header[:4])
	require.Equal(t, 10, wsEncodeFrameHeader(header[:], wsOpText, 70000))
	require.Equal(t, []byte{0x81, 0x7f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x11, 0x70}, header[:10])
}

type countingWriter struct {
	writes int
	buf    bytes.Buffer
}

func (w *countingWriter) Write(b []byte) (int, error) {
	w.writes++
	return w.buf.Write(b)
}

func TestWsWriteServerMessageSingleWrite(t *testing.T) {
	writer := &countingWriter{}
	require.NoError(t, wsWriteServerMessage(writer, wsOpText, []byte("hello")))
	require.Equal(t, 1, writer.writes)
	require.Equal(t, []byte{0x81, 0x05, 'h', 'e', 'l', 'l', 'o'}, writer.buf.Bytes())

	writer.buf.Reset()
	writer.writes = 0
	payload := make([]byte, 200)
	require.NoError(t, wsWriteServerMessage(writer, wsOpText, payload))
	require.Equal(t, 1, writer.writes)
	require.Equal(t, []byte{0x81, 0x7e, 0x00, 0xc8}, writer.buf.Bytes()[:4])
	require.Equal(t, 204, writer.buf.Len())
}

func TestWsWriteServerMessageLargeFrameFallsBack(t *testing.T) {
	writer := &countingWriter{}
	payload := make([]byte, 70000)
	require.NoError(t, wsWriteServerMessage(writer, wsOpText, payload))
	require.Equal(t, 2, writer.writes)
	require.Equal(t, []byte{0x81, 0x7f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x11, 0x70}, writer.buf.Bytes()[:10])
	require.Equal(t, 70010, writer.buf.Len())
}

func TestWsWriteServerMessageAllocatesNothing(t *testing.T) {
	payload := make([]byte, 1350)
	allocations := testing.AllocsPerRun(1000, func() {
		if err := wsWriteServerMessage(io.Discard, wsOpText, payload); err != nil {
			t.Fatal(err)
		}
	})
	require.LessOrEqual(t, allocations, 0.5)
}

// dialWebSocket performs the handshake against the raw listener and returns
// the conn with the reader that holds anything the handshake left buffered.
func dialWebSocket(t *testing.T, addr, path string) (net.Conn, *bufio.Reader) {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	const key = "dGhlIHNhbXBsZSBub25jZQ=="
	request := "GET " + path + " HTTP/1.1\r\nHost: route.test\r\n" +
		"Upgrade: websocket\r\nConnection: Upgrade\r\n" +
		"Sec-WebSocket-Key: " + key + "\r\nSec-WebSocket-Version: 13\r\n\r\n"
	_, err = conn.Write([]byte(request))
	require.NoError(t, err)
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))

	reader := bufio.NewReader(conn)
	statusLine, err := reader.ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "HTTP/1.1 101 Switching Protocols\r\n", statusLine)
	mimeHeader, err := textproto.NewReader(reader).ReadMIMEHeader()
	require.NoError(t, err)
	require.Equal(t, "Upgrade", mimeHeader.Get("Connection"))
	require.Equal(t, "websocket", mimeHeader.Get("Upgrade"))
	require.Equal(t, N.GetWebSocketSecAccept(key), mimeHeader.Get("Sec-WebSocket-Accept"))
	require.NoError(t, conn.SetReadDeadline(time.Time{}))
	return conn, reader
}

func TestWsUpgradeGoldenHandshake(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server := &mhttp.Server{Handler: mhttp.HandlerFunc(func(w mhttp.ResponseWriter, r *mhttp.Request) {
		conn, _, err := wsUpgrade(r, w)
		if err != nil {
			return
		}
		defer conn.Close()
		_ = wsWriteServerText(conn, []byte("ping"))
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })

	_, reader := dialWebSocket(t, listener.Addr().String(), "/")
	var head [2]byte
	_, err = io.ReadFull(reader, head[:])
	require.NoError(t, err)
	require.Equal(t, []byte{0x81, 0x04}, head[:])
	payload := make([]byte, 4)
	_, err = io.ReadFull(reader, payload)
	require.NoError(t, err)
	require.Equal(t, []byte("ping"), payload)
}

type goldenResponseWriter struct {
	header mhttp.Header
	body   bytes.Buffer
	status int
}

func (w *goldenResponseWriter) Header() mhttp.Header        { return w.header }
func (w *goldenResponseWriter) Write(b []byte) (int, error) { return w.body.Write(b) }
func (w *goldenResponseWriter) WriteHeader(status int)      { w.status = status }

func TestWriteConnectionsGoldenBody(t *testing.T) {
	request, err := mhttp.NewRequest(mhttp.MethodGet, "http://route.test/connections", nil)
	require.NoError(t, err)

	recorder := &goldenResponseWriter{header: make(mhttp.Header)}
	writeConnections(recorder, request, &statistic.Snapshot{DownloadTotal: 1, UploadTotal: 2, Memory: 3})
	require.Equal(t, `{"downloadTotal":1,"uploadTotal":2,"connections":[],"memory":3}`+"\n", recorder.body.String())
	require.Equal(t, "application/json", recorder.header.Get("Content-Type"))
	require.Equal(t, 0, recorder.status)

	// The pooled buffer keeps serving larger documents unchanged.
	recorder.body.Reset()
	bigger := &statistic.Snapshot{
		DownloadTotal: 4,
		UploadTotal:   5,
		Memory:        6,
		Connections:   []*statistic.TrackerInfo{nil},
	}
	writeConnections(recorder, request, bigger)
	require.Equal(t, string(bigger.AppendJSON(nil))+"\n", recorder.body.String())
}

// The websocket document is framed with the reserved header in front of the
// buffer, so the announced length must exactly match the document the plain
// response carries.
func TestRouterConnectionsWebsocketFrame(t *testing.T) {
	local, remote := net.Pipe()
	defer remote.Close()

	metadata := &C.Metadata{
		NetWork: C.TCP,
		Type:    C.HTTP,
		SrcIP:   netip.MustParseAddr("10.0.0.2"),
		SrcPort: 40000,
		DstIP:   netip.MustParseAddr("10.0.0.3"),
		DstPort: 443,
		Host:    "connections.example.com",
	}
	tracker := statistic.NewTCPTracker(
		&statisticPipeConn{ExtendedConn: N.NewExtendedConn(local)},
		statistic.DefaultManager,
		metadata,
		nil,
		7,
		9,
		false,
	)
	statistic.DefaultManager.Join(tracker)
	defer func() {
		statistic.DefaultManager.Leave(tracker)
		_ = tracker.Close()
	}()

	base := newTestServer(t, "")
	_, reader := dialWebSocket(t, strings.TrimPrefix(base, "http://"), "/connections?interval=1000")

	var head [2]byte
	_, err := io.ReadFull(reader, head[:])
	require.NoError(t, err)
	require.Equal(t, byte(0x81), head[0])

	var payloadLen int
	switch {
	case head[1] == 126:
		var ext [2]byte
		_, err = io.ReadFull(reader, ext[:])
		require.NoError(t, err)
		payloadLen = int(binary.BigEndian.Uint16(ext[:]))
		require.GreaterOrEqual(t, payloadLen, 126)
	case head[1] == 127:
		var ext [8]byte
		_, err = io.ReadFull(reader, ext[:])
		require.NoError(t, err)
		payloadLen = int(binary.BigEndian.Uint64(ext[:]))
		require.GreaterOrEqual(t, payloadLen, 65536)
	default:
		payloadLen = int(head[1])
		require.Less(t, payloadLen, 126)
	}

	payload := make([]byte, payloadLen)
	_, err = io.ReadFull(reader, payload)
	require.NoError(t, err)

	var document struct {
		DownloadTotal int64 `json:"downloadTotal"`
		Connections   []struct {
			ID string `json:"id"`
		} `json:"connections"`
	}
	require.NoError(t, json.Unmarshal(payload, &document))
	found := false
	for _, connection := range document.Connections {
		if connection.ID == tracker.ID() {
			found = true
		}
	}
	require.True(t, found)
}
