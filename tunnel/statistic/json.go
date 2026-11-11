package statistic

import (
	"net/netip"
	"strconv"

	"github.com/gofrs/uuid/v5"
	"github.com/metacubex/mihomo/common/jsonutil"

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
		dst = info.AppendJSON(dst)
	}
	dst = append(dst, `],"memory":`...)
	dst = strconv.AppendUint(dst, s.Memory, 10)
	return append(dst, '}')
}

// AppendJSON appends one connection row to dst. The host's own encoder for a
// whole table calls it per row, and a wrapper that answers one table entry
// calls it on its own.
func (i *TrackerInfo) AppendJSON(dst []byte) []byte {
	dst = i.appendPrefixJSON(dst)
	dst = append(dst, `,"upload":`...)
	dst = strconv.AppendInt(dst, i.UploadTotal.Load(), 10)
	dst = append(dst, `,"download":`...)
	dst = strconv.AppendInt(dst, i.DownloadTotal.Load(), 10)
	return i.appendSuffixJSON(dst)
}

func (i *TrackerInfo) appendPrefixJSON(dst []byte) []byte {
	i.prefixOnce.Do(func() {
		buf := make([]byte, 0, 512)
		buf = append(buf, `{"id":"`...)
		buf = appendUUID(buf, i.UUID)
		buf = append(buf, `","metadata":{`...)
		buf = appendMetadataJSON(buf, i.Metadata)
		buf = append(buf, '}')
		i.prefixJSON = buf
	})
	return append(dst, i.prefixJSON...)
}

func (i *TrackerInfo) appendSuffixJSON(dst []byte) []byte {
	i.suffixOnce.Do(func() {
		buf := make([]byte, 0, 256)
		buf = append(buf, `,"start":`...)
		buf = strconv.AppendInt(buf, i.Start, 10)
		buf = append(buf, `,"chains":`...)
		buf = appendJSONStringSlice(buf, i.Chain)
		buf = append(buf, `,"providerChains":`...)
		buf = appendJSONStringSlice(buf, i.ProviderChain)
		buf = append(buf, `,"rule":`...)
		buf = jsonutil.AppendString(buf, i.Rule)
		buf = append(buf, `,"rulePayload":`...)
		buf = jsonutil.AppendString(buf, i.RulePayload)
		buf = append(buf, '}')
		i.suffixJSON = buf
	})
	return append(dst, i.suffixJSON...)
}

func appendMetadataJSON(dst []byte, md *C.Metadata) []byte {
	dst = append(dst, `"network":`...)
	dst = jsonutil.AppendString(dst, md.NetWork.String())
	dst = append(dst, `,"type":`...)
	dst = jsonutil.AppendString(dst, md.Type.String())
	dst = append(dst, `,"sourceIP":`...)
	dst = appendJSONAddr(dst, md.SrcIP)
	dst = append(dst, `,"destinationIP":`...)
	dst = appendJSONAddr(dst, md.DstIP)
	dst = append(dst, `,"sourceGeoIP":`...)
	dst = appendJSONStringSlice(dst, md.SrcGeoIP)
	dst = append(dst, `,"destinationGeoIP":`...)
	dst = appendJSONStringSlice(dst, md.DstGeoIP)
	dst = append(dst, `,"sourceIPASN":`...)
	dst = jsonutil.AppendString(dst, md.SrcIPASN)
	dst = append(dst, `,"destinationIPASN":`...)
	dst = jsonutil.AppendString(dst, md.DstIPASN)
	dst = append(dst, `,"sourcePort":`...)
	dst = appendJSONQuotedUint(dst, uint64(md.SrcPort))
	dst = append(dst, `,"destinationPort":`...)
	dst = appendJSONQuotedUint(dst, uint64(md.DstPort))
	dst = append(dst, `,"inboundIP":`...)
	dst = appendJSONAddr(dst, md.InIP)
	dst = append(dst, `,"inboundPort":`...)
	dst = appendJSONQuotedUint(dst, uint64(md.InPort))
	dst = append(dst, `,"inboundName":`...)
	dst = jsonutil.AppendString(dst, md.InName)
	dst = append(dst, `,"inboundUser":`...)
	dst = jsonutil.AppendString(dst, md.InUser)
	dst = append(dst, `,"rematchName":`...)
	dst = jsonutil.AppendString(dst, md.RematchName)
	dst = append(dst, `,"host":`...)
	dst = jsonutil.AppendString(dst, md.Host)
	dst = append(dst, `,"dnsMode":`...)
	dst = jsonutil.AppendString(dst, md.DNSMode.String())
	dst = append(dst, `,"uid":`...)
	dst = strconv.AppendUint(dst, uint64(md.Uid), 10)
	dst = append(dst, `,"process":`...)
	dst = jsonutil.AppendString(dst, md.Process)
	dst = append(dst, `,"processPath":`...)
	dst = jsonutil.AppendString(dst, md.ProcessPath)
	dst = append(dst, `,"specialProxy":`...)
	dst = jsonutil.AppendString(dst, md.SpecialProxy)
	dst = append(dst, `,"specialRules":`...)
	dst = jsonutil.AppendString(dst, md.SpecialRules)
	dst = append(dst, `,"remoteDestination":`...)
	dst = jsonutil.AppendString(dst, md.RemoteDst)
	dst = append(dst, `,"dscp":`...)
	dst = strconv.AppendUint(dst, uint64(md.DSCP), 10)
	dst = append(dst, `,"sniffHost":`...)
	return jsonutil.AppendString(dst, md.SniffHost)
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
		dst = jsonutil.AppendString(dst, value)
	}
	return append(dst, ']')
}
