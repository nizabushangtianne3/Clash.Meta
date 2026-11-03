package constant

import "testing"

func TestAdapterTypeStringCoversAllNames(t *testing.T) {
	for at := Direct; at <= GostRelay; at++ {
		if at.String() == "" || at.String() == "Unknown" {
			t.Fatalf("missing name for %d", int(at))
		}
	}
	if AdapterType(-1).String() != "Unknown" {
		t.Fatal("invalid adapter type must stay Unknown")
	}
}
