package outbound

import (
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewShadowSocksSharesPluginDecoderAcrossGoroutines(t *testing.T) {
	option := ShadowSocksOption{
		Name:       "node",
		Server:     "10.0.0.1",
		Port:       443,
		Cipher:     "aes-128-gcm",
		Password:   "password",
		Plugin:     "obfs",
		PluginOpts: map[string]any{"mode": "http"},
	}

	const workers = 8
	const builds = 8
	errs := make([]error, workers)

	var wg sync.WaitGroup
	for worker := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range builds {
				proxy, err := NewShadowSocks(option)
				if err == nil {
					if name := proxy.Name(); name != "node" {
						err = errors.New("unexpected proxy name")
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
