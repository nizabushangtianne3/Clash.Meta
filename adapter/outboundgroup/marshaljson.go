package outboundgroup

// A group document carries the group name so the host embeds it verbatim
// instead of decoding it to stamp the name back on.
type selectorView struct {
	Type          string   `json:"type"`
	Name          string   `json:"name"`
	Now           string   `json:"now"`
	All           []string `json:"all"`
	TestURL       string   `json:"testUrl"`
	Hidden        bool     `json:"hidden"`
	Icon          string   `json:"icon"`
	EmptyFallback string   `json:"emptyFallback"`
}

type probedView struct {
	Type           string   `json:"type"`
	Name           string   `json:"name"`
	Now            string   `json:"now"`
	All            []string `json:"all"`
	TestURL        string   `json:"testUrl"`
	ExpectedStatus string   `json:"expectedStatus"`
	Fixed          string   `json:"fixed"`
	Hidden         bool     `json:"hidden"`
	Icon           string   `json:"icon"`
	EmptyFallback  string   `json:"emptyFallback"`
}

type loadBalanceView struct {
	Type           string   `json:"type"`
	Name           string   `json:"name"`
	All            []string `json:"all"`
	TestURL        string   `json:"testUrl"`
	ExpectedStatus string   `json:"expectedStatus"`
	Hidden         bool     `json:"hidden"`
	Icon           string   `json:"icon"`
	EmptyFallback  string   `json:"emptyFallback"`
}
