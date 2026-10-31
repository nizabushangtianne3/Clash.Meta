package outboundgroup

import (
	"bytes"
	"unicode/utf8"

	C "github.com/metacubex/mihomo/constant"
)

const jsonHexDigits = "0123456789abcdef"

func writeJSONString(dst *bytes.Buffer, s string) {
	dst.WriteByte('"')
	start := 0
	for i := 0; i < len(s); {
		if b := s[i]; b < utf8.RuneSelf {
			if b >= 0x20 && b != '"' && b != '\\' && b != '<' && b != '>' && b != '&' {
				i++
				continue
			}
			dst.WriteString(s[start:i])
			switch b {
			case '\\', '"':
				dst.WriteByte('\\')
				dst.WriteByte(b)
			case '\b':
				dst.WriteString(`\b`)
			case '\f':
				dst.WriteString(`\f`)
			case '\n':
				dst.WriteString(`\n`)
			case '\r':
				dst.WriteString(`\r`)
			case '\t':
				dst.WriteString(`\t`)
			default:
				dst.WriteString(`\u00`)
				dst.WriteByte(jsonHexDigits[b>>4])
				dst.WriteByte(jsonHexDigits[b&0x0f])
			}
			i++
			start = i
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			dst.WriteString(s[start:i])
			dst.WriteRune(utf8.RuneError)
			i += size
			start = i
			continue
		}
		if r == '\u2028' || r == '\u2029' {
			dst.WriteString(s[start:i])
			dst.WriteString(`\u202`)
			dst.WriteByte(jsonHexDigits[r&0x0f])
			i += size
			start = i
			continue
		}
		i += size
	}
	dst.WriteString(s[start:])
	dst.WriteByte('"')
}

func writeGroupNames(dst *bytes.Buffer, proxies []C.Proxy) {
	dst.WriteByte('[')
	for i, proxy := range proxies {
		if i != 0 {
			dst.WriteByte(',')
		}
		writeJSONString(dst, proxy.Name())
	}
	dst.WriteByte(']')
}

func writeStringField(dst *bytes.Buffer, key, value string) {
	dst.WriteByte(',')
	writeJSONString(dst, key)
	dst.WriteByte(':')
	writeJSONString(dst, value)
}

func writeBoolField(dst *bytes.Buffer, key string, value bool) {
	dst.WriteByte(',')
	writeJSONString(dst, key)
	dst.WriteByte(':')
	if value {
		dst.WriteString("true")
	} else {
		dst.WriteString("false")
	}
}
