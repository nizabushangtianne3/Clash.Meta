package mekya

import (
	"encoding/binary"
	"io"
	"sync"

	"github.com/metacubex/mihomo/common/pool"
)

const packetBundleOverhead = 2

var bundleHeaderPool = sync.Pool{
	New: func() any { return new([packetBundleOverhead]byte) },
}

func writePacketBundle(w io.Writer, packet []byte) error {
	if len(packet) > 0xffff {
		return io.ErrShortBuffer
	}
	headerObject := bundleHeaderPool.Get().(*[packetBundleOverhead]byte)
	header := headerObject[:]
	binary.BigEndian.PutUint16(header[:], uint16(len(packet)))
	_, err := w.Write(header)
	bundleHeaderPool.Put(headerObject)
	if err != nil {
		return err
	}
	_, err = w.Write(packet)
	return err
}

func readPacketBundle(r io.Reader) ([]byte, error) {
	headerObject := bundleHeaderPool.Get().(*[packetBundleOverhead]byte)
	header := headerObject[:]
	_, err := io.ReadFull(r, header)
	length := binary.BigEndian.Uint16(header[:])
	bundleHeaderPool.Put(headerObject)
	if err != nil {
		return nil, err
	}
	packet := pool.Get(int(length))
	if _, err := io.ReadFull(r, packet); err != nil {
		_ = pool.Put(packet)
		return nil, err
	}
	return packet, nil
}
