package obfs

import (
	"crypto/rand"
	"crypto/sha256"
)

// [salt][obfuscated payload]

const saltLen = 16

type XPlusObfuscator struct {
	Key []byte
}

func keyAndSaltSum(key, salt []byte) [sha256.Size]byte {
	var buf [64]byte
	if n := copy(buf[:], key); n == len(key) && copy(buf[n:], salt) == len(salt) {
		return sha256.Sum256(buf[:n+len(salt)])
	}

	h := sha256.New()
	h.Write(key)
	h.Write(salt)
	var sum [sha256.Size]byte
	h.Sum(sum[:0])
	return sum
}

func NewXPlusObfuscator(key []byte) *XPlusObfuscator {
	return &XPlusObfuscator{
		Key: key,
	}
}

func (x *XPlusObfuscator) Deobfuscate(in []byte, out []byte) int {
	pLen := len(in) - saltLen
	if pLen <= 0 || len(out) < pLen {
		// Invalid
		return 0
	}
	key := keyAndSaltSum(x.Key, in[:saltLen])
	// Deobfuscate the payload
	for i, c := range in[saltLen:] {
		out[i] = c ^ key[i%sha256.Size]
	}
	return pLen
}

func (x *XPlusObfuscator) Obfuscate(in []byte, out []byte) int {
	_, _ = rand.Read(out[:saltLen]) // salt
	// Obfuscate the payload
	key := keyAndSaltSum(x.Key, out[:saltLen])
	for i, c := range in {
		out[i+saltLen] = c ^ key[i%sha256.Size]
	}
	return len(in) + saltLen
}
