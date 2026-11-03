package jsonutil

import (
	"time"
	"unicode/utf8"

	"github.com/gofrs/uuid/v5"
)

const hexDigits = "0123456789abcdef"

// StringLen reports the byte count AppendString adds for s, so a caller can
// size a buffer that never grows while the strings of one document are
// appended. A wrong count only costs a reallocation.
func StringLen(s string) int {
	length := 2
	for i := 0; i < len(s); {
		if b := s[i]; b < utf8.RuneSelf {
			switch b {
			case '\\', '"':
				length += 2
			case '\b', '\f', '\n', '\r', '\t':
				length += 2
			default:
				if b < 0x20 || b == '<' || b == '>' || b == '&' {
					length += 6
				} else {
					length++
				}
			}
			i++
			continue
		}

		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && size == 1:
			length += utf8.RuneLen(utf8.RuneError)
		case r == '\u2028' || r == '\u2029':
			length += 6
		default:
			length += size
		}
		i += size
	}
	return length
}

// AppendString quotes a string with encoding/json HTML escaping enabled.
func AppendString(dst []byte, s string) []byte {
	dst = append(dst, '"')
	start := 0
	for i := 0; i < len(s); {
		if b := s[i]; b < utf8.RuneSelf {
			if b >= 0x20 && b != '"' && b != '\\' && b != '<' && b != '>' && b != '&' {
				i++
				continue
			}
			dst = append(dst, s[start:i]...)
			switch b {
			case '\\', '"':
				dst = append(dst, '\\', b)
			case '\b':
				dst = append(dst, '\\', 'b')
			case '\f':
				dst = append(dst, '\\', 'f')
			case '\n':
				dst = append(dst, '\\', 'n')
			case '\r':
				dst = append(dst, '\\', 'r')
			case '\t':
				dst = append(dst, '\\', 't')
			default:
				dst = append(dst, '\\', 'u', '0', '0', hexDigits[b>>4], hexDigits[b&0x0f])
			}
			i++
			start = i
			continue
		}

		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			dst = append(dst, s[start:i]...)
			dst = utf8.AppendRune(dst, utf8.RuneError)
			i += size
			start = i
			continue
		}
		if r == '\u2028' || r == '\u2029' {
			dst = append(dst, s[start:i]...)
			dst = append(dst, '\\', 'u', '2', '0', '2', hexDigits[r&0x0f])
			i += size
			start = i
			continue
		}
		i += size
	}
	dst = append(dst, s[start:]...)
	return append(dst, '"')
}

// AppendUUID writes the canonical lower-case UUID form inside JSON quotes.
func AppendUUID(dst []byte, u uuid.UUID) []byte {
	dst = append(dst, '"')
	dst = append(dst, hexDigits[u[0]>>4], hexDigits[u[0]&0x0f], hexDigits[u[1]>>4], hexDigits[u[1]&0x0f], hexDigits[u[2]>>4], hexDigits[u[2]&0x0f], hexDigits[u[3]>>4], hexDigits[u[3]&0x0f])
	dst = append(dst, '-')
	dst = append(dst, hexDigits[u[4]>>4], hexDigits[u[4]&0x0f], hexDigits[u[5]>>4], hexDigits[u[5]&0x0f])
	dst = append(dst, '-')
	dst = append(dst, hexDigits[u[6]>>4], hexDigits[u[6]&0x0f], hexDigits[u[7]>>4], hexDigits[u[7]&0x0f])
	dst = append(dst, '-')
	dst = append(dst, hexDigits[u[8]>>4], hexDigits[u[8]&0x0f], hexDigits[u[9]>>4], hexDigits[u[9]&0x0f])
	dst = append(dst, '-')
	return append(dst, hexDigits[u[10]>>4], hexDigits[u[10]&0x0f], hexDigits[u[11]>>4], hexDigits[u[11]&0x0f], hexDigits[u[12]>>4], hexDigits[u[12]&0x0f], hexDigits[u[13]>>4], hexDigits[u[13]&0x0f], hexDigits[u[14]>>4], hexDigits[u[14]&0x0f], hexDigits[u[15]>>4], hexDigits[u[15]&0x0f], '"')
}

// AppendBool appends the JSON spelling of value.
func AppendBool(dst []byte, value bool) []byte {
	if value {
		return append(dst, "true"...)
	}
	return append(dst, "false"...)
}

// AppendTime writes value in encoding/json's RFC3339Nano form.
func AppendTime(dst []byte, value time.Time) []byte {
	dst = append(dst, '"')
	dst = value.AppendFormat(dst, time.RFC3339Nano)
	return append(dst, '"')
}
