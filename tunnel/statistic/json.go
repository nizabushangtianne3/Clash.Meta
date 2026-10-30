package statistic

import (
	"net/netip"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/gofrs/uuid/v5"

	C "github.com/metacubex/mihomo/constant"
)

// The connection view is encoded by hand. The reflection encoder called a
// marshaler for every field carrying an address, a uuid or a timestamp, and
// each of those had to hand back a fresh slice, so a poll paid nine
// allocations per connection for a document the host reads and discards.
// The field set, their order and their escaping stay pinned by
// connection_view_test.go against the document encoding/json produced before.

const jsonHexDigits = "0123456789abcdef"

// AppendJSON appends the connection table to dst in the shape the host reads.
func (s *Snapshot) AppendJSON(dst []byte) []byte {
	dst = append(dst, `{"downloadTotal":`...)
	dst = strconv.AppendInt(dst, s.DownloadTotal, 10)
	dst = append(dst, `,"uploadTotal":`...)
	dst = strconv.AppendInt(dst, s.UploadTotal, 10)
	dst = append(dst, `,"connections":[`...)
	for i, info := range s.Connections {
		if i > 0 {
			dst = append(dst, ',')
		}
		if info == nil {
			dst = append(dst, 'n', 'u', 'l', 'l')
			continue
		}
		dst = info.appendJSON(dst)
	}
	dst = append(dst, `],"memory":`...)
	dst = strconv.AppendUint(dst, s.Memory, 10)
	return append(dst, '}')
}

func (i *TrackerInfo) appendJSON(dst []byte) []byte {
	dst = append(dst, `{"id":"`...)
	dst = appendUUID(dst, i.UUID)
	dst = append(dst, `","metadata":`...)
	if i.Metadata == nil {
		dst = append(dst, 'n', 'u', 'l', 'l')
	} else {
		dst = append(dst, '{')
		dst = appendMetadataJSON(dst, i.Metadata)
		dst = append(dst, '}')
	}
	dst = append(dst, `,"upload":`...)
	dst = strconv.AppendInt(dst, i.UploadTotal.Load(), 10)
	dst = append(dst, `,"download":`...)
	dst = strconv.AppendInt(dst, i.DownloadTotal.Load(), 10)
	dst = append(dst, `,"start":`...)
	dst = i.appendStartJSON(dst)
	dst = append(dst, `,"chains":`...)
	dst = appendJSONStringSlice(dst, i.Chain)
	dst = append(dst, `,"providerChains":`...)
	dst = appendJSONStringSlice(dst, i.ProviderChain)
	dst = append(dst, `,"rule":`...)
	dst = appendJSONString(dst, i.Rule)
	dst = append(dst, `,"rulePayload":`...)
	dst = appendJSONString(dst, i.RulePayload)
	return append(dst, '}')
}

func (i *TrackerInfo) appendStartJSON(dst []byte) []byte {
	i.startJSONOnce.Do(func() {
		i.startJSON = i.Start.AppendFormat(make([]byte, 0, len(time.RFC3339Nano)+18), time.RFC3339Nano)
	})
	dst = append(dst, '"')
	dst = append(dst, i.startJSON...)
	return append(dst, '"')
}

func appendMetadataJSON(dst []byte, md *C.Metadata) []byte {
	dst = append(dst, `"network":`...)
	dst = appendJSONString(dst, md.NetWork.String())
	dst = append(dst, `,"type":`...)
	dst = appendJSONString(dst, md.Type.String())
	dst = append(dst, `,"sourceIP":`...)
	dst = appendJSONAddr(dst, md.SrcIP)
	dst = append(dst, `,"destinationIP":`...)
	dst = appendJSONAddr(dst, md.DstIP)
	dst = append(dst, `,"sourceGeoIP":`...)
	dst = appendJSONStringSlice(dst, md.SrcGeoIP)
	dst = append(dst, `,"destinationGeoIP":`...)
	dst = appendJSONStringSlice(dst, md.DstGeoIP)
	dst = append(dst, `,"sourceIPASN":`...)
	dst = appendJSONString(dst, md.SrcIPASN)
	dst = append(dst, `,"destinationIPASN":`...)
	dst = appendJSONString(dst, md.DstIPASN)
	dst = append(dst, `,"sourcePort":`...)
	dst = appendJSONQuotedUint(dst, uint64(md.SrcPort))
	dst = append(dst, `,"destinationPort":`...)
	dst = appendJSONQuotedUint(dst, uint64(md.DstPort))
	dst = append(dst, `,"inboundIP":`...)
	dst = appendJSONAddr(dst, md.InIP)
	dst = append(dst, `,"inboundPort":`...)
	dst = appendJSONQuotedUint(dst, uint64(md.InPort))
	dst = append(dst, `,"inboundName":`...)
	dst = appendJSONString(dst, md.InName)
	dst = append(dst, `,"inboundUser":`...)
	dst = appendJSONString(dst, md.InUser)
	dst = append(dst, `,"rematchName":`...)
	dst = appendJSONString(dst, md.RematchName)
	dst = append(dst, `,"host":`...)
	dst = appendJSONString(dst, md.Host)
	dst = append(dst, `,"dnsMode":`...)
	dst = appendJSONString(dst, md.DNSMode.String())
	dst = append(dst, `,"uid":`...)
	dst = strconv.AppendUint(dst, uint64(md.Uid), 10)
	dst = append(dst, `,"process":`...)
	dst = appendJSONString(dst, md.Process)
	dst = append(dst, `,"processPath":`...)
	dst = appendJSONString(dst, md.ProcessPath)
	dst = append(dst, `,"specialProxy":`...)
	dst = appendJSONString(dst, md.SpecialProxy)
	dst = append(dst, `,"specialRules":`...)
	dst = appendJSONString(dst, md.SpecialRules)
	dst = append(dst, `,"remoteDestination":`...)
	dst = appendJSONString(dst, md.RemoteDst)
	dst = append(dst, `,"dscp":`...)
	dst = strconv.AppendUint(dst, uint64(md.DSCP), 10)
	dst = append(dst, `,"sniffHost":`...)
	return appendJSONString(dst, md.SniffHost)
}

// appendUUID writes the canonical form the uuid marshaler produced.
func appendUUID(dst []byte, u uuid.UUID) []byte {
	dst = appendHexBytes(dst, u[0], u[1], u[2], u[3])
	dst = append(dst, '-')
	dst = appendHexBytes(dst, u[4], u[5])
	dst = append(dst, '-')
	dst = appendHexBytes(dst, u[6], u[7])
	dst = append(dst, '-')
	dst = appendHexBytes(dst, u[8], u[9])
	dst = append(dst, '-')
	return appendHexBytes(dst, u[10], u[11], u[12], u[13], u[14], u[15])
}

func appendHexBytes(dst []byte, bytes ...byte) []byte {
	for _, b := range bytes {
		dst = append(dst, jsonHexDigits[b>>4], jsonHexDigits[b&0x0f])
	}
	return dst
}

func appendJSONAddr(dst []byte, addr netip.Addr) []byte {
	dst = append(dst, '"')
	dst, _ = addr.AppendText(dst)
	return append(dst, '"')
}

func appendJSONQuotedUint(dst []byte, v uint64) []byte {
	dst = append(dst, '"')
	dst = strconv.AppendUint(dst, v, 10)
	return append(dst, '"')
}

func appendJSONStringSlice(dst []byte, values []string) []byte {
	if values == nil {
		return append(dst, 'n', 'u', 'l', 'l')
	}
	dst = append(dst, '[')
	for i, value := range values {
		if i > 0 {
			dst = append(dst, ',')
		}
		dst = appendJSONString(dst, value)
	}
	return append(dst, ']')
}

// appendJSONString quotes and escapes the way encoding/json does with HTML
// escaping on, which is what both the reflection writer and the websocket
// writer in hub/route apply.
func appendJSONString(dst []byte, s string) []byte {
	if s == "" {
		return append(dst, '"', '"')
	}
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
				dst = append(dst, '\\', 'u', '0', '0', jsonHexDigits[b>>4], jsonHexDigits[b&0x0f])
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
		// json escapes the two unicode separators because they break some
		// hosts that read the document as javascript.
		if r == '\u2028' || r == '\u2029' {
			dst = append(dst, s[start:i]...)
			dst = append(dst, '\\', 'u', '2', '0', '2', jsonHexDigits[r&0x0f])
			i += size
			start = i
			continue
		}
		i += size
	}
	dst = append(dst, s[start:]...)
	return append(dst, '"')
}
