package route

import (
	"strconv"
	"time"

	"github.com/metacubex/mihomo/tunnel/statistic"

	"github.com/metacubex/chi"
	"github.com/metacubex/chi/render"
	"github.com/metacubex/http"
)

// The table is polled on a timer, so the writers append it straight into a
// buffer they reuse instead of paying encoding/json for a document that is
// read once and dropped.

func writeConnections(w http.ResponseWriter, r *http.Request, snapshot *statistic.Snapshot) {
	w.Header().Set("Content-Type", "application/json")
	if status, ok := r.Context().Value(render.StatusCtxKey).(int); ok {
		w.WriteHeader(status)
	}
	// One right sized buffer per request: an address, a uuid, a chain and a
	// handful of names fit in a few hundred bytes per connection.
	body := snapshot.AppendJSON(make([]byte, 0, 256+384*len(snapshot.Connections)))
	// encoding/json's Encoder closed the document with a newline.
	_, _ = w.Write(append(body, '\n'))
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
		snapshot := statistic.DefaultManager.Snapshot()
		writeConnections(w, r, snapshot)
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

	var scratch []byte
	sendSnapshot := func() error {
		snapshot := statistic.DefaultManager.Snapshot()
		scratch = snapshot.AppendJSON(scratch[:0])
		return wsWriteServerText(conn, scratch)
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
