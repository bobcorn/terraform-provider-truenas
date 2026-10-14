// Copyright TrueNAS 2026
// SPDX-License-Identifier: MPL-2.0

package user_test

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/truenas/terraform-provider-truenas/internal/acctest"
)

// TestAccUser_reservedPartialUpdate changes only the display name of an
// existing reserved administrator, then restores it and forgets the import.
// The unchanged reserved home must never be sent back through user.update.
func TestAccUser_reservedPartialUpdate(t *testing.T) {
	acctest.DisruptiveCheck(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	raw, err := acctest.Client().CallRead(ctx, "user.query", []any{[]any{"username", "=", "truenas_admin"}})
	if err != nil {
		t.Fatal(err)
	}
	var users []struct {
		ID                   int64    `json:"id"`
		Username             string   `json:"username"`
		FullName             string   `json:"full_name"`
		Home                 string   `json:"home"`
		Shell                string   `json:"shell"`
		Locked               bool     `json:"locked"`
		PasswordDisabled     bool     `json:"password_disabled"`
		Groups               []int64  `json:"groups"`
		SudoCommands         []string `json:"sudo_commands"`
		SudoCommandsNoPasswd []string `json:"sudo_commands_nopasswd"`
	}
	if err := json.Unmarshal(raw, &users); err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || !strings.HasPrefix(users[0].Home, "/home/") {
		t.Skip("Requires the existing truenas_admin account with a reserved /home path")
	}
	original := users[0]
	t.Cleanup(func() {
		cleanup, done := context.WithTimeout(context.Background(), time.Minute)
		defer done()
		if _, err := acctest.RestoreCall(cleanup, "user.update", original.ID, map[string]any{"full_name": original.FullName, "groups": original.Groups, "sudo_commands": original.SudoCommands, "sudo_commands_nopasswd": original.SudoCommandsNoPasswd}); err != nil {
			t.Errorf("restore reserved administrator display name: %v", err)
			return
		}
		raw, err := acctest.RestoreCall(cleanup, "user.get_instance", original.ID)
		if err != nil {
			t.Error(err)
			return
		}
		restored := original
		if err := json.Unmarshal(raw, &restored); err != nil {
			t.Error(err)
			return
		}
		if !reflect.DeepEqual(restored, original) {
			t.Error("Reserved administrator attributes were not restored")
		}
	})
	config := func(name string) string {
		return acctest.ProviderConfig() + fmt.Sprintf(`
import {
 to = truenas_user.test
 id = %q
}
resource "truenas_user" "test" {
 username = %q
 full_name = %q
 home = %q
 shell = %q
 lifecycle {
  prevent_destroy = true
 }
}
`, fmt.Sprint(original.ID), original.Username, name, original.Home, original.Shell)
	}
	checks := func(name string) resource.TestCheckFunc {
		checks := []resource.TestCheckFunc{
			resource.TestCheckResourceAttr("truenas_user.test", "full_name", name),
			resource.TestCheckResourceAttr("truenas_user.test", "home", original.Home),
			resource.TestCheckResourceAttr("truenas_user.test", "locked", fmt.Sprint(original.Locked)),
			resource.TestCheckResourceAttr("truenas_user.test", "password_disabled", fmt.Sprint(original.PasswordDisabled)),
			resource.TestCheckResourceAttr("truenas_user.test", "groups.#", fmt.Sprint(len(original.Groups))),
			resource.TestCheckResourceAttr("truenas_user.test", "sudo_commands.#", fmt.Sprint(len(original.SudoCommands))),
			resource.TestCheckResourceAttr("truenas_user.test", "sudo_commands_nopasswd.#", fmt.Sprint(len(original.SudoCommandsNoPasswd))),
		}
		for i, group := range original.Groups {
			checks = append(checks, resource.TestCheckResourceAttr("truenas_user.test", fmt.Sprintf("groups.%d", i), fmt.Sprint(group)))
		}
		return resource.ComposeTestCheckFunc(checks...)
	}
	updated := acctest.RandName("TF acceptance administrator")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.DisruptiveCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: config(original.FullName), Check: checks(original.FullName)},
			{Config: config(updated), Check: checks(updated), ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
				plancheck.ExpectResourceAction("truenas_user.test", plancheck.ResourceActionUpdate),
			}}},
			{Config: config(original.FullName), Check: checks(original.FullName)},
			// Never delete an existing administrator during framework teardown.
			{Config: acctest.ProviderConfig() + `
removed {
 from = truenas_user.test
 lifecycle {
  destroy = false
 }
}
`},
		},
	})
}
