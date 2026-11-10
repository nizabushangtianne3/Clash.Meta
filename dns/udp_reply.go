package dns

import (
	"encoding/binary"
	"strings"

	D "github.com/miekg/dns"
)

// DNS header flag bits from RFC 1035. miekg/dns keeps these constants private,
// but the UDP fast path decodes the header before it builds the typed reply.
const (
	dnsFlagResponse uint16 = 1 << 15
	dnsFlagOpcode   uint16 = 0xf << 11
	dnsFlagAA       uint16 = 1 << 10
	dnsFlagTC       uint16 = 1 << 9
	dnsFlagRD       uint16 = 1 << 8
	dnsFlagRA       uint16 = 1 << 7
	dnsFlagZero     uint16 = 1 << 6
	dnsFlagAD       uint16 = 1 << 5
	dnsFlagCD       uint16 = 1 << 4
	dnsFlagRcode    uint16 = 0xf
)

// maxUDPFastAnswers bounds the multi-answer slab. Larger legitimate answers use
// the full parser rather than reserving an unbounded response-shaped slab.
const maxUDPFastAnswers = 8

// udpReplyBase owns fields shared by every fast-path response. The returned
// *D.Msg points into its containing slab, so the slab lives exactly as long as
// the caller-owned reply.
type udpReplyBase struct {
	msg       D.Msg
	question  [1]D.Question
	optRecord D.OPT
	optRR     [1]D.RR
}

func newUDPReplyBase(id, bits uint16) udpReplyBase {
	return udpReplyBase{msg: D.Msg{MsgHdr: D.MsgHdr{
		Id:                 id,
		Response:           bits&dnsFlagResponse != 0,
		Opcode:             int(bits & dnsFlagOpcode >> 11),
		Authoritative:      bits&dnsFlagAA != 0,
		Truncated:          bits&dnsFlagTC != 0,
		RecursionDesired:   bits&dnsFlagRD != 0,
		RecursionAvailable: bits&dnsFlagRA != 0,
		Zero:               bits&dnsFlagZero != 0,
		AuthenticatedData:  bits&dnsFlagAD != 0,
		CheckingDisabled:   bits&dnsFlagCD != 0,
		Rcode:              int(bits & dnsFlagRcode),
	}}}
}

func (b *udpReplyBase) setQuestion(question D.Question) {
	b.question[0] = question
	b.msg.Question = b.question[:1:1]
}

func (b *udpReplyBase) setEmptyEDNS(wire []byte, end int) bool {
	if wire[11] == 0 {
		return end == len(wire)
	}
	opt, ok := unpackUDPOPT(wire, end)
	if !ok {
		return false
	}
	b.optRecord = opt
	b.optRR[0] = &b.optRecord
	b.msg.Extra = b.optRR[:1:1]
	return true
}

// directAUDPReplySlab owns only the fields required by an A response.
type directAUDPReplySlab struct {
	udpReplyBase
	record D.A
	data   [4]byte
	answer [1]D.RR
}

// directAAAAUDPReplySlab owns only the fields required by an AAAA response.
type directAAAAUDPReplySlab struct {
	udpReplyBase
	record D.AAAA
	data   [16]byte
	answer [1]D.RR
}

// unpackSimpleUDPReply decodes the common UDP shapes: direct A/AAAA addresses
// or one CNAME followed by the requested address family, with no authority
// records. An additional record is accepted only when it is an empty EDNS OPT;
// anything else, including TSIG, returns false for the complete miekg parser.
func unpackSimpleUDPReply(wire []byte, query D.Question) (*D.Msg, bool) {
	if len(wire) < 12 ||
		wire[4] != 0 || wire[5] != 1 ||
		wire[6] != 0 || wire[7] < 1 ||
		wire[8] != 0 || wire[9] != 0 ||
		wire[10] != 0 || wire[11] > 1 ||
		wire[7] > maxUDPFastAnswers {
		return nil, false
	}

	bits := binary.BigEndian.Uint16(wire[2:])
	question, off, ok := matchUDPReplyQuestion(wire, query)
	if !ok {
		return nil, false
	}

	if wire[7] == 2 {
		slab := &cnameUDPReplySlab{udpReplyBase: newUDPReplyBase(binary.BigEndian.Uint16(wire), bits)}
		slab.setQuestion(question)
		return unpackCNAMEUDPReply(slab, wire, off, query)
	}
	if wire[7] > 2 {
		base := newUDPReplyBase(binary.BigEndian.Uint16(wire), bits)
		base.setQuestion(question)
		return unpackMultiUDPReply(&base, wire, off, query)
	}
	return unpackDirectUDPReply(wire, off, query, bits)
}

func unpackDirectUDPReply(wire []byte, off int, query D.Question, bits uint16) (*D.Msg, bool) {
	nameEnd, ok := skipUDPReplyName(wire, off)
	if !ok || nameEnd+10 > len(wire) {
		return nil, false
	}
	rrtype := binary.BigEndian.Uint16(wire[nameEnd:])
	rrclass := binary.BigEndian.Uint16(wire[nameEnd+2:])
	ttl := binary.BigEndian.Uint32(wire[nameEnd+4:])
	rdlength := int(binary.BigEndian.Uint16(wire[nameEnd+8:]))
	dataOff := nameEnd + 10
	end := dataOff + rdlength
	if (rrtype != D.TypeA && rrtype != D.TypeAAAA) || rrclass != D.ClassINET || end > len(wire) {
		return nil, false
	}

	answerName, _, ok := unpackUDPReplyName(wire, off, query.Name, query, "", 0)
	if !ok {
		return nil, false
	}
	header := D.RR_Header{
		Name: answerName, Rrtype: rrtype, Class: rrclass, Ttl: ttl, Rdlength: uint16(rdlength),
	}

	switch rrtype {
	case D.TypeA:
		if rdlength != 4 {
			return nil, false
		}
		slab := &directAUDPReplySlab{udpReplyBase: newUDPReplyBase(binary.BigEndian.Uint16(wire), bits)}
		slab.setQuestion(query)
		copy(slab.data[:], wire[dataOff:end])
		slab.record = D.A{Hdr: header, A: slab.data[:]}
		slab.answer[0] = &slab.record
		slab.msg.Answer = slab.answer[:1:1]
		if !slab.setEmptyEDNS(wire, end) {
			return nil, false
		}
		return &slab.msg, true
	case D.TypeAAAA:
		if rdlength != 16 {
			return nil, false
		}
		slab := &directAAAAUDPReplySlab{udpReplyBase: newUDPReplyBase(binary.BigEndian.Uint16(wire), bits)}
		slab.setQuestion(query)
		copy(slab.data[:], wire[dataOff:end])
		slab.record = D.AAAA{Hdr: header, AAAA: slab.data[:]}
		slab.answer[0] = &slab.record
		slab.msg.Answer = slab.answer[:1:1]
		if !slab.setEmptyEDNS(wire, end) {
			return nil, false
		}
		return &slab.msg, true
	default:
		return nil, false
	}
}

// cnameUDPReplySlab owns the two answer slots and the decoded CNAME record. The
// target string is the one unavoidable variable-length allocation; a fixed
// 255-byte buffer would enlarge every direct answer instead.
type cnameUDPReplySlab struct {
	udpReplyBase
	record     D.CNAME
	answers    [2]D.RR
	aRecord    D.A
	aaaaRecord D.AAAA
	aData      [4]byte
	aaaaData   [16]byte
}

func unpackCNAMEUDPReply(slab *cnameUDPReplySlab, wire []byte, off int, query D.Question) (*D.Msg, bool) {
	firstName, off, ok := unpackUDPReplyName(wire, off, query.Name, query, "", 0)
	if !ok || off+10 > len(wire) {
		return nil, false
	}
	class := binary.BigEndian.Uint16(wire[off+2:])
	ttl := binary.BigEndian.Uint32(wire[off+4:])
	rdlength := int(binary.BigEndian.Uint16(wire[off+8:]))
	off += 10
	end := off + rdlength
	if binary.BigEndian.Uint16(wire[off-10:]) != D.TypeCNAME ||
		class != D.ClassINET || end > len(wire) {
		return nil, false
	}
	target, next, err := D.UnpackDomainName(wire, off)
	if err != nil || next != end || target == "" {
		return nil, false
	}
	slab.record = D.CNAME{
		Hdr: D.RR_Header{
			Name: firstName, Rrtype: D.TypeCNAME, Class: class, Ttl: ttl, Rdlength: uint16(rdlength),
		},
		Target: target,
	}
	slab.answers[0] = &slab.record

	secondName, off, ok := unpackUDPReplyName(wire, end, query.Name, query, target, off)
	if !ok || off+10 > len(wire) {
		return nil, false
	}
	rrtype := binary.BigEndian.Uint16(wire[off:])
	class = binary.BigEndian.Uint16(wire[off+2:])
	ttl = binary.BigEndian.Uint32(wire[off+4:])
	rdlength = int(binary.BigEndian.Uint16(wire[off+8:]))
	off += 10
	end = off + rdlength
	if (rrtype != D.TypeA && rrtype != D.TypeAAAA) || rrtype != query.Qtype ||
		class != D.ClassINET || end > len(wire) {
		return nil, false
	}
	header := D.RR_Header{
		Name: secondName, Rrtype: rrtype, Class: class, Ttl: ttl, Rdlength: uint16(rdlength),
	}
	switch rrtype {
	case D.TypeA:
		if rdlength != len(slab.aData) {
			return nil, false
		}
		copy(slab.aData[:], wire[off:end])
		slab.aRecord = D.A{Hdr: header, A: slab.aData[:]}
		slab.answers[1] = &slab.aRecord
	case D.TypeAAAA:
		if rdlength != len(slab.aaaaData) {
			return nil, false
		}
		copy(slab.aaaaData[:], wire[off:end])
		slab.aaaaRecord = D.AAAA{Hdr: header, AAAA: slab.aaaaData[:]}
		slab.answers[1] = &slab.aaaaRecord
	default:
		return nil, false
	}
	slab.msg.Answer = slab.answers[:2:2]
	if !slab.setEmptyEDNS(wire, end) {
		return nil, false
	}
	return &slab.msg, true
}

// multiUDPReplySlab owns exact-size storage for a direct address set or a
// CNAME followed by every address. Variable slices avoid enlarging the common
// one- and two-answer paths.
type multiUDPReplySlab struct {
	cname      D.CNAME
	answers    []D.RR
	addresses  []D.A
	addresses6 []D.AAAA
	data       []byte
}

func unpackMultiUDPReply(slab *udpReplyBase, wire []byte, off int, query D.Question) (*D.Msg, bool) {
	count := int(wire[7])
	if query.Qtype != D.TypeA && query.Qtype != D.TypeAAAA {
		return nil, false
	}
	nameEnd, ok := skipUDPReplyName(wire, off)
	if !ok || nameEnd+10 > len(wire) {
		return nil, false
	}
	firstType := binary.BigEndian.Uint16(wire[nameEnd:])
	if firstType != D.TypeCNAME && firstType != query.Qtype {
		return nil, false
	}
	cname := firstType == D.TypeCNAME
	addressCount := count
	if cname {
		addressCount--
		if addressCount == 0 {
			return nil, false
		}
	}
	addressSize := 4
	if query.Qtype == D.TypeAAAA {
		addressSize = 16
	}
	multi := &multiUDPReplySlab{
		answers: make([]D.RR, count),
		data:    make([]byte, 0, addressSize*addressCount),
	}
	if query.Qtype == D.TypeA {
		multi.addresses = make([]D.A, addressCount)
	} else {
		multi.addresses6 = make([]D.AAAA, addressCount)
	}

	aliasName := ""
	aliasOff := 0
	answer := 0
	address := 0
	for i := range count {
		name, recordOff, ok := unpackUDPReplyName(wire, off, query.Name, query, aliasName, aliasOff)
		if !ok || recordOff+10 > len(wire) {
			return nil, false
		}
		rrtype := binary.BigEndian.Uint16(wire[recordOff:])
		class := binary.BigEndian.Uint16(wire[recordOff+2:])
		ttl := binary.BigEndian.Uint32(wire[recordOff+4:])
		rdlength := int(binary.BigEndian.Uint16(wire[recordOff+8:]))
		dataOff := recordOff + 10
		end := dataOff + rdlength
		if class != D.ClassINET || end > len(wire) {
			return nil, false
		}

		if i == 0 && rrtype == D.TypeCNAME {
			target, next, err := D.UnpackDomainName(wire, dataOff)
			if err != nil || next != end || target == "" {
				return nil, false
			}
			multi.cname = D.CNAME{
				Hdr: D.RR_Header{
					Name: name, Rrtype: rrtype, Class: class, Ttl: ttl, Rdlength: uint16(rdlength),
				},
				Target: target,
			}
			multi.answers[0] = &multi.cname
			answer = 1
			aliasName, aliasOff = target, dataOff
			off = end
			continue
		}
		if rrtype != query.Qtype || rdlength != addressSize || address == addressCount {
			return nil, false
		}
		start := len(multi.data)
		multi.data = append(multi.data, wire[dataOff:end]...)
		header := D.RR_Header{
			Name: name, Rrtype: rrtype, Class: class, Ttl: ttl, Rdlength: uint16(rdlength),
		}
		if query.Qtype == D.TypeA {
			multi.addresses[address] = D.A{
				Hdr: header,
				A:   multi.data[start:len(multi.data):len(multi.data)],
			}
			multi.answers[answer] = &multi.addresses[address]
		} else {
			multi.addresses6[address] = D.AAAA{
				Hdr:  header,
				AAAA: multi.data[start:len(multi.data):len(multi.data)],
			}
			multi.answers[answer] = &multi.addresses6[address]
		}
		address++
		answer++
		off = end
	}
	if address != addressCount {
		return nil, false
	}
	slab.msg.Answer = multi.answers[:count:count]
	if !slab.setEmptyEDNS(wire, off) {
		return nil, false
	}
	return &slab.msg, true
}

// matchUDPReplyQuestion avoids allocating a domain string for the common reply:
// it matches the request wire labels in place and falls back for escaped names.
func matchUDPReplyQuestion(wire []byte, query D.Question) (D.Question, int, bool) {
	end, ok := matchUDPReplyName(wire, 12, query.Name)
	if !ok || end+4 > len(wire) {
		return D.Question{}, 0, false
	}
	question := D.Question{
		Name:   query.Name,
		Qtype:  binary.BigEndian.Uint16(wire[end:]),
		Qclass: binary.BigEndian.Uint16(wire[end+2:]),
	}
	if question.Qtype != query.Qtype || question.Qclass != query.Qclass {
		return D.Question{}, 0, false
	}
	return question, end + 4, true
}

func skipUDPReplyName(wire []byte, off int) (int, bool) {
	for range 128 {
		if off >= len(wire) {
			return 0, false
		}
		length := int(wire[off])
		switch length & 0xc0 {
		case 0:
			if length == 0 {
				return off + 1, true
			}
			off++
			if off+length > len(wire) {
				return 0, false
			}
			off += length
		case 0xc0:
			return off + 2, true
		default:
			return 0, false
		}
	}
	return 0, false
}

func asciiEqualFold(a []byte, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		aa, bb := a[i], b[i]
		if aa >= 'A' && aa <= 'Z' {
			aa += 'a' - 'A'
		}
		if bb >= 'A' && bb <= 'Z' {
			bb += 'a' - 'A'
		}
		if aa != bb {
			return false
		}
	}
	return true
}

func matchUDPReplyName(wire []byte, off int, name string) (int, bool) {
	if name == "" || name == "." {
		if off >= len(wire) || wire[off] != 0 {
			return 0, false
		}
		return off + 1, true
	}
	if name[0] == '.' || name[len(name)-1] != '.' || strings.IndexByte(name, '\\') >= 0 {
		return 0, false
	}
	begin := 0
	for begin < len(name)-1 {
		end := strings.IndexByte(name[begin:], '.')
		if end >= 0 {
			end += begin
		}
		if end < 0 || end == begin || end-begin > 63 {
			return 0, false
		}
		if off >= len(wire) || int(wire[off]) != end-begin || off+1+end-begin > len(wire) {
			return 0, false
		}
		off++
		if !asciiEqualFold(wire[off:off+end-begin], name[begin:end]) {
			return 0, false
		}
		off += end - begin
		begin = end + 1
	}
	if off >= len(wire) || wire[off] != 0 {
		return 0, false
	}
	return off + 1, true
}

// unpackUDPReplyName reuses the request name when the answer owner is either
// compressed back to offset 12 or written uncompressed in place.
func unpackUDPReplyName(wire []byte, off int, owner string, query D.Question, aliasName string, aliasOff int) (string, int, bool) {
	if off+2 <= len(wire) && wire[off]&0xc0 == 0xc0 {
		pointer := binary.BigEndian.Uint16(wire[off:]) & 0x3fff
		if pointer == 12 {
			return owner, off + 2, true
		}
		if aliasName != "" && pointer == uint16(aliasOff) {
			return aliasName, off + 2, true
		}
	}
	if end, ok := matchUDPReplyName(wire, off, query.Name); ok {
		return owner, end, true
	}
	if aliasName != "" {
		if end, ok := matchUDPReplyName(wire, off, aliasName); ok {
			return aliasName, end, true
		}
	}
	name, next, err := D.UnpackDomainName(wire, off)
	if err != nil {
		return "", 0, false
	}
	return name, next, true
}

// unpackUDPOPT accepts only the common empty EDNS response record. Options have
// variable ownership and allocation shapes, so they stay on the full parser.
func unpackUDPOPT(wire []byte, off int) (D.OPT, bool) {
	if off+11 > len(wire) ||
		wire[off] != 0 ||
		binary.BigEndian.Uint16(wire[off+1:]) != D.TypeOPT ||
		binary.BigEndian.Uint16(wire[off+9:]) != 0 ||
		off+11 != len(wire) {
		return D.OPT{}, false
	}
	return D.OPT{Hdr: D.RR_Header{
		Name:   ".",
		Rrtype: D.TypeOPT,
		Class:  binary.BigEndian.Uint16(wire[off+3:]),
		Ttl:    binary.BigEndian.Uint32(wire[off+5:]),
	}}, true
}
