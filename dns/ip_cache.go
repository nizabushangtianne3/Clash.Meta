package dns

import (
	"context"
	"net/netip"
	"slices"
	"time"

	"github.com/samber/lo"

	"github.com/metacubex/mihomo/component/resolver"
	D "github.com/miekg/dns"
)

// cachedIPs pairs the addresses a lookup answers with the message they came
// from. A lookup the host traces has to report the answer the message path
// reports, and that message is the one this cache already holds: the message
// path answers a copy whose lifetimes it rewrites, while the stored message
// never changes, so a cache hit reports the same answer again.
type cachedIPs struct {
	ips []netip.Addr
	msg *D.Msg
}

func ipCacheEntryKey(host string, qtype uint16) ipCacheKey {
	return ipCacheKey{host: host, qtype: qtype}
}

func (r *Resolver) lookupIPFromCache(host string, qtype uint16) (cachedIPs, bool) {
	if r == nil || r.ipCache == nil {
		return cachedIPs{}, false
	}

	entry, expireTime, hit := r.ipCache.GetWithExpire(ipCacheEntryKey(host, qtype))
	if !hit || !time.Now().Before(expireTime) {
		return cachedIPs{}, false
	}
	return entry, true
}

// lookupIPFromCachePair answers both address families from the cache. Each
// family is traced as the message path would have traced it, so a host that
// records queries still sees a record for every answer it handed out.
func (r *Resolver) lookupIPFromCachePair(ctx context.Context, host string) ([]netip.Addr, bool) {
	if r == nil || r.ipCache == nil {
		return nil, false
	}

	now := time.Now()
	ipv4, ipv4Expire, ipv4Hit := r.ipCache.GetWithExpire(ipCacheEntryKey(host, D.TypeA))
	if !ipv4Hit || ipv4.msg == nil || !now.Before(ipv4Expire) {
		return nil, false
	}
	ipv6, ipv6Expire, ipv6Hit := r.ipCache.GetWithExpire(ipCacheEntryKey(host, D.TypeAAAA))
	if !ipv6Hit || ipv6.msg == nil || !now.Before(ipv6Expire) {
		return nil, false
	}

	r.traceCacheHit(ctx, ipv4.msg.Question[0], ipv4.msg)
	r.traceCacheHit(ctx, ipv6.msg.Question[0], ipv6.msg)

	combined := make([]netip.Addr, 0, len(ipv4.ips)+len(ipv6.ips))
	combined = append(combined, ipv4.ips...)
	return append(combined, ipv6.ips...), true
}

func (r *Resolver) cacheLookupIP(host string, qtype uint16, msg *D.Msg, ips []netip.Addr) {
	if r == nil || r.ipCache == nil || msg == nil || len(ips) == 0 {
		return
	}

	ttl := minimalTTL(lo.Concat(msg.Answer, msg.Ns, msg.Extra))
	if ttl == 0 {
		return
	}
	r.ipCache.SetWithExpire(
		ipCacheEntryKey(host, qtype),
		cachedIPs{ips: slices.Clone(ips), msg: msg},
		time.Now().Add(time.Duration(ttl)*time.Second),
	)
}

func (r *Resolver) lookupIPCached(ctx context.Context, host string, dnsType uint16) ([]netip.Addr, error) {
	if entry, hit := r.lookupIPFromCache(host, dnsType); hit {
		if entry.msg != nil {
			r.traceCacheHit(ctx, entry.msg.Question[0], entry.msg)
		}
		return slices.Clone(entry.ips), nil
	}

	query := &D.Msg{}
	query.SetQuestion(D.Fqdn(host), dnsType)

	msg, err := r.answerForIPLookup(ctx, query)
	if err != nil {
		return nil, err
	}

	ips := msgToIP(msg)
	r.cacheLookupIP(host, dnsType, msg, ips)
	if len(ips) == 0 {
		return nil, resolver.ErrIPNotFound
	}
	return ips, nil
}

// answerForIPLookup answers a question whose addresses the caller only wants
// to read. A hit reports the message as the cache holds it and traces it the
// way ExchangeContext does; nothing rewrites the lifetimes of a message this
// path never hands out, so the stored message keeps its own TTLs and the next
// lookup reports the same answer again. A miss goes through the exchange,
// which is where an answer gets traced once it arrives.
func (r *Resolver) answerForIPLookup(ctx context.Context, query *D.Msg) (*D.Msg, error) {
	question := query.Question[0]
	if msg, expireTime, hit := r.cache.GetWithExpire(questionKey(question)); hit {
		if !time.Now().Before(expireTime) {
			r.refreshAsync(query)
		}
		r.traceCacheHit(ctx, question, msg)
		return msg, nil
	}
	return r.ExchangeContext(ctx, query)
}
