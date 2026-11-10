package config

import (
	"testing"

	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/rules"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The prelude every document here needs: one proxy for rules to name, one
// group, and the geodata settings the parse reads before anything else.
const documentPrelude = `mixed-port: 7890
mode: rule
log-level: info
ipv6: false
geodata-mode: false
etag-support: false
proxies:
  - {name: node-0000, type: ss, server: 10.0.0.1, port: 443, cipher: aes-128-gcm, password: password}
proxy-groups:
  - {name: Proxy, type: select, proxies: [node-0000]}
`

const defaultRules = `rules:
  - MATCH,DIRECT
`

func parseDocument(t *testing.T, body string) (*Config, error) {
	t.Helper()
	rawCfg, err := UnmarshalRawConfig([]byte(documentPrelude + body))
	require.NoError(t, err, "the document itself must be valid yaml")
	return ParseRawConfig(rawCfg)
}

// A sub-rule that points at another resolves, and the hop records the name it
// points at: that name is what the verification walk follows.
func TestParseSubRulesResolvesTheChainADocumentDeclares(t *testing.T) {
	cfg, err := parseDocument(t, defaultRules+`sub-rules:
  leaf:
    - DOMAIN,leaf.test,node-0000
    - MATCH,Proxy
  middle:
    - SUB-RULE,(NETWORK,tcp),leaf
`)
	require.NoError(t, err)

	require.Len(t, cfg.SubRules, 2)
	assert.Len(t, cfg.SubRules["leaf"], 2)
	require.Len(t, cfg.SubRules["middle"], 1)
	hop := cfg.SubRules["middle"][0]
	assert.Equal(t, C.SubRules, hop.RuleType())
	assert.Equal(t, "leaf", hop.Adapter())
}

func TestParseSubRulesRefusesASubRuleThatNamesItself(t *testing.T) {
	_, err := parseDocument(t, defaultRules+`sub-rules:
  loop:
    - SUB-RULE,(NETWORK,tcp),loop
`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "circular references")
}

// Two sub-rules pointing at each other are refused, but the walk starts from
// whichever name the map hands it first, so the path it prints is not fixed.
func TestParseSubRulesRefusesATwoStepCycle(t *testing.T) {
	_, err := parseDocument(t, defaultRules+`sub-rules:
  alpha:
    - SUB-RULE,(NETWORK,tcp),beta
  beta:
    - SUB-RULE,(NETWORK,udp),alpha
`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sub-rule error: circular references")
}

// A third sub-rule reached by two paths is a join, not a cycle: the walk tracks
// the path it came by, so a shared leaf is allowed to be shared.
func TestParseSubRulesLetsTwoSubRulesShareAThird(t *testing.T) {
	cfg, err := parseDocument(t, defaultRules+`sub-rules:
  shared:
    - MATCH,Proxy
  left:
    - SUB-RULE,(NETWORK,tcp),shared
  right:
    - SUB-RULE,(NETWORK,udp),shared
  both:
    - SUB-RULE,(DST-PORT,443),left
    - SUB-RULE,(DST-PORT,80),right
`)
	require.NoError(t, err)
	assert.Len(t, cfg.SubRules["both"], 2)
	assert.Len(t, cfg.SubRules["shared"], 1)
}

// A SUB-RULE line that names a sub-rule nobody declared fails while the rules
// are placed, before the walk ever runs.
func TestParseRulesRefusesASubRuleTargetThatWasNeverDeclared(t *testing.T) {
	_, err := parseDocument(t, `rules:
  - SUB-RULE,(NETWORK,tcp),nowhere
`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sub-rule [nowhere] not found")
}

func TestParseSubRulesRefusesAnEmptyName(t *testing.T) {
	_, err := parseDocument(t, defaultRules+`sub-rules:
  "":
    - MATCH,Proxy
`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sub-rule name is empty")
}

// The dangling-target branch of the walk cannot be reached from a document:
// such a hop fails earlier, while the rules are placed. Call the walk directly
// so the branch has an answer pinned.
func TestVerifySubRuleCircularReferencesNamesADanglingTarget(t *testing.T) {
	hop, err := rules.ParseRule("SUB-RULE", "(NETWORK,tcp)", "nowhere", nil, map[string][]C.Rule{})
	require.NoError(t, err)

	err = verifySubRuleCircularReferences("start", map[string][]C.Rule{"start": {hop}}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "[nowhere] not found")

	require.NoError(t, verifySubRule(map[string][]C.Rule{}), "nothing to verify verifies")
}

func TestParseRulesRefusesTheLinesItCannotPlace(t *testing.T) {
	for _, tc := range []struct {
		name string
		line string
		want string
	}{
		{"a line with no target at all", "MATCH", "format invalid"},
		{"a target that is neither proxy nor sub-rule", "MATCH,nowhere", "proxy [nowhere] not found"},
		{"a rule set nobody declared", "RULE-SET,ads,node-0000", "rule set [ads] not found"},
		{"a type the engine does not know", "BOGUS,x,node-0000", "unsupported rule type: BOGUS"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseDocument(t, "rules:\n  - "+tc.line+"\n")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

// A single non-ip value is a name to follow, not an address; the chain only
// resolves at lookup time, so the parse keeps the name it was given.
func TestParseHostsAnswersIPsAndTheNamesItCanFollow(t *testing.T) {
	cfg, err := parseDocument(t, defaultRules+`hosts:
  ip.test: 1.2.3.4
  multi.test: [1.2.3.4, 5.6.7.8]
  alias.test: real.test
  real.test: 9.9.9.9
`)
	require.NoError(t, err)

	node := cfg.Hosts.Search("ip.test")
	require.NotNil(t, node)
	assert.False(t, node.Data().IsDomain)
	assert.Len(t, node.Data().IPs, 1)

	node = cfg.Hosts.Search("multi.test")
	require.NotNil(t, node)
	assert.Len(t, node.Data().IPs, 2)

	node = cfg.Hosts.Search("alias.test")
	require.NotNil(t, node)
	assert.True(t, node.Data().IsDomain)
	assert.Equal(t, "real.test", node.Data().Domain)
}

func TestParseHostsRefusesACycleInDomainMapping(t *testing.T) {
	_, err := parseDocument(t, defaultRules+`hosts:
  a.test: b.test
  b.test: a.test
`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "there is a cycle in domain name mapping")
}

func TestParseHostsRefusesTheValuesItCannotRead(t *testing.T) {
	_, err := parseDocument(t, defaultRules+`hosts:
  bad.test: [1.2.3.4, not-an-ip]
`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not a valid value")

	_, err = parseDocument(t, defaultRules+`hosts:
  weird.test: {label: 1}
`)
	require.Error(t, err, "a mapping is neither a name nor an address list")
}

// The trie refuses an empty domain; one such key is skipped with a warning and
// the rest of the table still parses.
func TestParseHostsSkipsADomainTheTrieRefuses(t *testing.T) {
	cfg, err := parseDocument(t, defaultRules+`hosts:
  "": 1.2.3.4
  good.test: 1.2.3.4
`)
	require.NoError(t, err)

	node := cfg.Hosts.Search("good.test")
	require.NotNil(t, node)
	assert.Len(t, node.Data().IPs, 1)
}
