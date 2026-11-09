package provider

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/metacubex/mihomo/adapter"
	"github.com/metacubex/mihomo/adapter/outbound"
	"github.com/metacubex/mihomo/component/resource"
	C "github.com/metacubex/mihomo/constant"
)

// memberListParser reads the member names a test wrote, one per line.
func memberListParser() resource.Parser[[]C.Proxy] {
	return func(buf []byte) ([]C.Proxy, error) {
		var proxies []C.Proxy
		for _, name := range strings.Fields(string(buf)) {
			proxies = append(proxies, adapter.NewProxy(outbound.NewDirectWithOption(outbound.DirectOption{Name: name})))
		}
		return proxies, nil
	}
}

func writeMembers(t *testing.T, path string, members ...string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.Join(members, "\n")), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// ListGeneration is the only signal a host has that any provider's list moved
// without its asking, so it has to hold still between swaps and move on one.
func TestListGenerationHoldsStillAndMovesOnASwap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "members.yaml")
	writeMembers(t, path, "member-a", "member-b")

	hc := NewHealthCheck(nil, "", 0, 0, false, nil)
	pd, err := NewProxySetProvider("members", 0, nil, memberListParser(), resource.NewFileVehicle(path), hc)
	if err != nil {
		t.Fatalf("NewProxySetProvider: %v", err)
	}
	if err := pd.Initial(); err != nil {
		t.Fatalf("Initial: %v", err)
	}

	first := ListGeneration()
	if got := ListGeneration(); got != first {
		t.Errorf("a read moved the generation %d -> %d", first, got)
	}

	writeMembers(t, path, "member-a", "member-c")
	if err := pd.Update(); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got := ListGeneration(); got <= first {
		t.Errorf("generation = %d after a swap, want it past %d", got, first)
	}
}

// A provider reports its own state to the host, and the host reads the member
// names out of its document; both have to agree about what the list holds.
func TestProxySetProviderReportsItsOwnState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "members.yaml")
	writeMembers(t, path, "member-a", "member-b")

	hc := NewHealthCheck(nil, "", 0, 0, false, nil)
	pd, err := NewProxySetProvider("members", 0, nil, memberListParser(), resource.NewFileVehicle(path), hc)
	if err != nil {
		t.Fatalf("NewProxySetProvider: %v", err)
	}
	if err := pd.Initial(); err != nil {
		t.Fatalf("Initial: %v", err)
	}

	if got := pd.Count(); got != 2 {
		t.Errorf("count = %d, want the list it read", got)
	}
	if got := pd.Version(); got == 0 {
		t.Error("version = 0 after the initial read, so a caller cannot tell a list from no list")
	}
	names := make([]string, 0, pd.Count())
	for _, proxy := range pd.Proxies() {
		names = append(names, proxy.Name())
	}
	if !slices.Equal(names, []string{"member-a", "member-b"}) {
		t.Errorf("proxies = %v, want the list in file order", names)
	}

	// A touch is the recency a health check reads, not a content change, so it
	// must leave the generation where it is.
	before := ListGeneration()
	pd.Touch()
	if got := ListGeneration(); got != before {
		t.Errorf("a touch moved the generation %d -> %d", before, got)
	}

	if url := pd.HealthCheckURL(); url != "" {
		t.Errorf("health check url = %q, want none configured", url)
	}

	document := pd.AppendJSON(nil)
	for _, name := range names {
		if !strings.Contains(string(document), `"name":"`+name+`"`) {
			t.Errorf("document %s is missing %s", document, name)
		}
	}
	marshaled, err := pd.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	if !bytes.Equal(marshaled, document) {
		t.Errorf("MarshalJSON = %s, want the same document as AppendJSON", marshaled)
	}
}
