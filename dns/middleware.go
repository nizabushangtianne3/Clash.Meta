package dns

import (
	"net/netip"
	"strings"
	"time"

	"github.com/metacubex/mihomo/common/lru"
	"github.com/metacubex/mihomo/component/fakeip"
	"github.com/metacubex/mihomo/component/resolver"
	C "github.com/metacubex/mihomo/constant"
	icontext "github.com/metacubex/mihomo/context"
	"github.com/metacubex/mihomo/log"

	D "github.com/miekg/dns"
)

type (
	handler    func(ctx *icontext.DNSContext, r *D.Msg) (*D.Msg, error)
	middleware func(next handler) handler
)

func withHosts(mapping *lru.LruCache[netip.Addr, string]) middleware {
	return func(next handler) handler {
		return func(ctx *icontext.DNSContext, r *D.Msg) (*D.Msg, error) {
			q := r.Question[0]

			if !isIPRequest(q) {
				return next(ctx, r)
			}

			host := strings.TrimRight(q.Name, ".")
			handleCName := func(resp *D.Msg, domain string) {
				rr := &D.CNAME{}
				rr.Hdr = D.RR_Header{Name: q.Name, Rrtype: D.TypeCNAME, Class: D.ClassINET, Ttl: 10}
				rr.Target = domain + "."
				resp.Answer = append([]D.RR{rr}, resp.Answer...)
			}
			record, ok := resolver.DefaultHosts.Search(host, q.Qtype != D.TypeA && q.Qtype != D.TypeAAAA)
			if !ok {
				if record != nil && record.IsDomain {
					// replace request domain
					newR := r.Copy()
					newR.Question[0].Name = record.Domain + "."
					resp, err := next(ctx, newR)
					if err == nil {
						resp.Id = r.Id
						resp.Question = r.Question
						handleCName(resp, record.Domain)
					}
					return resp, err
				}
				return next(ctx, r)
			}

			msg := r.Copy()
			handleIPs := func() {
				for _, ipAddr := range record.IPs {
					if ipAddr.Is4() && q.Qtype == D.TypeA {
						rr := &D.A{}
						rr.Hdr = D.RR_Header{Name: q.Name, Rrtype: D.TypeA, Class: D.ClassINET, Ttl: 10}
						rr.A = ipAddr.AsSlice()
						msg.Answer = append(msg.Answer, rr)
						if mapping != nil {
							mapping.SetWithExpire(ipAddr, host, time.Now().Add(time.Second*10))
						}
					} else if ipAddr.Is6() && q.Qtype == D.TypeAAAA {
						rr := &D.AAAA{}
						rr.Hdr = D.RR_Header{Name: q.Name, Rrtype: D.TypeAAAA, Class: D.ClassINET, Ttl: 10}
						rr.AAAA = ipAddr.AsSlice()
						msg.Answer = append(msg.Answer, rr)
						if mapping != nil {
							mapping.SetWithExpire(ipAddr, host, time.Now().Add(time.Second*10))
						}
					}
				}
			}

			switch q.Qtype {
			case D.TypeA:
				handleIPs()
			case D.TypeAAAA:
				handleIPs()
			case D.TypeCNAME:
				handleCName(msg, record.Domain)
			default:
				return next(ctx, r)
			}

			msg.SetRcode(r, D.RcodeSuccess)
			msg.Authoritative = true
			msg.RecursionAvailable = true
			return msg, nil
		}
	}
}

func withMapping(mapping *lru.LruCache[netip.Addr, string]) middleware {
	return func(next handler) handler {
		return func(ctx *icontext.DNSContext, r *D.Msg) (*D.Msg, error) {
			q := r.Question[0]

			if !isIPRequest(q) {
				return next(ctx, r)
			}

			msg, err := next(ctx, r)
			if err != nil {
				return nil, err
			}

			host := strings.TrimRight(q.Name, ".")

			for _, ans := range msg.Answer {
				var ip netip.Addr
				var ttl uint32

				switch a := ans.(type) {
				case *D.A:
					ip, _ = netip.AddrFromSlice(a.A)
					ttl = a.Hdr.Ttl
				case *D.AAAA:
					ip, _ = netip.AddrFromSlice(a.AAAA)
					ttl = a.Hdr.Ttl
				default:
					continue
				}
				if !ip.IsValid() {
					continue
				}
				if !ip.IsGlobalUnicast() {
					continue
				}
				ip = ip.Unmap()

				if ttl < 1 {
					ttl = 1
				}

				mapping.SetWithExpire(ip, host, time.Now().Add(time.Second*time.Duration(ttl)))
			}

			return msg, nil
		}
	}
}

// fakeIPAnswer keeps the record and the one-element interface slice in a
// single allocation. The returned DNS message owns the combined object.
type fakeIPAnswerA struct {
	record D.A
	answer [1]D.RR
}

type fakeIPAnswerAAAA struct {
	record D.AAAA
	answer [1]D.RR
}

func withFakeIP(skipper *fakeip.Skipper, fakePool *fakeip.Pool, fakePool6 *fakeip.Pool, fakeIPTTL int) middleware {
	return func(next handler) handler {
		return func(ctx *icontext.DNSContext, r *D.Msg) (*D.Msg, error) {
			q := r.Question[0]

			host := strings.TrimRight(q.Name, ".")
			if skipper.ShouldSkipped(host) {
				return next(ctx, r)
			}

			var (
				combinedA    *fakeIPAnswerA
				combinedAAAA *fakeIPAnswerAAAA
				rr           D.RR
			)
			switch q.Qtype {
			case D.TypeA:
				if fakePool == nil {
					return handleMsgWithEmptyAnswer(r), nil
				}
				ip := fakePool.Lookup(host)
				a4 := ip.As4()
				combinedA = &fakeIPAnswerA{}
				combinedA.record.Hdr = D.RR_Header{Name: q.Name, Rrtype: D.TypeA, Class: D.ClassINET, Ttl: dnsDefaultTTL}
				copy(combinedA.record.A[:], a4[:])
				combinedA.answer[0] = &combinedA.record
				rr = combinedA.answer[0]
			case D.TypeAAAA:
				if fakePool6 == nil {
					return handleMsgWithEmptyAnswer(r), nil
				}
				ip := fakePool6.Lookup(host)
				a16 := ip.As16()
				combinedAAAA = &fakeIPAnswerAAAA{}
				combinedAAAA.record.Hdr = D.RR_Header{Name: q.Name, Rrtype: D.TypeAAAA, Class: D.ClassINET, Ttl: dnsDefaultTTL}
				copy(combinedAAAA.record.AAAA[:], a16[:])
				combinedAAAA.answer[0] = &combinedAAAA.record
				rr = combinedAAAA.answer[0]
			case D.TypeSVCB, D.TypeHTTPS:
				return handleMsgWithEmptyAnswer(r), nil
			default:
				return next(ctx, r)
			}

			// The request is owned by this DNS exchange, so the reply reuses its
			// header, question and EDNS0 record instead of cloning them.
			r.Response = true
			r.Rcode = D.RcodeSuccess
			switch {
			case combinedA != nil:
				r.Answer = combinedA.answer[:]
			case combinedAAAA != nil:
				r.Answer = combinedAAAA.answer[:]
			default:
				r.Answer = []D.RR{rr}
			}
			setMsgTTL(r, uint32(fakeIPTTL))
			r.Authoritative = true
			r.RecursionAvailable = true

			return r, nil
		}
	}
}

func withResolver(resolver resolver.Resolver, ipv6 bool) handler {
	return func(ctx *icontext.DNSContext, r *D.Msg) (*D.Msg, error) {
		q := r.Question[0]

		// return a empty AAAA msg when ipv6 disabled
		if !ipv6 && q.Qtype == D.TypeAAAA {
			return handleMsgWithEmptyAnswer(r), nil
		}

		msg, err := resolver.ExchangeContext(ctx, r)
		if err != nil {
			log.Debugln("[DNS Server] Exchange %s failed: %v", q.String(), err)
			return msg, err
		}
		msg.SetRcode(r, msg.Rcode)
		msg.Authoritative = true

		return msg, nil
	}
}

func compose(middlewares []middleware, endpoint handler) handler {
	length := len(middlewares)
	h := endpoint
	for i := length - 1; i >= 0; i-- {
		middleware := middlewares[i]
		h = middleware(h)
	}

	return h
}

func newHandler(resolver resolver.Resolver, mapper *ResolverEnhancer) handler {
	var middlewares []middleware

	if mapper.useHosts {
		middlewares = append(middlewares, withHosts(mapper.mapping))
	}

	if mapper.mode == C.DNSFakeIP {
		middlewares = append(middlewares, withFakeIP(mapper.fakeIPSkipper, mapper.fakeIPPool, mapper.fakeIPPool6, mapper.fakeIPTTL))
	}

	if mapper.mode != C.DNSNormal {
		middlewares = append(middlewares, withMapping(mapper.mapping))
	}

	return compose(middlewares, withResolver(resolver, mapper.ipv6))
}
