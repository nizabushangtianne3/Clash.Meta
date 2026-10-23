package mkcp

import (
	"crypto/cipher"
	"encoding/binary"
	"errors"
)

const (
	fnv32Offset = 2166136261
	fnv32Prime  = 16777619
)

type simpleAuthenticator struct{}

func fnv32a(data []byte) uint32 {
	hash := uint32(fnv32Offset)
	for _, item := range data {
		hash ^= uint32(item)
		hash *= fnv32Prime
	}
	return hash
}

func newSimpleAuthenticator() cipher.AEAD {
	return &simpleAuthenticator{}
}

func (*simpleAuthenticator) NonceSize() int {
	return 0
}

func (*simpleAuthenticator) Overhead() int {
	return 6
}

func (*simpleAuthenticator) Seal(dst, nonce, plain, extra []byte) []byte {
	dst = append(dst, 0, 0, 0, 0, 0, 0)
	binary.BigEndian.PutUint16(dst[4:], uint16(len(plain)))
	dst = append(dst, plain...)

	binary.BigEndian.PutUint32(dst[:4], fnv32a(dst[4:]))

	dstLen := len(dst)
	if extra := 4 - dstLen%4; extra != 4 {
		dst = append(dst, make([]byte, extra)...)
		xorfwd(dst)
		return dst[:dstLen]
	}
	xorfwd(dst)
	return dst
}

func (*simpleAuthenticator) Open(dst, nonce, ciphertext, extra []byte) ([]byte, error) {
	dst = append(dst, ciphertext...)
	dstLen := len(dst)
	if extra := 4 - dstLen%4; extra != 4 {
		dst = append(dst, make([]byte, extra)...)
		xorbkd(dst)
		dst = dst[:dstLen]
	} else {
		xorbkd(dst)
	}

	if len(dst) < 6 {
		return nil, errors.New("mkcp: invalid auth")
	}
	if binary.BigEndian.Uint32(dst[:4]) != fnv32a(dst[4:]) {
		return nil, errors.New("mkcp: invalid auth")
	}
	length := int(binary.BigEndian.Uint16(dst[4:6]))
	if len(dst)-6 != length {
		return nil, errors.New("mkcp: invalid auth")
	}
	return dst[6:], nil
}

func xorfwd(x []byte) {
	for i := 4; i < len(x); i++ {
		x[i] ^= x[i-4]
	}
}

func xorbkd(x []byte) {
	for i := len(x) - 1; i >= 4; i-- {
		x[i] ^= x[i-4]
	}
}
