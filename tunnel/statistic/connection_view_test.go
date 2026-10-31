package statistic

import (
	"encoding/json"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	C "github.com/metacubex/mihomo/constant"
)

// The golden document was produced by encoding/json over the struct tags
// before the hand written encoder replaced it, so a difference here is a wire
// format break the host would see.

func goldenSnapshot() *Snapshot {
	metadata := C.Metadata{
		NetWork:      C.TCP,
		Type:         C.HTTP,
		SrcIP:        netip.MustParseAddr("192.168.100.17"),
		DstIP:        netip.MustParseAddr("fe80::1%eth0"),
		SrcGeoIP:     []string{"CN", "US"},
		DstGeoIP:     []string{},
		SrcIPASN:     "AS4134",
		SrcPort:      54321,
		DstPort:      65535,
		InIP:         netip.MustParseAddr("::ffff:10.10.10.10"),
		InName:       "mixed",
		InUser:       "user<one>&two",
		Host:         "example.com</script>",
		DNSMode:      C.DNSFakeIP,
		Uid:          1001,
		Process:      "chrome.exe\u2028line",
		ProcessPath:  `C:\Program Files\Google\Chrome\app.exe`,
		SpecialProxy: "Proxy",
		SpecialRules: "Rule",
		RemoteDst:    "198.18.0.1:443",
		DSCP:         34,
		SniffHost:    "sniff.example.com",
	}
	rich := &TrackerInfo{
		UUID:        uuid.Must(uuid.FromString("0f8fad5b-d9cb-469f-a165-70867728950e")),
		Start:       time.Date(2026, 10, 26, 3, 4, 5, 123456789, time.FixedZone("weird", 5*3600+1800)),
		Chain:       C.Chain{"DIRECT", "Proxy"},
		Rule:        "DOMAIN-SUFFIX,example.com",
		RulePayload: "payload",
	}
	rich.metadata = metadata
	rich.Metadata = &rich.metadata
	rich.UploadTotal.Store(9876543210)
	rich.DownloadTotal.Store(-42)

	zero := &TrackerInfo{
		Start:         time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC),
		Chain:         C.Chain{},
		ProviderChain: C.Chain{"one"},
	}
	zero.metadata = C.Metadata{}
	zero.Metadata = &zero.metadata

	unknown := &TrackerInfo{
		UUID:  uuid.Must(uuid.FromString("ffffffff-ffff-ffff-ffff-ffffffffffff")),
		Start: time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC),
	}
	unknown.metadata = C.Metadata{
		NetWork: C.NetWork(77),
		Type:    C.Type(88),
		DNSMode: C.DNSMode(99),
	}
	unknown.Metadata = &unknown.metadata

	return &Snapshot{
		DownloadTotal: 1234567890123,
		UploadTotal:   -987654321,
		Memory:        4096,
		Connections:   []*TrackerInfo{rich, zero, unknown, nil},
	}
}

func goldenDocument(t *testing.T) string {
	t.Helper()

	golden, err := os.ReadFile(filepath.Join("testdata", "connection-view-golden.json"))
	require.NoError(t, err)
	return string(golden)
}

func TestSnapshotAppendJSONMatchesTheGoldenDocument(t *testing.T) {
	assert.Equal(t, goldenDocument(t), string(goldenSnapshot().AppendJSON(nil)))
}

func TestAppendJSONStringMatchesEncodingJSON(t *testing.T) {
	values := []string{
		"", "plain", `quotes"and\backslash`, "tab\tnewline\nreturn\r",
		"<script>&</script>", "control\u0001\u001fchars", "del\u007fchar",
		"separators\u2028and\u2029too", "emoji\U0001F600ok", "latin1\u00e9accent",
		"invalidutf8\xffend", "\xff\xfe\xfd", "trailingtruncated\xe4\xb8",
	}

	for _, value := range values {
		expected, err := json.Marshal(value)
		require.NoError(t, err)
		assert.Equal(t, string(expected), string(appendJSONString(nil, value)), value)
	}
}

func TestAppendJSONIsAllocationFree(t *testing.T) {
	snapshot := goldenSnapshot()
	// The encoder itself must not allocate; the writers hand it a buffer they
	// reuse between polls.
	scratch := make([]byte, 0, 4096)
	allocs := testing.AllocsPerRun(100, func() {
		scratch = snapshot.AppendJSON(scratch[:0])
	})
	assert.Zero(t, allocs)
}
