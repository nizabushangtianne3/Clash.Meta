package provider

import (
	"encoding/json"
	"testing"
	"time"

	P "github.com/metacubex/mihomo/constant/provider"
	"github.com/stretchr/testify/require"
)

func TestInlineRuleProviderAppendJSONMatchesAPIContract(t *testing.T) {
	updated := time.Date(2026, 11, 1, 12, 34, 56, 123456789, time.UTC)
	pd := NewInlineProvider("inline-rules", P.Domain, []string{"example.com", ".example.org"}, nil)
	inline := pd.(*InlineProvider)
	inline.updateAt = updated

	document := string(pd.AppendJSON(nil))
	var view struct {
		Behavior    string    `json:"behavior"`
		Format      *string   `json:"format"`
		Name        string    `json:"name"`
		RuleCount   int       `json:"ruleCount"`
		Type        string    `json:"type"`
		UpdatedAt   time.Time `json:"updatedAt"`
		VehicleType string    `json:"vehicleType"`
		Payload     []string  `json:"payload"`
	}
	require.NoError(t, json.Unmarshal([]byte(document), &view))
	require.Equal(t, "Domain", view.Behavior)
	require.Nil(t, view.Format)
	require.Equal(t, "inline-rules", view.Name)
	require.Equal(t, 2, view.RuleCount)
	require.Equal(t, "Rule", view.Type)
	require.Equal(t, updated, view.UpdatedAt)
	require.Equal(t, "Inline", view.VehicleType)
	require.Equal(t, []string{"example.com", ".example.org"}, view.Payload)
}
func TestRuleProviderJSONOmitsNilPayload(t *testing.T) {
	document := string(appendRuleProviderJSON(nil, "Domain", "MrsRule", "remote-rules", 3,
		"Rule", time.Date(2026, 11, 1, 12, 34, 56, 123456789, time.UTC), "HTTP", nil))
	var view struct {
		Payload []string `json:"payload"`
		Format  string   `json:"format"`
	}
	require.NoError(t, json.Unmarshal([]byte(document), &view))
	require.Nil(t, view.Payload)
	require.Equal(t, "MrsRule", view.Format)
}
