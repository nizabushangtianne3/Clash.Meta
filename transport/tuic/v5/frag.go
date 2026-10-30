package v5

import (
	"bytes"
	"sync"

	"github.com/metacubex/mihomo/common/pool"

	"github.com/metacubex/quic-go"
)

// MaxFragSize is a safe udp relay packet size
// because tuicv5 support udp fragment so we unneeded to do a magic modify for quic-go to increase MaxDatagramFrameSize
// it may not work fine in some platform
// "1200" from quic-go's MaxDatagramSize
// "-3" from quic-go's DatagramFrame.MaxDataLen
var MaxFragSize = 1200 - PacketOverHead - 3

func fragWriteNative(quicConn *quic.Conn, packet Packet, buf *bytes.Buffer, fragSize int) (err error) {
	fullPayload := packet.DATA
	off := 0
	fragID := uint8(0)
	fragCount := uint8((len(fullPayload) + fragSize - 1) / fragSize) // round up
	packet.FRAG_TOTAL = fragCount
	for off < len(fullPayload) {
		payloadSize := len(fullPayload) - off
		if payloadSize > fragSize {
			payloadSize = fragSize
		}
		frag := packet
		frag.FRAG_ID = fragID
		frag.SIZE = uint16(payloadSize)
		frag.DATA = fullPayload[off : off+payloadSize]
		off += payloadSize
		fragID++
		buf.Reset()
		err = frag.WriteTo(buf)
		if err != nil {
			return
		}
		data := buf.Bytes()
		err = quicConn.SendDatagram(data)
		if err != nil {
			return
		}
		packet.ADDR.TYPE = AtypNone // avoid "fragment 2/2: address in non-first fragment"
	}
	return
}

type deFragger struct {
	bags  map[uint16]*packetBag
	next  uint64
	mutex sync.Mutex
}

const maxPartialMessages = 64

type packetBag struct {
	frags    []Packet
	filled   [4]uint64
	count    uint8
	lastSeen uint64
	mutex    sync.Mutex
}

func newPacketBag() *packetBag { return new(packetBag) }

func (b *packetBag) reset() {
	for i := range b.frags {
		_ = pool.Put(b.frags[i].DATA)
		b.frags[i] = Packet{}
	}
	b.filled = [4]uint64{}
	b.count = 0
}

func (b *packetBag) reserve(size uint8) {
	clear(b.frags)
	if cap(b.frags) < int(size) {
		b.frags = make([]Packet, size)
		return
	}
	b.frags = b.frags[:size]
}

func (b *packetBag) assemble() Packet {
	size := 0
	for _, frag := range b.frags {
		size += len(frag.DATA)
	}

	data := pool.Get(size)
	offset := 0
	for _, frag := range b.frags {
		offset += copy(data[offset:], frag.DATA)
	}

	packet := b.frags[0]
	packet.SIZE = uint16(size)
	packet.DATA = data[:size]
	packet.FRAG_ID = 0
	packet.FRAG_TOTAL = 1
	return packet
}

func (d *deFragger) bagFor(m *Packet) *packetBag {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	if d.bags == nil {
		d.bags = make(map[uint16]*packetBag)
	}
	bag, ok := d.bags[m.PKT_ID]
	if ok {
		d.touch(bag)
		return bag
	}
	if len(d.bags) >= maxPartialMessages {
		d.evictOldest()
	}
	bag = newPacketBag()
	bag.lastSeen = d.next
	d.next++
	d.bags[m.PKT_ID] = bag
	return bag
}

func (d *deFragger) touch(bag *packetBag) {
	bag.lastSeen = d.next
	d.next++
}

func (d *deFragger) evictOldest() {
	var oldest *packetBag
	var oldestKey uint16
	for key, bag := range d.bags {
		if oldest == nil || bag.lastSeen < oldest.lastSeen {
			oldest, oldestKey = bag, key
		}
	}
	if oldest == nil {
		return
	}
	oldest.mutex.Lock()
	oldest.reset()
	oldest.mutex.Unlock()
	delete(d.bags, oldestKey)
}

func (d *deFragger) Feed(m *Packet) (Packet, bool) {
	bag := d.bagFor(m)
	bag.mutex.Lock()
	defer bag.mutex.Unlock()
	return d.feedLocked(m, bag)
}

func (d *deFragger) feedLocked(m *Packet, bag *packetBag) (packet Packet, ok bool) {
	if m.FRAG_TOTAL <= 1 {
		return *m, true
	}
	if m.FRAG_ID >= m.FRAG_TOTAL {
		_ = pool.Put(m.DATA)
		m.DATA = nil
		return Packet{}, false
	}
	if int(m.FRAG_TOTAL) != len(bag.frags) {
		// A new message invalidates any partially received message in the bag.
		bag.reset()
		bag.reserve(m.FRAG_TOTAL)
		bag.count = 1
		bag.frags[m.FRAG_ID] = *m
		bag.filled[m.FRAG_ID>>6] |= 1 << (m.FRAG_ID & 63)
		return Packet{}, false
	}
	if bag.filled[m.FRAG_ID>>6]&(1<<(m.FRAG_ID&63)) != 0 {
		_ = pool.Put(m.DATA)
		m.DATA = nil
		return Packet{}, false
	}
	bag.frags[m.FRAG_ID] = *m
	bag.filled[m.FRAG_ID>>6] |= 1 << (m.FRAG_ID & 63)
	bag.count++
	if int(bag.count) != len(bag.frags) {
		return Packet{}, false
	}

	packet = bag.assemble()
	bag.reset()
	return packet, true
}
