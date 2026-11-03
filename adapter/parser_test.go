package adapter

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseProxySharesDecoderAcrossGoroutines(t *testing.T) {
	mapping := map[string]any{
		"name":     "node",
		"type":     "ss",
		"server":   "10.0.0.1",
		"port":     443,
		"cipher":   "aes-128-gcm",
		"password": "password",
	}

	const workers = 8
	const parses = 16
	errs := make([]error, workers)

	var wg sync.WaitGroup
	for worker := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range parses {
				proxy, err := ParseProxy(mapping, nil)
				if err == nil {
					if name := proxy.Name(); name != "node" {
						err = assert.AnError
					}
				}
				errs[worker] = err
			}
		}()
	}
	wg.Wait()

	for _, err := range errs {
		require.NoError(t, err)
	}
}
