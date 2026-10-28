package provider

import (
	"strings"

	"github.com/metacubex/mihomo/component/ruleindex"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/rules/common"
)

// classicalRuleIndex answers "which rule could match this host next" without
// walking the rule list. Exact domains live in a map and suffixes in a trie,
// both spanning the whole list, and every rule neither can answer for sits in
// an ascending position list. The scan resumes at the smallest candidate at or
// after its cursor, so rule order survives however the rule kinds interleave.
type classicalRuleIndex struct {
	ruleCount int
	exact     map[string][]int
	suffix    *ruleindex.SuffixTrie

	plainPositions []int
}

func buildClassicalRuleIndex(rules []C.Rule) *classicalRuleIndex {
	if len(rules) == 0 {
		return nil
	}

	index := &classicalRuleIndex{ruleCount: len(rules)}
	for position, rule := range rules {
		domain, exact, ok := classicalIndexedRule(rule)
		if !ok {
			index.plainPositions = append(index.plainPositions, position)
			continue
		}
		if exact {
			if index.exact == nil {
				index.exact = make(map[string][]int)
			}
			index.exact[domain] = append(index.exact[domain], position)
			continue
		}
		if index.suffix == nil {
			index.suffix = &ruleindex.SuffixTrie{}
		}
		index.suffix.Add(domain, position)
	}
	return index
}

// nextIndexed returns the first position at or after position whose rule the
// index can answer for, or the length of the rule list when none is left.
func (index *classicalRuleIndex) nextIndexed(position int, host string) int {
	if position >= index.ruleCount {
		return position
	}

	candidate := index.ruleCount
	if index.exact != nil || index.suffix != nil {
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
	return candidate
}

func classicalIndexedRule(rule C.Rule) (payload string, exact bool, ok bool) {
	switch typed := rule.(type) {
	case *common.Domain:
		return typed.Payload(), true, true
	case *common.DomainSuffix:
		return typed.Payload(), false, indexableSuffixPayload(typed.Payload())
	default:
		return "", false, false
	}
}

func indexableSuffixPayload(suffix string) bool {
	return suffix != "" && !strings.Contains(suffix, "..") &&
		!strings.HasPrefix(suffix, ".") && !strings.HasSuffix(suffix, ".")
}
