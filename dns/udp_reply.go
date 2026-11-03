package dns

import (
	"encoding/binary"

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

// udpReplySlab keeps the returned message and every object its slices point to
// in one allocation. The returned *D.Msg points into this slab, so the slab
// lives exactly as long as the caller-owned reply.
type udpReplySlab struct {
	msg        D.Msg
	question   [1]D.Question
	answer     [1]D.RR
	aRecord    D.A
	aaaaRecord D.AAAA
	aData      [4]byte
	aaaaData   [16]byte
}

// unpackSimpleUDPReply decodes the common UDP shape: one question and one
// direct A/AAAA answer with no authority, additional or TSIG records. Anything
// else returns false for the complete miekg parser.
func unpackSimpleUDPReply(wire []byte) (*D.Msg, bool) {
	if len(wire) < 12 ||
		wire[4] != 0 || wire[5] != 1 || // one question
		wire[6] != 0 || wire[7] != 1 || // one answer
		wire[8] != 0 || wire[9] != 0 || // no authority
		wire[10] != 0 || wire[11] != 0 { // no additional/TSIG
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

	question, off, ok := unpackUDPReplyQuestion(wire)
	if !ok {
		return nil, false
	}
	slab.question[0] = question
	slab.msg.Question = slab.question[:1:1]

	answerName, off, ok := unpackUDPReplyName(wire, off, question.Name)
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
		slab.answer[0] = &slab.aRecord
	case D.TypeAAAA:
		if rdlength != len(slab.aaaaData) {
			return nil, false
		}
		copy(slab.aaaaData[:], wire[off:end])
		slab.aaaaRecord = D.AAAA{Hdr: header, AAAA: slab.aaaaData[:]}
		slab.answer[0] = &slab.aaaaRecord
	default:
		return nil, false
	}
	if end != len(wire) {
		return nil, false
	}

	slab.msg.Answer = slab.answer[:1:1]
	return &slab.msg, true
}

func unpackUDPReplyQuestion(wire []byte) (D.Question, int, bool) {
	name, off, err := D.UnpackDomainName(wire, 12)
	if err != nil || off+4 > len(wire) {
		return D.Question{}, 0, false
	}
	return D.Question{
		Name:   name,
		Qtype:  binary.BigEndian.Uint16(wire[off:]),
		Qclass: binary.BigEndian.Uint16(wire[off+2:]),
	}, off + 4, true
}

// unpackUDPReplyName avoids a second domain-name allocation when the answer
// owner is the conventional compression pointer back to the question name.
func unpackUDPReplyName(wire []byte, off int, owner string) (string, int, bool) {
	if off+2 <= len(wire) && wire[off]&0xc0 == 0xc0 {
		pointer := binary.BigEndian.Uint16(wire[off:]) & 0x3fff
		if pointer == 12 {
			return owner, off + 2, true
		}
	}
	name, next, err := D.UnpackDomainName(wire, off)
	if err != nil {
		return "", 0, false
	}
	return name, next, true
}
