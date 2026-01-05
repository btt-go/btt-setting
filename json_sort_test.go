package bttsetting

import (
	"encoding/json"
	"testing"
)

// TestJSONMarshalMapKeyOrderDeterminism verifies that encoding/json deterministically sorts map keys.
// We repeat this 10,000 times to ensure stability.
func TestJSONMarshalMapKeyOrderDeterminism(t *testing.T) {
	// Expected JSON output with keys sorted alphabetically
	expected := `{"apple":2,"banana":4,"mango":3,"zebra":1}`

	for i := 0; i < 10000; i++ {
		// Construct the map. Go maps are unordered and iteration order is randomized.
		m := map[string]int{
			"zebra":  1,
			"apple":  2,
			"mango":  3,
			"banana": 4,
		}

		b, err := json.Marshal(m)
		if err != nil {
			t.Fatalf("iteration %d: failed to marshal: %v", i, err)
		}

		got := string(b)
		if got != expected {
			t.Fatalf("iteration %d: non-deterministic output detected.\nExpected: %s\nGot:      %s", i, expected, got)
		}
	}
}
