// Copyright TrueNAS 2026
// SPDX-License-Identifier: MPL-2.0

package zvol

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestPreserveCompression guards the v1.5.12 compression fix on zvols (mirror of
// the dataset case): ZFS folds the level-1 fast tier to plain zstd-fast, so
// compression="ZSTD-FAST-1" must still round-trip when the API reports
// "zstd-fast"; real drift overwrites state with the lower-cased API value.
func TestPreserveCompression(t *testing.T) {
	cases := []struct {
		name    string
		current types.String
		apiVal  string
		want    string
	}{
		{"fast-1 alias kept", types.StringValue("ZSTD-FAST-1"), "zstd-fast", "ZSTD-FAST-1"},
		{"equal-fold casing kept", types.StringValue("ZSTD-5"), "zstd-5", "ZSTD-5"},
		{"real drift overwrites", types.StringValue("lz4"), "zstd", "zstd"},
		{"null current -> lowercased api", types.StringNull(), "lz4", "lz4"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := preserveCompression(tc.current, tc.apiVal).ValueString(); got != tc.want {
				t.Errorf("preserveCompression(%v, %q) = %q, want %q", tc.current, tc.apiVal, got, tc.want)
			}
		})
	}
}

// fakeStater drives waitForZvolDevice without a real server: it fails the first
// okAfter-1 stat calls (node absent) then succeeds, or never succeeds when
// okAfter is 0 (node never appears).
type fakeStater struct {
	calls   int
	okAfter int
}

func (f *fakeStater) CallRead(_ context.Context, _ string, _ ...any) (json.RawMessage, error) {
	f.calls++
	if f.okAfter > 0 && f.calls >= f.okAfter {
		return json.RawMessage(`{"type":"SYMLINK"}`), nil
	}
	return nil, errors.New("[ENOENT] path does not exist")
}

// TestWaitForZvolDevice covers the #40 device-settle wait deterministically —
// the lab hardware can't reproduce the real udev lag at the Terraform level, so
// this is the regression guard for the wait's logic itself.
func TestWaitForZvolDevice(t *testing.T) {
	oldTO, oldPI := zvolDeviceWaitTimeout, zvolDevicePollInterval
	zvolDeviceWaitTimeout, zvolDevicePollInterval = 2*time.Second, 5*time.Millisecond
	t.Cleanup(func() { zvolDeviceWaitTimeout, zvolDevicePollInterval = oldTO, oldPI })

	t.Run("returns promptly once the node appears", func(t *testing.T) {
		f := &fakeStater{okAfter: 4}
		start := time.Now()
		waitForZvolDevice(context.Background(), f, "tank/x")
		if f.calls < 4 {
			t.Fatalf("polled %d times, want at least 4 (should poll until the node appears)", f.calls)
		}
		if el := time.Since(start); el > 500*time.Millisecond {
			t.Fatalf("took %v, expected to return soon after the node appeared", el)
		}
	})

	t.Run("times out without hanging when the node never appears", func(t *testing.T) {
		f := &fakeStater{okAfter: 0}
		start := time.Now()
		waitForZvolDevice(context.Background(), f, "tank/x")
		el := time.Since(start)
		if el < zvolDeviceWaitTimeout {
			t.Fatalf("returned after %v, before the %v timeout", el, zvolDeviceWaitTimeout)
		}
		if el > zvolDeviceWaitTimeout+500*time.Millisecond {
			t.Fatalf("overran the timeout: %v", el)
		}
	})

	t.Run("honors context cancellation", func(t *testing.T) {
		f := &fakeStater{okAfter: 0}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		start := time.Now()
		waitForZvolDevice(ctx, f, "tank/x")
		if el := time.Since(start); el > 500*time.Millisecond {
			t.Fatalf("did not honor a cancelled context: ran %v", el)
		}
	})
}
