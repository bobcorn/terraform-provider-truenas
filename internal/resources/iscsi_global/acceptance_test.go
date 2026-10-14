// Copyright TrueNAS 2026
// SPDX-License-Identifier: MPL-2.0

package iscsi_global_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/truenas/terraform-provider-truenas/internal/acctest"
)

// TestAccISCSIGlobalDataSource_basic reads the current TrueNAS iSCSI global
// configuration through the truenas_iscsi_global datasource only. It never
// writes: the iSCSI global configuration serves live storage, and the box's
// real configuration must not be overwritten by an acceptance test run.
//
// Requires:
//   - TF_ACC=1
//   - TRUENAS_API_KEY (or TRUENAS_USERNAME+TRUENAS_PASSWORD) set for
//     acctest.PreCheck
//   - A running TrueNAS instance
func TestAccISCSIGlobalDataSource_basic(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("Acceptance tests skipped: set TF_ACC=1 and ensure a TrueNAS instance is available")
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: acctest.ProviderConfig() + `
data "truenas_iscsi_global" "test" {}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.truenas_iscsi_global.test", "id", "iscsi_global"),
					resource.TestCheckResourceAttrSet("data.truenas_iscsi_global.test", "basename"),
					resource.TestCheckResourceAttrSet("data.truenas_iscsi_global.test", "listen_port"),
				),
			},
		},
	})
}

// iscsiGlobalOriginal captures the one field
// TestAccISCSIGlobal_setAndRestore touches. It captures EVERY global field the
// test's Terraform config writes, not just pool_avail_threshold: the config
// sets basename/alua/iser/isns_servers/listen_port too, so the test must put
// all of them back. PoolAvailThreshold is nullable on the wire (nil means "no
// threshold configured"). basename in particular must be restored to the box's
// real value — overwriting it changes every target's IQN and can knock iSCSI
// clients (e.g. a Proxmox host booting a VM off a LUN) offline.
type iscsiGlobalOriginal struct {
	Basename           string   `json:"basename"`
	ListenPort         int64    `json:"listen_port"`
	ALUA               bool     `json:"alua"`
	ISER               bool     `json:"iser"`
	ISNSServers        []string `json:"isns_servers"`
	PoolAvailThreshold *int64   `json:"pool_avail_threshold"`
}

// readISCSIGlobalOriginal reads the box's current iSCSI global config via
// iscsi.global.config, so the test can build its config from the real values
// and restore them exactly afterward.
func readISCSIGlobalOriginal(t *testing.T) iscsiGlobalOriginal {
	t.Helper()
	raw, err := acctest.Client().Call(context.Background(), "iscsi.global.config")
	if err != nil {
		t.Fatalf("error reading current iscsi global config: %v", err)
	}
	var orig iscsiGlobalOriginal
	if err := json.Unmarshal(raw, &orig); err != nil {
		t.Fatalf("error parsing iscsi.global.config response: %v", err)
	}
	if orig.ISNSServers == nil {
		orig.ISNSServers = []string{}
	}
	return orig
}

// restoreISCSIGlobal sends every field the test writes back to its original
// value via iscsi.global.update. It runs from t.Cleanup, so it restores the
// box even if the Terraform steps fail partway through.
func restoreISCSIGlobal(t *testing.T, orig iscsiGlobalOriginal) {
	t.Helper()
	var pat any
	if orig.PoolAvailThreshold != nil {
		pat = *orig.PoolAvailThreshold
	}
	if _, err := acctest.Client().Call(context.Background(), "iscsi.global.update", map[string]any{
		"basename":             orig.Basename,
		"listen_port":          orig.ListenPort,
		"alua":                 orig.ALUA,
		"iser":                 orig.ISER,
		"isns_servers":         orig.ISNSServers,
		"pool_avail_threshold": pat,
	}); err != nil {
		t.Fatalf("error restoring iscsi global config: %v", err)
	}
}

// TestAccISCSIGlobal_setAndRestore drives the singleton
// truenas_iscsi_global resource's "pool_avail_threshold" field through a
// test value and back to the value read from the box before the test ran,
// then imports it. It requires TF_ACC=1 and TRUENAS_DISRUPTIVE=1
// (acctest.DisruptiveCheck), since it mutates the box's live iSCSI global
// configuration; a t.Cleanup-registered API restore is the safety net if
// the Terraform steps fail.
//
// Safety: the config sets basename/alua/iser/isns_servers/listen_port to the
// box's ORIGINAL values (read first), so applying changes none of them — only
// pool_avail_threshold, which is advisory-only (it controls the pool-capacity
// ALERT threshold and does not affect existing targets, extents, or active
// sessions). t.Cleanup restores every captured field regardless. This is the
// correct form of an earlier bug where the config hard-coded a basename and the
// restore omitted it, changing every target's IQN on the box.
//
// The model represents a null pool_avail_threshold as the state value 0
// (see responseToModel / updatePayload's three-way handling: explicit 0 in
// config sends JSON nil to the API). So when the original value is nil, the
// terraform-restore step below sets 0, which round-trips back to nil on the
// wire exactly like the box's original unset state.
func TestAccISCSIGlobal_setAndRestore(t *testing.T) {
	acctest.DisruptiveCheck(t)

	orig := readISCSIGlobalOriginal(t)
	t.Cleanup(func() { restoreISCSIGlobal(t, orig) })

	var origValue int64
	if orig.PoolAvailThreshold != nil {
		origValue = *orig.PoolAvailThreshold
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: acctest.ProviderConfig() + testAccISCSIGlobalConfig(orig, 80),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("truenas_iscsi_global.test", "id", "iscsi_global"),
					resource.TestCheckResourceAttr("truenas_iscsi_global.test", "pool_avail_threshold", "80"),
				),
			},
			{
				Config: acctest.ProviderConfig() + testAccISCSIGlobalConfig(orig, origValue),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("truenas_iscsi_global.test", "pool_avail_threshold", fmt.Sprintf("%d", origValue)),
				),
			},
			{
				ResourceName:      "truenas_iscsi_global.test",
				ImportState:       true,
				ImportStateId:     "iscsi_global",
				ImportStateVerify: true,
			},
		},
	})
}

// testAccISCSIGlobalConfig builds the resource config from the box's ORIGINAL
// global settings, varying only pool_avail_threshold (the field under test).
// basename/alua/iser/isns_servers/listen_port are set to their current values,
// so applying is a no-op for them and the box's iSCSI identity never changes.
func testAccISCSIGlobalConfig(orig iscsiGlobalOriginal, poolAvailThreshold int64) string {
	isns := "[]"
	if len(orig.ISNSServers) > 0 {
		quoted := make([]string, len(orig.ISNSServers))
		for i, s := range orig.ISNSServers {
			quoted[i] = fmt.Sprintf("%q", s)
		}
		isns = "[" + strings.Join(quoted, ", ") + "]"
	}
	return fmt.Sprintf(`
resource "truenas_iscsi_global" "test" {
  pool_avail_threshold = %d

  alua         = %t
  basename     = %q
  iser         = %t
  isns_servers = %s
  listen_port  = %d
}
`, poolAvailThreshold, orig.ALUA, orig.Basename, orig.ISER, isns, orig.ListenPort)
}
