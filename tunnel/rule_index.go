package tunnel

import (
	"net/netip"
	"strings"

	C "github.com/metacubex/mihomo/constant"
	rulesCommon "github.com/metacubex/mihomo/rules/common"
)

// ruleIndex answers "which rule could match this connection next" without
// walking the rule list. Domain rules live in an exact map and a suffix trie,
// address rules in two prefix tries, and every rule those structures cannot
// answer for in an ascending position list. The scan always resumes at the
// smallest candidate at or after its cursor, so rule order survives no matter
// how the rule kinds interleave.
type ruleIndex struct {
	exact  map[string][]int
	suffix *suffixNode
	ipDst  *ipRuleTrie
	ipSrc  *ipRuleTrie

	// plainPositions holds rules without an indexable payload. ipPositions
	// holds the address rules, which join the scan as plain rules while
	// neither address is known so each of them still resolves the host
	// through its own match helper.
	plainPositions []int
	ipPositions    []int
	usesHelper     []bool
}

type suffixNode struct {
	children  map[string]*suffixNode
	positions []int
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
					index.suffix = &suffixNode{}
				}
				insertSuffix(index.suffix, domain, position)
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

// next returns the first position at or after position whose rule could match
// the metadata, or the length of the rule list when none of them can.
func (index *ruleIndex) next(position int, metadata *C.Metadata) int {
	if index == nil || position >= len(index.usesHelper) {
		return position
	}

	candidate := len(index.usesHelper)
	if index.exact != nil || index.suffix != nil {
		host := metadata.RuleHost()
		if hit := positionAtOrAfter(index.exact[host], position); hit >= 0 && hit < candidate {
			candidate = hit
		}
		if index.suffix != nil {
			if hit := index.suffix.match(host, position); hit >= 0 && hit < candidate {
				candidate = hit
			}
		}
	}
	// Every address rule records its position in ipPositions, so a nil slice
	// keeps lists without them from paying for the address checks below.
	if index.ipPositions != nil {
		if index.ipDst != nil && metadata.DstIP.IsValid() {
			if hit := index.ipDst.lookup(metadata.DstIP, position); hit >= 0 && hit < candidate {
				candidate = hit
			}
		}
		if index.ipSrc != nil && metadata.SrcIP.IsValid() {
			if hit := index.ipSrc.lookup(metadata.SrcIP, position); hit >= 0 && hit < candidate {
				candidate = hit
			}
		}
		if !metadata.DstIP.IsValid() && !metadata.SrcIP.IsValid() {
			if hit := positionAtOrAfter(index.ipPositions, position); hit >= 0 && hit < candidate {
				candidate = hit
			}
		}
	}
	if len(index.plainPositions) > 0 {
		if hit := positionAtOrAfter(index.plainPositions, position); hit >= 0 && hit < candidate {
			candidate = hit
		}
	}
	return candidate
}

// after resumes the scan past a rule that did not settle the connection.
func (index *ruleIndex) after(position int, metadata *C.Metadata) int {
	return index.next(position+1, metadata)
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

func insertSuffix(root *suffixNode, suffix string, position int) {
	node := root
	labels := strings.Split(suffix, ".")
	for index := len(labels) - 1; index >= 0; index-- {
		label := labels[index]
		if node.children == nil {
			node.children = make(map[string]*suffixNode)
		}
		child, ok := node.children[label]
		if !ok {
			child = &suffixNode{}
			node.children[label] = child
		}
		node = child
	}
	node.positions = append(node.positions, position)
}

func (node *suffixNode) match(host string, from int) int {
	hit := -1
	current := node
	for end := len(host); end > 0; {
		start := end - 1
		for start >= 0 && host[start] != '.' {
			start--
		}
		child, ok := current.children[host[start+1:end]]
		if !ok {
			break
		}
		current = child
		if position := positionAtOrAfter(current.positions, from); position >= 0 &&
			(hit < 0 || position < hit) {
			hit = position
		}
		end = start
	}
	return hit
}

func positionAtOrAfter(positions []int, target int) int {
	left, right := 0, len(positions)
	for left < right {
		middle := int(uint(left+right) >> 1)
		if positions[middle] < target {
			left = middle + 1
		} else {
			right = middle
		}
	}
	if left == len(positions) {
		return -1
	}
	return positions[left]
}
