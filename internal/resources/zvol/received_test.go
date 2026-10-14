// Copyright TrueNAS 2026
// SPDX-License-Identifier: MPL-2.0

package zvol

import (
	"encoding/json"
	"testing"
)

// TestIsExplicitSource_Zvol: RECEIVED is explicit on a zvol too (the #44 fix
// extends to zvol, which the dataset-only PR missed). A replicated zvol's
// RECEIVED properties must be preserved, not read back as inherited.
func TestIsExplicitSource_Zvol(t *testing.T) {
	for src, want := range map[string]bool{
		"LOCAL": true, "RECEIVED": true, "INHERITED": false, "DEFAULT": false,
	} {
		if got := isExplicitSource(src); got != want {
			t.Errorf("isExplicitSource(%q) = %v, want %v", src, got, want)
		}
	}
	v := "OFF"
	if got := localStringOrInherit(zfsSourced{Value: &v, Source: "RECEIVED"}).ValueString(); got != "OFF" {
		t.Errorf("localStringOrInherit(RECEIVED) = %q, want OFF (preserved)", got)
	}
	if got := localStringOrInherit(zfsSourced{Value: &v, Source: "INHERITED"}).ValueString(); got != "INHERIT" {
		t.Errorf("localStringOrInherit(INHERITED) = %q, want INHERIT", got)
	}
	rz := zfsSourced{Parsed: json.RawMessage(`0`), Source: "RECEIVED"}
	if got := localInt(rz); got.IsNull() || got.ValueInt64() != 0 {
		t.Errorf("localInt(RECEIVED 0) = %v, want 0", got)
	}
}
