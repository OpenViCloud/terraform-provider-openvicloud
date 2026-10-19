package datasource

import (
	"encoding/json"
	"testing"
)

func TestExact_KeepsEveryDigitTheApiSent(t *testing.T) {
	price := json.Number("0.0012345678")

	if got := exact(&price).ValueString(); got != "0.0012345678" {
		t.Fatalf("exact = %q", got)
	}
	if !exact(nil).IsNull() {
		t.Fatal("a missing number must be null, not 0")
	}
}

func TestText_And_Whole_MissingIsNull(t *testing.T) {
	if !text(nil).IsNull() || !whole(nil).IsNull() {
		t.Fatal("missing values must be null")
	}
	name := "Ubuntu"
	size := int32(20)
	if text(&name).ValueString() != "Ubuntu" || whole(&size).ValueInt64() != 20 {
		t.Fatal("present values must be kept")
	}
}
