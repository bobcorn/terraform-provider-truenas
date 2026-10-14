// Copyright TrueNAS 2026
// SPDX-License-Identifier: MPL-2.0

package snapshot_task_run

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/truenas/terraform-provider-truenas/internal/client"
)

var _ action.Action = &Action{}
var _ action.ActionWithConfigure = &Action{}

// Action runs a periodic snapshot task now (pool.snapshottask.run). The method
// is version-dependent: synchronous on 26.0 but a job on 28.0 (probed live), so
// it is invoked via CallJob — which waits for completion on 28.0 (the snapshot
// is actually taken before the action returns) and falls back cleanly when the
// method returns synchronously on 26.0. A plain Call returned before the 28.0
// job finished.
type Action struct{ client *client.Client }

func New() action.Action { return &Action{} }

type model struct {
	ID types.Int64 `tfsdk:"id"`
}

func (a *Action) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_snapshot_task_run"
}

func (a *Action) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Runs a periodic snapshot task immediately (pool.snapshottask.run).",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Required:    true,
				Description: "Periodic snapshot task id (truenas_periodic_snapshot_task.<name>.id).",
			},
		},
	}
}

func (a *Action) Configure(_ context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("expected *client.Client, got %T", req.ProviderData))
		return
	}
	a.client = c
}

func (a *Action) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var cfg model
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if resp.SendProgress != nil {
		resp.SendProgress(action.InvokeProgressEvent{Message: "Running snapshot task…"})
	}
	if _, err := a.client.CallJob(ctx, "pool.snapshottask.run", buildParams(cfg)...); err != nil {
		resp.Diagnostics.AddError("Snapshot task run failed", err.Error())
		return
	}
	if resp.SendProgress != nil {
		resp.SendProgress(action.InvokeProgressEvent{Message: "Snapshot task started."})
	}
}

func buildParams(m model) []any {
	return []any{m.ID.ValueInt64()}
}
