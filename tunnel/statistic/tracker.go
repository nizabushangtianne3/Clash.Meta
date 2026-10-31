package statistic

import (
	"encoding/binary"
	"io"
	"net"
	"net/netip"
	"sync"
	syncatomic "sync/atomic"
	"time"

	"github.com/metacubex/mihomo/common/atomic"
	"github.com/metacubex/mihomo/common/buf"
	N "github.com/metacubex/mihomo/common/net"
	C "github.com/metacubex/mihomo/constant"

	"github.com/gofrs/uuid/v5"
)

var trackerID syncatomic.Uint64

// Tracker IDs are local API handles, not security tokens. A process-local
// counter gives the same uniqueness as CSPRNG UUIDs without one per connection.
func newTrackerID() uuid.UUID {
	var id uuid.UUID
	binary.LittleEndian.PutUint64(id[:8], trackerID.Add(1))
	id[6] = (id[6] & 0x0f) | 0x40
	id[8] = (id[8] & 0x3f) | 0x80
	return id
}

type Tracker interface {
	ID() string
	Close() error
	Info() *TrackerInfo
	C.Connection
}

type TrackerInfo struct {
	UUID          uuid.UUID    `json:"id"`
	metadata      C.Metadata   `json:"-"`
	Metadata      *C.Metadata  `json:"metadata"`
	UploadTotal   atomic.Int64 `json:"upload"`
	DownloadTotal atomic.Int64 `json:"download"`
	Start         time.Time    `json:"start"`
	Chain         C.Chain      `json:"chains"`
	ProviderChain C.Chain      `json:"providerChains"`
	Rule          string       `json:"rule"`
	RulePayload   string       `json:"rulePayload"`

	// The connection view's static prefix and suffix are fixed when the tracker
	// is created; only traffic counters change between polls.
	prefixJSON []byte
	prefixOnce sync.Once
	suffixJSON []byte
	suffixOnce sync.Once
}

type tcpCounterConn struct {
	C.Conn
	tracker *tcpTracker
}

func (c *tcpCounterConn) Read(p []byte) (int, error) {
	return c.tracker.Read(p)
}

func (c *tcpCounterConn) Write(p []byte) (int, error) {
	return c.tracker.Write(p)
}

func (c *tcpCounterConn) ReadBuffer(buffer *buf.Buffer) error {
	return c.tracker.ReadBuffer(buffer)
}

func (c *tcpCounterConn) WriteBuffer(buffer *buf.Buffer) error {
	return c.tracker.WriteBuffer(buffer)
}

func (c *tcpCounterConn) ReaderReplaceable() bool { return false }

func (c *tcpCounterConn) WriterReplaceable() bool { return false }

type tcpTracker struct {
	C.Conn `json:"-"`
	TrackerInfo
	manager *Manager
	stripe  *counterStripe
	counter tcpCounterConn

	pushToManager bool `json:"-"`
	direct        bool `json:"-"`
}

func (tt *tcpTracker) pushUploaded(size int64) {
	if tt.pushToManager {
		tt.stripe.pushUploaded(size, tt.direct)
	}
}

func (tt *tcpTracker) pushDownloaded(size int64) {
	if tt.pushToManager {
		tt.stripe.pushDownloaded(size, tt.direct)
	}
}

func (tt *tcpTracker) ID() string {
	return tt.UUID.String()
}

func (tt *tcpTracker) Info() *TrackerInfo {
	return &tt.TrackerInfo
}

func (tt *tcpTracker) Read(b []byte) (int, error) {
	n, err := tt.Conn.Read(b)
	download := int64(n)
	tt.pushDownloaded(download)
	tt.DownloadTotal.Add(download)
	return n, err
}

func (tt *tcpTracker) ReadBuffer(buffer *buf.Buffer) (err error) {
	err = tt.Conn.ReadBuffer(buffer)
	download := int64(buffer.Len())
	tt.pushDownloaded(download)
	tt.DownloadTotal.Add(download)
	return
}

func (tt *tcpTracker) UnwrapReader() (io.Reader, []N.CountFunc) {
	return &tt.counter, nil
}

func (tt *tcpTracker) Write(b []byte) (int, error) {
	n, err := tt.Conn.Write(b)
	upload := int64(n)
	tt.pushUploaded(upload)
	tt.UploadTotal.Add(upload)
	return n, err
}

func (tt *tcpTracker) WriteBuffer(buffer *buf.Buffer) (err error) {
	upload := int64(buffer.Len())
	err = tt.Conn.WriteBuffer(buffer)
	tt.pushUploaded(upload)
	tt.UploadTotal.Add(upload)
	return
}

func (tt *tcpTracker) UnwrapWriter() (io.Writer, []N.CountFunc) {
	return &tt.counter, nil
}

func (tt *tcpTracker) Close() error {
	tt.manager.Leave(tt)
	return tt.Conn.Close()
}

func (tt *tcpTracker) Upstream() any {
	return tt.Conn
}

func NewTCPTracker(conn C.Conn, manager *Manager, metadata *C.Metadata, rule C.Rule, uploadTotal int64, downloadTotal int64, pushToManager bool) *tcpTracker {
	chains, providerChains := cloneChains(conn.Chains(), conn.ProviderChains())
	tt := &tcpTracker{
		Conn:    conn,
		manager: manager,
		stripe:  manager.bindStripe(),
		TrackerInfo: TrackerInfo{
			UUID:          newTrackerID(),
			Start:         time.Now(),
			Chain:         chains,
			ProviderChain: providerChains,
			Rule:          "",
		},
		pushToManager: pushToManager,
		direct:        IsDirect(conn.Chains().Last()),
	}
	tt.TrackerInfo.UploadTotal.Store(uploadTotal)
	tt.TrackerInfo.DownloadTotal.Store(downloadTotal)
	tt.metadata = *metadata
	tt.Metadata = &tt.metadata
	tt.metadata.RemoteDst = conn.RemoteDestination()
	tt.counter = tcpCounterConn{Conn: conn, tracker: tt}

	if pushToManager {
		if uploadTotal > 0 {
			tt.stripe.pushUploaded(uploadTotal, tt.direct)
		}
		if downloadTotal > 0 {
			tt.stripe.pushDownloaded(downloadTotal, tt.direct)
		}
	}

	if rule != nil {
		tt.TrackerInfo.Rule = rule.RuleType().String()
		tt.TrackerInfo.RulePayload = rule.Payload()
	}

	manager.Join(tt)
	return tt
}

type udpTracker struct {
	C.PacketConn `json:"-"`
	TrackerInfo
	manager *Manager
	stripe  *counterStripe

	pushToManager bool `json:"-"`
	direct        bool `json:"-"`
}

func (ut *udpTracker) pushUploaded(size int64) {
	if ut.pushToManager {
		ut.stripe.pushUploaded(size, ut.direct)
	}
}

func (ut *udpTracker) pushDownloaded(size int64) {
	if ut.pushToManager {
		ut.stripe.pushDownloaded(size, ut.direct)
	}
}

func (ut *udpTracker) ID() string {
	return ut.UUID.String()
}

func (ut *udpTracker) Info() *TrackerInfo {
	return &ut.TrackerInfo
}

func (ut *udpTracker) ReadFrom(b []byte) (int, net.Addr, error) {
	n, addr, err := ut.PacketConn.ReadFrom(b)
	download := int64(n)
	ut.pushDownloaded(download)
	ut.DownloadTotal.Add(download)
	return n, addr, err
}

func (ut *udpTracker) WaitReadFrom() (data []byte, addr netip.AddrPort, err error) {
	data, addr, err = ut.PacketConn.WaitReadFrom()
	download := int64(len(data))
	ut.pushDownloaded(download)
	ut.DownloadTotal.Add(download)
	return
}

func (ut *udpTracker) WriteTo(b []byte, addr net.Addr) (int, error) {
	n, err := ut.PacketConn.WriteTo(b, addr)
	upload := int64(n)
	ut.pushUploaded(upload)
	ut.UploadTotal.Add(upload)
	return n, err
}

func (ut *udpTracker) Close() error {
	ut.manager.Leave(ut)
	return ut.PacketConn.Close()
}

func (ut *udpTracker) Upstream() any {
	return ut.PacketConn
}

func NewUDPTracker(conn C.PacketConn, manager *Manager, metadata *C.Metadata, rule C.Rule, uploadTotal int64, downloadTotal int64, pushToManager bool) *udpTracker {
	chains, providerChains := cloneChains(conn.Chains(), conn.ProviderChains())
	ut := &udpTracker{
		PacketConn: conn,
		manager:    manager,
		stripe:     manager.bindStripe(),
		TrackerInfo: TrackerInfo{
			UUID:          newTrackerID(),
			Start:         time.Now(),
			Chain:         chains,
			ProviderChain: providerChains,
			Rule:          "",
		},
		pushToManager: pushToManager,
		direct:        IsDirect(conn.Chains().Last()),
	}
	ut.TrackerInfo.UploadTotal.Store(uploadTotal)
	ut.TrackerInfo.DownloadTotal.Store(downloadTotal)
	ut.metadata = *metadata
	ut.Metadata = &ut.metadata
	ut.metadata.RemoteDst = conn.RemoteDestination()

	if pushToManager {
		if uploadTotal > 0 {
			ut.stripe.pushUploaded(uploadTotal, ut.direct)
		}
		if downloadTotal > 0 {
			ut.stripe.pushDownloaded(downloadTotal, ut.direct)
		}
	}

	if rule != nil {
		ut.TrackerInfo.Rule = rule.RuleType().String()
		ut.TrackerInfo.RulePayload = rule.Payload()
	}

	manager.Join(ut)
	return ut
}

func cloneChains(chain, providerChain C.Chain) (C.Chain, C.Chain) {
	total := len(chain) + len(providerChain)
	if total == 0 {
		return nil, nil
	}

	combined := make(C.Chain, 0, total)
	combined = append(combined, chain...)
	combined = append(combined, providerChain...)

	return combined[:len(chain):len(chain)], combined[len(chain):total:total]
}
