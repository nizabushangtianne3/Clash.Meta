package vision

import (
	"bytes"
	"encoding/binary"

	"github.com/metacubex/mihomo/common/buf"
	N "github.com/metacubex/mihomo/common/net"
	"github.com/metacubex/mihomo/log"

	"github.com/gofrs/uuid/v5"
	"github.com/metacubex/randv2"
)

const (
	PaddingHeaderLen = uuid.Size + 1 + 2 + 2 // =21

	commandPaddingContinue byte = 0x00
	commandPaddingEnd      byte = 0x01
	commandPaddingDirect   byte = 0x02
)

func ApplyPadding(buffer *buf.Buffer, command byte, userUUID *[]byte, paddingTLS bool) {
	contentLen := int32(buffer.Len())
	var paddingLen int32
	if contentLen < 900 {
		if paddingTLS {
			paddingLen = randv2.Int32N(500) + 900 - contentLen
		} else {
			paddingLen = randv2.Int32N(256)
		}
	}

	binary.BigEndian.PutUint16(buffer.ExtendHeader(2), uint16(paddingLen))
	binary.BigEndian.PutUint16(buffer.ExtendHeader(2), uint16(contentLen))
	buffer.ExtendHeader(1)[0] = command
	if userUUID != nil && *userUUID != nil {
		copy(buffer.ExtendHeader(uuid.Size), *userUUID)
		*userUUID = nil
	}

	buffer.Extend(int(paddingLen))
	if log.DebugEnabled() {
		log.Debugln("XTLS Vision write padding: command=%d, payloadLen=%d, paddingLen=%d", command, contentLen, paddingLen)
	}
}

const xrayBufSize = 8192

func (vc *Conn) ReshapeBuffer(buffer *buf.Buffer) []*buf.Buffer {
	const bufferLimit = xrayBufSize - PaddingHeaderLen
	options := N.NewReadWaitOptions(nil, vc)
	var buffers []*buf.Buffer

	for buffer.Len() > 0 {
		cutAt := min(bufferLimit, buffer.Len())
		if len(buffers) == 0 {
			if tlsStart := bytes.LastIndex(buffer.Bytes(), tlsApplicationDataStart); tlsStart >= 21 && tlsStart <= cutAt {
				cutAt = tlsStart
			}
		}

		// Pooled source buffers may have no front headroom. Vision padding
		// prepends headers, so every chunk must own a headroom-ready buffer.
		headerBuffer := options.NewBuffer()
		n, _ := headerBuffer.Write(buffer.Bytes()[:cutAt])
		buffer.Advance(n)
		buffers = append(buffers, headerBuffer)
	}

	buffer.Release()
	return buffers
}
