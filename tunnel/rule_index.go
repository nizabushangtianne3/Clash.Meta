package tunnel

import (
	"net/netip"

	"github.com/metacubex/mihomo/component/ruleindex"
	C "github.com/metacubex/mihomo/constant"
	rulesCommon "github.com/metacubex/mihomo/rules/common"
)

// ruleIndex answers "which rule could match this connection next" without
// walking the whole rule list. Domain rules live in an exact map and a suffix
// trie, address rules in two prefix tries, all spanning the entire list, and
// every rule those structures cannot answer for sits in an ascending position
// list. Rule order survives however the kinds interleave because the scan
// always takes the smallest candidate.
type ruleIndex struct {
	exact  map[string][]int
	suffix *ruleindex.SuffixTrie
	ipDst  *ipRuleTrie
	ipSrc  *ipRuleTrie

	// plainPositions holds rules without an indexable payload. ipPositions
	// holds the address rules, which the scan takes as plain rules while
	// neither address is known so each of them still resolves the host
	// through its own match helper.
	plainPositions []int
	ipPositions    []int
	usesHelper     []bool
}

func buildRuleIndex(rules []C.Rule) *ruleIndex {
	if len(rules) == 0 {
		return nil
	}

	index := &ruleIndex{
		usesHelper: make([]bool, len(rules)),
	}
	for position, rule := range rules {
		index.usesHelper[position] = ruleUsesMatchHelper(rule)
		if domain, exact, ok := indexedDomainPayload(rule); ok {
			if exact {
				if index.exact == nil {
					index.exact = make(map[string][]int)
				}
				index.exact[domain] = append(index.exact[domain], position)
			} else {
				if index.suffix == nil {
					index.suffix = &ruleindex.SuffixTrie{}
				}
				index.suffix.Add(domain, position)
			}
			continue
		}
		if prefix, source, ok := indexedIPPayload(rule); ok {
			if source {
				if index.ipSrc == nil {
					index.ipSrc = &ipRuleTrie{}
				}
				index.ipSrc.insert(prefix, position)
			} else {
				if index.ipDst == nil {
					index.ipDst = &ipRuleTrie{}
				}
				index.ipDst.insert(prefix, position)
			}
			index.ipPositions = append(index.ipPositions, position)
			continue
		}
		index.plainPositions = append(index.plainPositions, position)
	}
	return index
}

// ruleScan walks one rule list for one connection. The rules the index cannot
// answer for come out of an ascending list, so walking a run of them costs one
// comparison each; the index is consulted only when the scan is about to hand
// out a rule it produced, which keeps a host lookup off the path of every rule
// that walks past no candidate at all.
type ruleScan struct {
	index     *ruleIndex
	metadata  *C.Metadata
	ruleCount int

	cursor  int
	indexed int
	plain   int
}

func newRuleScan(index *ruleIndex, ruleCount int, metadata *C.Metadata) ruleScan {
	scan := ruleScan{
		index:     index,
		metadata:  metadata,
		ruleCount: ruleCount,
		cursor:    -1,
		indexed:   -1,
	}
	return scan
}

// next hands out the next rule position, or the rule count when the list is
// exhausted.
func (scan *ruleScan) next() int {
	if scan.index == nil {
		scan.cursor++
		return scan.cursor
	}

	// The next candidate is computed only once the scan is past the one it
	// holds, so a rule that settles the connection costs one lookup, not two.
	if scan.indexed <= scan.cursor {
		if scan.cursor >= scan.ruleCount {
			return scan.cursor
		}
		scan.indexed = scan.index.nextIndexed(scan.cursor+1, scan.metadata)
	}

	next := scan.indexed
	if len(scan.index.plainPositions) > 0 {
		for scan.plain < len(scan.index.plainPositions) &&
			scan.index.plainPositions[scan.plain] <= scan.cursor {
			scan.plain++
		}
		if scan.plain < len(scan.index.plainPositions) &&
			scan.index.plainPositions[scan.plain] < next {
			next = scan.index.plainPositions[scan.plain]
		}
	}
	scan.cursor = next
	return next
}

// nextIndexed returns the first position at or after position whose rule the
// index can answer for, or the length of the rule list when none is left.
func (index *ruleIndex) nextIndexed(position int, metadata *C.Metadata) int {
	if position >= len(index.usesHelper) {
		return position
	}

	candidate := len(index.usesHelper)
	if index.exact != nil || index.suffix != nil {
		host := metadata.RuleHost()
		if positions, ok := index.exact[host]; ok {
			if hit := ruleindex.AtOrAfter(positions, position); hit >= 0 && hit < candidate {
				candidate = hit
			}
		}
		if index.suffix != nil {
			if hit := index.suffix.Match(host, position); hit >= 0 && hit < candidate {
				candidate = hit
			}
		}
	}
	// Every address rule records its position in ipPositions, so a nil slice
	// keeps lists without them from paying for the address checks below.
	if index.ipPositions != nil {
		destination, source := metadata.DstIP.IsValid(), metadata.SrcIP.IsValid()
		if index.ipDst != nil && destination {
			if hit := index.ipDst.lookup(metadata.DstIP, position); hit >= 0 && hit < candidate {
				candidate = hit
			}
		}
		if index.ipSrc != nil && source {
			if hit := index.ipSrc.lookup(metadata.SrcIP, position); hit >= 0 && hit < candidate {
				candidate = hit
			}
		}
		if !destination && !source {
			if hit := ruleindex.AtOrAfter(index.ipPositions, position); hit >= 0 && hit < candidate {
				candidate = hit
			}
		}
	}
	return candidate
}

// usesHelperAt reports whether the rule at position needs the match helper.
// The flag is precomputed at index build time so the per-connection scan stops
// unwrapping wrappers to find out.
func (index *ruleIndex) usesHelperAt(position int, rules []C.Rule) bool {
	if index != nil && position < len(index.usesHelper) {
		return index.usesHelper[position]
	}
	return ruleUsesMatchHelper(rules[position])
}

func indexedIPPayload(rule C.Rule) (netip.Prefix, bool, bool) {
	for rule != nil {
		switch typed := rule.(type) {
		case *rulesCommon.IPCIDR:
			return typed.IPPrefix(), typed.IsSourceIPRule(), true
		case interface{ Unwrap() C.Rule }:
			rule = typed.Unwrap()
		default:
			return netip.Prefix{}, false, false
		}
	}
	return netip.Prefix{}, false, false
}

func buildSubRuleIndexes(source map[string][]C.Rule) map[string]*ruleIndex {
	if source == nil {
		return nil
	}
	result := make(map[string]*ruleIndex, len(source))
	for name, rules := range source {
		result[name] = buildRuleIndex(rules)
	}
	return result
}

func indexedDomainPayload(rule C.Rule) (string, bool, bool) {
	for rule != nil {
		switch typed := rule.(type) {
		case *rulesCommon.Domain:
			return typed.Payload(), true, true
		case *rulesCommon.DomainSuffix:
			return typed.Payload(), false, true
		case interface{ Unwrap() C.Rule }:
			rule = typed.Unwrap()
		default:
			return "", false, false
		}
	}
	return "", false, false
}
