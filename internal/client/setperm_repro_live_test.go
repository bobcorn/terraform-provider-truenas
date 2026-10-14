// Copyright TrueNAS 2026
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"testing"
)

// TestSetpermConcurrency_Serialized is the #45 regression: TrueNAS 25.10
// silently drops some filesystem.setperm jobs run concurrently (reproduced:
// 6/12 concurrent raw CallJob no-op, 12/12 when serialized). CallFilesystemMutation
// serializes them client-side, so firing N concurrently through it must apply
// all N. Gated on SETPERM_REPRO=1 (+ TF_ACC/TRUENAS_* and TRUENAS_TEST_POOL).
func TestSetpermConcurrency_Serialized(t *testing.T) {
	if os.Getenv("SETPERM_REPRO") == "" {
		t.Skip("set SETPERM_REPRO=1 (+ TF_ACC/TRUENAS_* and TRUENAS_TEST_POOL) to run")
	}
	pool := os.Getenv("TRUENAS_TEST_POOL")
	if pool == "" {
		t.Skip("set TRUENAS_TEST_POOL")
	}
	ctx := context.Background()
	c := liveClient(t)

	const n = 12
	base := fmt.Sprintf("%s/spr%d", pool, os.Getpid()%100000)
	mk := func(name string) string {
		if _, err := c.Call(ctx, "pool.dataset.create", map[string]any{"name": name, "type": "FILESYSTEM"}); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		raw, err := c.CallRead(ctx, "pool.dataset.get_instance", name)
		if err != nil {
			t.Fatalf("get_instance %s: %v", name, err)
		}
		var inst struct {
			Mountpoint string `json:"mountpoint"`
		}
		_ = json.Unmarshal(raw, &inst)
		return inst.Mountpoint
	}
	_ = mk(base)
	t.Cleanup(func() {
		_, _ = c.Call(context.Background(), "pool.dataset.delete", base, map[string]any{"recursive": true})
	})
	paths := make([]string, n)
	for i := range paths {
		paths[i] = mk(fmt.Sprintf("%s/p%d", base, i))
	}

	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range paths {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = c.CallFilesystemMutation(ctx, "filesystem.setperm", map[string]any{
				"path":    paths[i],
				"mode":    "750",
				"uid":     1001,
				"gid":     1001,
				"options": map[string]any{"recursive": false, "traverse": false},
			})
		}(i)
	}
	wg.Wait()

	applied := 0
	for i, p := range paths {
		if errs[i] != nil {
			t.Errorf("path %d: setperm error: %v", i, errs[i])
			continue
		}
		raw, err := c.CallRead(ctx, "filesystem.stat", p)
		if err != nil {
			t.Errorf("path %d: stat: %v", i, err)
			continue
		}
		var st struct {
			Mode int64 `json:"mode"`
			UID  int64 `json:"uid"`
		}
		_ = json.Unmarshal(raw, &st)
		if st.Mode&0o777 == 0o750 && st.UID == 1001 {
			applied++
		} else {
			t.Errorf("path %d: NOT applied — mode=%o uid=%d (want 750/1001)", i, st.Mode&0o777, st.UID)
		}
	}
	if applied != n {
		t.Fatalf("serialized setperm applied %d/%d (issue #45 not fixed)", applied, n)
	}
}
