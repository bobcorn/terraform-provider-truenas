// Copyright TrueNAS 2026
// SPDX-License-Identifier: MPL-2.0

package dataset

import (
	"encoding/json"
	"testing"
)

func mkSourced(src, val string) zfsSourced {
	v := val
	return zfsSourced{Value: &v, Parsed: json.RawMessage(`"` + val + `"`), Source: src}
}

// TestIsExplicitSource: LOCAL and RECEIVED are explicit (belong to this
// dataset); INHERITED and DEFAULT are not. This is the #44 fix — a replicated
// dataset's RECEIVED properties must be preserved, not treated as inherited.
func TestIsExplicitSource(t *testing.T) {
	for src, want := range map[string]bool{
		"LOCAL": true, "RECEIVED": true, "INHERITED": false, "DEFAULT": false, "": false,
	} {
		if got := isExplicitSource(src); got != want {
			t.Errorf("isExplicitSource(%q) = %v, want %v", src, got, want)
		}
	}
}

// TestLocalHelpers_ReceivedPreserved: the source-aware readers keep RECEIVED
// values (like LOCAL) instead of reporting INHERIT / null.
func TestLocalHelpers_ReceivedPreserved(t *testing.T) {
	// localStringOrInherit: RECEIVED value kept; INHERITED -> "INHERIT".
	if got := localStringOrInherit(mkSourced("RECEIVED", "PASSTHROUGH")).ValueString(); got != "PASSTHROUGH" {
		t.Errorf("localStringOrInherit(RECEIVED) = %q, want PASSTHROUGH", got)
	}
	if got := localStringOrInherit(mkSourced("INHERITED", "PASSTHROUGH")).ValueString(); got != "INHERIT" {
		t.Errorf("localStringOrInherit(INHERITED) = %q, want INHERIT", got)
	}
	// localString: RECEIVED kept; DEFAULT -> null.
	if got := localString(mkSourced("RECEIVED", "/mnt/tank/x")); got.IsNull() || got.ValueString() != "/mnt/tank/x" {
		t.Errorf("localString(RECEIVED) = %v, want /mnt/tank/x", got)
	}
	if got := localString(mkSourced("DEFAULT", "whatever")); !got.IsNull() {
		t.Errorf("localString(DEFAULT) = %v, want null", got)
	}
	// localInt: RECEIVED numeric kept (incl zero); INHERITED -> null.
	rz := zfsSourced{Parsed: json.RawMessage(`0`), Source: "RECEIVED"}
	if got := localInt(rz); got.IsNull() || got.ValueInt64() != 0 {
		t.Errorf("localInt(RECEIVED 0) = %v, want 0", got)
	}
	iz := zfsSourced{Parsed: json.RawMessage(`5`), Source: "INHERITED"}
	if got := localInt(iz); !got.IsNull() {
		t.Errorf("localInt(INHERITED) = %v, want null", got)
	}
}
