package route

import (
	"strconv"
	"sync"
	"time"

	"github.com/metacubex/mihomo/tunnel/statistic"

	"github.com/metacubex/chi"
	"github.com/metacubex/chi/render"
	"github.com/metacubex/http"
)

// The table is polled on a timer, so the plain responses append it into a
// pooled buffer and the websocket polls build the frame header in front of
// the document instead of paying a second write or a fresh buffer per
// document.

var connectionsBodyPool = sync.Pool{New: func() any { return new([]byte) }}

func writeConnections(w http.ResponseWriter, r *http.Request, body []byte) {
	w.Header().Set("Content-Type", "application/json")
	if status, ok := r.Context().Value(render.StatusCtxKey).(int); ok {
		w.WriteHeader(status)
	}
	// encoding/json's Encoder closed the document with a newline.
	body = append(body, '\n')
	_, _ = w.Write(body)
}

func connectionRouter() http.Handler {
	r := chi.NewRouter()
	r.Get("/", getConnections)
	r.Delete("/", closeAllConnections)
	r.Delete("/{id}", closeConnection)
	return r
}

func getConnections(w http.ResponseWriter, r *http.Request) {
	if !(r.Header.Get("Upgrade") == "websocket") {
		bufp := connectionsBodyPool.Get().(*[]byte)
		body := statistic.DefaultManager.AppendJSON((*bufp)[:0])
		writeConnections(w, r, body)
		*bufp = body
		connectionsBodyPool.Put(bufp)
		return
	}

	conn, _, err := wsUpgrade(r, w)
	if err != nil {
		return
	}

	intervalStr := r.URL.Query().Get("interval")
	interval := 1000
	if intervalStr != "" {
		t, err := strconv.Atoi(intervalStr)
		if err != nil {
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, ErrBadRequest)
			return
		}

		interval = t
	}

	// The initial capacity keeps the reserved frame header inside the slice
	// before the first document grows it.
	scratch := make([]byte, 0, wsServerMaxFrameHeader+512)
	sendSnapshot := func() error {
		// Reserve the worst-case frame header, append the document after it,
		// then move the document over the unused header bytes once its length
		// is known, so the whole frame leaves in one write without a second
		// buffer.
		scratch = statistic.DefaultManager.AppendJSON(scratch[:wsServerMaxFrameHeader])
		n := len(scratch) - wsServerMaxFrameHeader
		headerLen := wsEncodeFrameHeader(scratch[:wsServerMaxFrameHeader], wsOpText, n)
		if headerLen < wsServerMaxFrameHeader {
			copy(scratch[headerLen:], scratch[wsServerMaxFrameHeader:])
		}
		_, err := conn.Write(scratch[:headerLen+n])
		return err
	}

	if err := sendSnapshot(); err != nil {
		return
	}

	tick := time.NewTicker(time.Millisecond * time.Duration(interval))
	defer tick.Stop()
	for range tick.C {
		if err := sendSnapshot(); err != nil {
			break
		}
	}
}

func closeConnection(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if c := statistic.DefaultManager.Get(id); c != nil {
		_ = c.Close()
	}
	render.NoContent(w, r)
}

func closeAllConnections(w http.ResponseWriter, r *http.Request) {
	statistic.DefaultManager.Range(func(c statistic.Tracker) bool {
		_ = c.Close()
		return true
	})
	render.NoContent(w, r)
}
