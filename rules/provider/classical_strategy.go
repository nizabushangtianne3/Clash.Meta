package provider

import (
	"fmt"

	C "github.com/metacubex/mihomo/constant"
	P "github.com/metacubex/mihomo/constant/provider"
	"github.com/metacubex/mihomo/log"
	"github.com/metacubex/mihomo/rules/common"
)

type classicalStrategy struct {
	rules []C.Rule
	index *classicalRuleIndex
	count int
	parse common.ParseRuleFunc
}

func (c *classicalStrategy) Behavior() P.RuleBehavior {
	return P.Classical
}

func (c *classicalStrategy) Match(metadata *C.Metadata, helper C.RuleMatchHelper) bool {
	host := metadata.RuleHost()
	index := c.index
	if index == nil {
		for _, rule := range c.rules {
			if matched, _ := rule.Match(metadata, helper); matched {
				return true
			}
		}
		return false
	}

	// The rules the index cannot answer for come out of an ascending list, so
	// walking a run of them costs one comparison each and the index is
	// consulted once per candidate it actually produces.
	position, plain, indexed := -1, 0, -1
	for {
		if indexed <= position {
			indexed = index.nextIndexed(position+1, host)
		}
		for plain < len(index.plainPositions) && index.plainPositions[plain] <= position {
			plain++
		}
		next := indexed
		if plain < len(index.plainPositions) && index.plainPositions[plain] < next {
			next = index.plainPositions[plain]
		}
		if next >= len(c.rules) {
			return false
		}
		if matched, _ := c.rules[next].Match(metadata, helper); matched {
			return true
		}
		position = next
	}
}

func (c *classicalStrategy) Count() int {
	return c.count
}

func (c *classicalStrategy) Reset() {
	c.rules = nil
	c.index = nil
	c.count = 0
}

func (c *classicalStrategy) Insert(rule string) {
	r, err := c.payloadToRule(rule)
	if err != nil {
		log.Warnln("parse classical rule [%s] error: %s", rule, err.Error())
	} else {
		c.rules = append(c.rules, r)
		c.count++
	}
}

func (c *classicalStrategy) payloadToRule(rule string) (C.Rule, error) {
	tp, payload, target, params := common.ParseRulePayload(rule, false)
	switch tp {
	case "MATCH", "RULE-SET", "SUB-RULE":
		return nil, fmt.Errorf("unsupported rule type on classical rule-set: %s", tp)
	}
	if c.parse == nil {
		return nil, fmt.Errorf("no rule parser is wired for the classical rule-set")
	}
	return c.parse(tp, payload, target, params, nil)
}

func (c *classicalStrategy) FinishInsert() {
	c.index = buildClassicalRuleIndex(c.rules)
}

func NewClassicalStrategy(parse common.ParseRuleFunc) *classicalStrategy {
	return &classicalStrategy{rules: []C.Rule{}, parse: parse}
}
