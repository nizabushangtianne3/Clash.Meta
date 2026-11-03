package dns

import (
	"net"
	"testing"

	D "github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

func simpleReplyWire(t *testing.T, qtype uint16, compress bool) ([]byte, *D.Msg) {
	t.Helper()
	query := &D.Msg{}
	query.SetQuestion("simple.example.com.", qtype)
	reply := D.Msg{Compress: compress}
	reply.Id = 4321
	reply.Response = true
	reply.RecursionAvailable = true
	reply.Question = append(reply.Question, query.Question[0])

	data := make([]byte, 4)
	var record D.RR
	if qtype == D.TypeA {
		data = net.IPv4(192, 0, 2, 10)
		record = &D.A{Hdr: D.RR_Header{Name: query.Question[0].Name, Rrtype: qtype, Class: D.ClassINET, Ttl: 60}, A: data}
	} else {
		data = net.ParseIP("2001:db8::10").To16()
		record = &D.AAAA{Hdr: D.RR_Header{Name: query.Question[0].Name, Rrtype: qtype, Class: D.ClassINET, Ttl: 60}, AAAA: data}
	}
	reply.Answer = append(reply.Answer, record)

	wire, err := reply.Pack()
	require.NoError(t, err)
	return wire, &reply
}

func genericUnpack(t *testing.T, wire []byte) *D.Msg {
	t.Helper()
	var generic D.Msg
	require.NoError(t, generic.Unpack(wire))
	return &generic
}

func TestUnpackSimpleUDPReplyParsesCompressedA(t *testing.T) {
	wire, _ := simpleReplyWire(t, D.TypeA, true)
	generic := genericUnpack(t, wire)
	fast, ok := unpackSimpleUDPReply(wire, generic.Question[0])
	require.True(t, ok)
	require.Equal(t, generic, fast)
	require.Len(t, fast.Question, 1)
	require.Len(t, fast.Answer, 1)
	require.Equal(t, "simple.example.com.", fast.Question[0].Name)
	answer := fast.Answer[0].(*D.A)
	require.Equal(t, net.IPv4(192, 0, 2, 10).To4(), answer.A.To4())
	require.EqualValues(t, 4, answer.Hdr.Rdlength)
}

func TestUnpackSimpleUDPReplyParsesUncompressedAAAA(t *testing.T) {
	wire, _ := simpleReplyWire(t, D.TypeAAAA, false)
	generic := genericUnpack(t, wire)
	fast, ok := unpackSimpleUDPReply(wire, generic.Question[0])
	require.True(t, ok)
	require.Equal(t, generic, fast)
	answer := fast.Answer[0].(*D.AAAA)
	require.Equal(t, net.ParseIP("2001:db8::10").To16(), answer.AAAA.To16())
	require.EqualValues(t, 16, answer.Hdr.Rdlength)
}

func TestUnpackSimpleUDPReplyFallbacksToGeneric(t *testing.T) {
	query := &D.Msg{}
	query.SetQuestion("simple.example.com.", D.TypeA)

	reply := D.Msg{}
	reply.SetReply(query)
	reply.Answer = append(reply.Answer, &D.A{
		Hdr: D.RR_Header{Name: query.Question[0].Name, Rrtype: D.TypeA, Class: D.ClassINET, Ttl: 60},
		A:   net.IPv4(192, 0, 2, 10),
	})
	reply.Extra = append(reply.Extra, &D.OPT{Hdr: D.RR_Header{Name: ".", Rrtype: D.TypeOPT}})
	wire, err := reply.Pack()
	require.NoError(t, err)
	generic := genericUnpack(t, wire)
	fast, ok := unpackSimpleUDPReply(wire, query.Question[0])
	require.True(t, ok)
	require.Equal(t, generic, fast)

	options := D.OPT{Hdr: D.RR_Header{Name: ".", Rrtype: D.TypeOPT}}
	options.Option = append(options.Option, &D.EDNS0_LOCAL{Code: 0xffd0, Data: []byte{1}})
	reply.Answer = []D.RR{reply.Answer[0]}
	reply.Extra = []D.RR{&options}
	wire, err = reply.Pack()
	require.NoError(t, err)
	_, ok = unpackSimpleUDPReply(wire, query.Question[0])
	require.False(t, ok)

	cname := D.Msg{}
	cname.SetReply(query)
	cname.Answer = append(cname.Answer, &D.CNAME{
		Hdr:    D.RR_Header{Name: query.Question[0].Name, Rrtype: D.TypeCNAME, Class: D.ClassINET, Ttl: 60},
		Target: "other.example.com.",
	})
	wire, err = cname.Pack()
	require.NoError(t, err)
	_, ok = unpackSimpleUDPReply(wire, query.Question[0])
	require.False(t, ok)

	question := D.Msg{}
	question.SetQuestion("simple.example.com.", D.TypeMX)
	wire, err = question.Pack()
	require.NoError(t, err)
	_, ok = unpackSimpleUDPReply(wire, query.Question[0])
	require.False(t, ok)

	wrongQuery := query.Question[0]
	wrongQuery.Qtype = D.TypeMX
	_, ok = unpackSimpleUDPReply(wire, wrongQuery)
	require.False(t, ok)
}

func TestUnpackSimpleUDPReplyRejectsBadRDataLength(t *testing.T) {
	wire, _ := simpleReplyWire(t, D.TypeA, false)
	wire[len(wire)-6] = 0
	wire[len(wire)-5] = 3
	_, ok := unpackSimpleUDPReply(wire, D.Question{Name: "simple.example.com.", Qtype: D.TypeA, Qclass: D.ClassINET})
	require.False(t, ok)

	var generic D.Msg
	require.Error(t, generic.Unpack(wire))
}

func BenchmarkUnpackSimpleUDPReplyOneA(b *testing.B) {
	query := &D.Msg{}
	query.SetQuestion("one.example.com.", D.TypeA)
	reply := D.Msg{Compress: true}
	reply.SetReply(query)
	reply.RecursionAvailable = true
	reply.Answer = append(reply.Answer, &D.A{
		Hdr: D.RR_Header{Name: query.Question[0].Name, Rrtype: D.TypeA, Class: D.ClassINET, Ttl: 60},
		A:   net.IPv4(192, 0, 2, 10),
	})
	wire, err := reply.Pack()
	require.NoError(b, err)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok := unpackSimpleUDPReply(wire, query.Question[0]); !ok {
			b.Fatal("simple reply was rejected")
		}
	}
}

func BenchmarkUnpackSimpleUDPReplyOneAAAAUncompressed(b *testing.B) {
	query := &D.Msg{}
	query.SetQuestion("aaaa.example.com.", D.TypeAAAA)
	reply := D.Msg{}
	reply.SetReply(query)
	reply.RecursionAvailable = true
	reply.Answer = append(reply.Answer, &D.AAAA{
		Hdr:  D.RR_Header{Name: query.Question[0].Name, Rrtype: D.TypeAAAA, Class: D.ClassINET, Ttl: 60},
		AAAA: net.ParseIP("2001:db8::10").To16(),
	})
	wire, err := reply.Pack()
	require.NoError(b, err)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok := unpackSimpleUDPReply(wire, query.Question[0]); !ok {
			b.Fatal("simple reply was rejected")
		}
	}
}

func BenchmarkUnpackSimpleUDPReplyOneAEDNS(b *testing.B) {
	query := &D.Msg{}
	query.SetQuestion("edns.example.com.", D.TypeA)
	reply := D.Msg{Compress: true}
	reply.SetReply(query)
	reply.RecursionAvailable = true
	reply.Answer = append(reply.Answer, &D.A{
		Hdr: D.RR_Header{Name: query.Question[0].Name, Rrtype: D.TypeA, Class: D.ClassINET, Ttl: 60},
		A:   net.IPv4(192, 0, 2, 10),
	})
	reply.Extra = append(reply.Extra, &D.OPT{Hdr: D.RR_Header{Name: ".", Rrtype: D.TypeOPT}})
	wire, err := reply.Pack()
	require.NoError(b, err)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok := unpackSimpleUDPReply(wire, query.Question[0]); !ok {
			b.Fatal("EDNS reply was rejected")
		}
	}
}
