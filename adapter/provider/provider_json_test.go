package provider

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/metacubex/mihomo/adapter"
	"github.com/metacubex/mihomo/adapter/outbound"
	"github.com/metacubex/mihomo/common/utils"
	C "github.com/metacubex/mihomo/constant"
	"github.com/stretchr/testify/require"
)

func newJSONTestProxy(name string) C.Proxy {
	return adapter.NewProxy(outbound.NewDirectWithOption(outbound.DirectOption{Name: name}))
}

func TestCompatibleProviderAppendJSONMatchesAPIContract(t *testing.T) {
	proxies := []C.Proxy{newJSONTestProxy("node"), newJSONTestProxy("other")}
	expectedStatus, err := utils.NewUnsignedRanges[uint16]("200,301")
	require.NoError(t, err)
	health := NewHealthCheck(proxies, "https://example.com/generate_204", 0, 0, true, expectedStatus)
	pd, err := NewCompatibleProvider("compatible", proxies, health)
	require.NoError(t, err)

	document := string(pd.AppendJSON(nil))
	var view struct {
		Name        string `json:"name"`
		Type        string `json:"type"`
		VehicleType string `json:"vehicleType"`
		Proxies     []struct {
			Name string `json:"name"`
		} `json:"proxies"`
		TestURL          string  `json:"testUrl"`
		ExpectedStatus   string  `json:"expectedStatus"`
		UpdatedAt        *string `json:"updatedAt"`
		SubscriptionInfo *string `json:"subscriptionInfo"`
	}
	require.NoError(t, json.Unmarshal([]byte(document), &view))
	require.Equal(t, "compatible", view.Name)
	require.Equal(t, "Proxy", view.Type)
	require.Equal(t, "Compatible", view.VehicleType)
	require.Len(t, view.Proxies, 2)
	require.Equal(t, "node", view.Proxies[0].Name)
	require.Equal(t, "other", view.Proxies[1].Name)
	require.Equal(t, "https://example.com/generate_204", view.TestURL)
	require.Equal(t, "200/301", view.ExpectedStatus)
	require.Nil(t, view.UpdatedAt)
	require.Nil(t, view.SubscriptionInfo)
}

func TestProviderJSONOmitsZeroTimeAndEncodesSubscription(t *testing.T) {
	updated := time.Date(2026, 11, 1, 12, 34, 56, 123456789, time.UTC)
	document := string(appendProviderJSON(nil, "proxy-set", "Proxy", "HTTP", nil,
		"https://example.com/generate_204", "200", updated,
		&SubscriptionInfo{Upload: 1, Download: 2, Total: 3, Expire: 4}))

	var view struct {
		UpdatedAt        time.Time `json:"updatedAt"`
		SubscriptionInfo *struct {
			Upload   int64 `json:"Upload"`
			Download int64 `json:"Download"`
			Total    int64 `json:"Total"`
			Expire   int64 `json:"Expire"`
		} `json:"subscriptionInfo"`
	}
	require.NoError(t, json.Unmarshal([]byte(document), &view))
	require.Equal(t, updated, view.UpdatedAt)
	require.NotNil(t, view.SubscriptionInfo)
	require.EqualValues(t, 1, view.SubscriptionInfo.Upload)
	require.EqualValues(t, 2, view.SubscriptionInfo.Download)
	require.EqualValues(t, 3, view.SubscriptionInfo.Total)
	require.EqualValues(t, 4, view.SubscriptionInfo.Expire)

	zero := string(appendProviderJSON(nil, "proxy-set", "Proxy", "HTTP", nil,
		"https://example.com/generate_204", "200", time.Time{}, nil))
	var omitted struct {
		UpdatedAt        *string `json:"updatedAt"`
		SubscriptionInfo *string `json:"subscriptionInfo"`
	}
	require.NoError(t, json.Unmarshal([]byte(zero), &omitted))
	require.Nil(t, omitted.UpdatedAt)
	require.Nil(t, omitted.SubscriptionInfo)
}
