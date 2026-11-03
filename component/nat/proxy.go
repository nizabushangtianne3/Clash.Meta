package nat

import (
	"net/netip"
	"sync/atomic"

	C "github.com/metacubex/mihomo/constant"
)

type WriteBackProxy struct {
	// Every packet of a flow carries a *constant.PacketAdapter, so the concrete
	// type stored here never changes and the per-packet update writes the
	// interface directly instead of boxing a fresh pointer.
	wb atomic.Value
}

func (w *WriteBackProxy) WriteBack(b []byte, addr netip.AddrPort) (n int, err error) {
	return w.wb.Load().(C.WriteBack).WriteBack(b, addr)
}

func (w *WriteBackProxy) UpdateWriteBack(wb C.WriteBack) {
	w.wb.Store(wb)
}

func NewWriteBackProxy(wb C.WriteBack) *WriteBackProxy {
	w := &WriteBackProxy{}
	w.UpdateWriteBack(wb)
	return w
}
