package executor_test

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/metacubex/mihomo/config"
	"github.com/metacubex/mihomo/hub/executor"
)

// applyConfig used to skip the inbound listener creation entirely after the
// upstream refactor commented out updateListeners - the standalone binary came
// up with only the external controller and ignored every proxy port. This test
// pins the startup: the mixed listener from the config must serve a proxied
// request through the full tunnel-match-dial-relay chain.
func TestApplyConfigStartsInboundListeners(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	// The probe port is released before the config applies it, so a busy runner
	// can swallow the bind. Retry the whole startup with a fresh port instead of
	// failing on one unlucky allocation.
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		lastErr = startInboundAndRequest(t, target.URL)
		if lastErr == nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("the proxied request failed - the inbound listener did not start or the tunnel rejected it: %v", lastErr)
}

func startInboundAndRequest(t *testing.T, targetURL string) error {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()

	proxyURL, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
	if err != nil {
		return err
	}

	configYAML := fmt.Sprintf(`
mixed-port: %d
find-process-mode: "off"
log-level: "silent"
proxies:
  - name: DIRECT-A
    type: direct
proxy-groups:
  - name: PROXY
    type: select
    proxies: [DIRECT-A]
rules:
  - MATCH,PROXY
`, port)

	cfg, err := config.Parse([]byte(configYAML))
	if err != nil {
		return err
	}
	executor.ApplyConfig(cfg, true)
	t.Cleanup(func() {
		empty, err := config.Parse([]byte("log-level: silent"))
		if err == nil {
			executor.ApplyConfig(empty, true)
		}
	})

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		},
	}
	resp, err := client.Get(targetURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status = %d, want 200", resp.StatusCode)
	}
	return nil
}
