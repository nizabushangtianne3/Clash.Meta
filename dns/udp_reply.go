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

// udpReplySlab keeps the returned message and its fixed-shape records together.
// The returned *D.Msg points into this slab, so the slab lives exactly as long
// as the caller-owned reply.
type udpReplySlab struct {
	msg        D.Msg
	question   [1]D.Question
	answers    [2]D.RR
	optRecord  D.OPT
	optRR      [1]D.RR
	aRecord    D.A
	aaaaRecord D.AAAA
	aData      [4]byte
	aaaaData   [16]byte
}

// unpackSimpleUDPReply decodes the common UDP shapes: one direct A/AAAA answer
// or one CNAME followed by the requested address, with no authority records. An
// additional record is accepted only when it is an empty EDNS OPT; anything
// else, including TSIG, returns false for the complete miekg parser.
func unpackSimpleUDPReply(wire []byte, query D.Question) (*D.Msg, bool) {
	if len(wire) < 12 ||
		wire[4] != 0 || wire[5] != 1 || // one question
		wire[6] != 0 || wire[7] < 1 || wire[7] > 2 || // one answer or CNAME+address
		wire[8] != 0 || wire[9] != 0 || // no authority
		wire[10] != 0 || wire[11] > 1 { // at most one additional/TSIG
		return nil, false
	}

	bits := binary.BigEndian.Uint16(wire[2:])
	slab := &udpReplySlab{}
	slab.msg.MsgHdr = D.MsgHdr{
		Id:                 binary.BigEndian.Uint16(wire),
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
	}

	question, off, ok := matchUDPReplyQuestion(wire, query)
	if !ok {
		return nil, false
	}
	slab.question[0] = question
	slab.msg.Question = slab.question[:1:1]

	if wire[7] == 2 {
		return unpackCNAMEUDPReply(slab, wire, off, query)
	}

	answerName, off, ok := unpackUDPReplyName(wire, off, question.Name, query, "", 0)
	if !ok || off+10 > len(wire) {
		return nil, false
	}
	rrtype := binary.BigEndian.Uint16(wire[off:])
	rrclass := binary.BigEndian.Uint16(wire[off+2:])
	ttl := binary.BigEndian.Uint32(wire[off+4:])
	rdlength := int(binary.BigEndian.Uint16(wire[off+8:]))
	off += 10
	end := off + rdlength
	if rrtype != D.TypeA && rrtype != D.TypeAAAA || end > len(wire) {
		return nil, false
	}

	header := D.RR_Header{
		Name:     answerName,
		Rrtype:   rrtype,
		Class:    rrclass,
		Ttl:      ttl,
		Rdlength: uint16(rdlength),
	}
	switch rrtype {
	case D.TypeA:
		if rdlength != len(slab.aData) {
			return nil, false
		}
		copy(slab.aData[:], wire[off:end])
		slab.aRecord = D.A{Hdr: header, A: slab.aData[:]}
		slab.answers[0] = &slab.aRecord
	case D.TypeAAAA:
		if rdlength != len(slab.aaaaData) {
			return nil, false
		}
		copy(slab.aaaaData[:], wire[off:end])
		slab.aaaaRecord = D.AAAA{Hdr: header, AAAA: slab.aaaaData[:]}
		slab.answers[0] = &slab.aaaaRecord
	default:
		return nil, false
	}
	slab.msg.Answer = slab.answers[:1:1]
	if !slab.setEmptyEDNS(wire, end) {
		return nil, false
	}
	return &slab.msg, true
}

// cnameUDPReplySlab owns the two answer slots and the decoded CNAME record. The
// target string is the one unavoidable variable-length allocation; a fixed
// 255-byte buffer would enlarge every direct answer instead.
type cnameUDPReplySlab struct {
	record  D.CNAME
	answers [2]D.RR
}

func unpackCNAMEUDPReply(slab *udpReplySlab, wire []byte, off int, query D.Question) (*D.Msg, bool) {
	cnameSlab := &cnameUDPReplySlab{}
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
	cnameSlab.record = D.CNAME{
		Hdr: D.RR_Header{
			Name: firstName, Rrtype: D.TypeCNAME, Class: class, Ttl: ttl, Rdlength: uint16(rdlength),
		},
		Target: target,
	}
	cnameSlab.answers[0] = &cnameSlab.record

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
		cnameSlab.answers[1] = &slab.aRecord
	case D.TypeAAAA:
		if rdlength != len(slab.aaaaData) {
			return nil, false
		}
		copy(slab.aaaaData[:], wire[off:end])
		slab.aaaaRecord = D.AAAA{Hdr: header, AAAA: slab.aaaaData[:]}
		cnameSlab.answers[1] = &slab.aaaaRecord
	default:
		return nil, false
	}
	slab.msg.Answer = cnameSlab.answers[:2:2]
	if !slab.setEmptyEDNS(wire, end) {
		return nil, false
	}
	return &slab.msg, true
}

func (slab *udpReplySlab) setEmptyEDNS(wire []byte, end int) bool {
	if wire[11] == 0 {
		return end == len(wire)
	}
	opt, ok := unpackUDPOPT(wire, end)
	if !ok {
		return false
	}
	slab.optRecord = opt
	slab.optRR[0] = &slab.optRecord
	slab.msg.Extra = slab.optRR[:1:1]
	return true
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
