package adapter

import "testing"

func TestProxyAppendJSONLengthStaysSmall(t *testing.T) {
	data, err := testProxy().MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if len(data) > 512 {
		t.Fatalf("proxy JSON length = %d, want <= 512", len(data))
	}
}
