package tfclient

import (
	"context"
	"encoding/json"
	"reflect"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// serviceInstanceReadback adapts the embedded Terraform resource for Upjet.
// Terraform normally retains configured parameters during Read. Upjet can
// reconstruct that configuration from the desired spec after losing its cache,
// so retaining it would hide an update that has never reached the broker.
type serviceInstanceReadback struct {
	resource.Resource
}

func (r *serviceInstanceReadback) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.Resource.(resource.ResourceWithConfigure).Configure(ctx, req, resp)
}

func (r *serviceInstanceReadback) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	r.Resource.(resource.ResourceWithIdentity).IdentitySchema(ctx, req, resp)
}

func (r *serviceInstanceReadback) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	r.Resource.(resource.ResourceWithImportState).ImportState(ctx, req, resp)
}

func (r *serviceInstanceReadback) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var prior jsontypes.Normalized
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("parameters"), &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Trigger the Terraform resource's import readback path even for an existing
	// instance. Change only the request copy, never the managed resource's spec.
	resp.Diagnostics.Append(req.State.SetAttribute(ctx, path.Root("parameters"), jsontypes.NewNormalizedNull())...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.Resource.Read(ctx, req, resp)
	if resp.Diagnostics.HasError() || resp.State.Raw.IsNull() {
		return
	}
	var observed jsontypes.Normalized
	resp.Diagnostics.Append(resp.State.GetAttribute(ctx, path.Root("parameters"), &observed)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Some offerings do not expose parameters. Preserve their existing
	// Terraform behavior; absence of readback is not an empty broker object.
	if observed.IsNull() || observed.IsUnknown() || observed.ValueString() == "" {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("parameters"), prior)...)
	}
}

func (r *serviceInstanceReadback) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}
	var desired, observed jsontypes.Normalized
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("parameters"), &desired)...)
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("parameters"), &observed)...)
	if resp.Diagnostics.HasError() || desired.IsNull() || desired.IsUnknown() || observed.IsNull() || observed.IsUnknown() {
		return
	}
	var want, got map[string]any
	if json.Unmarshal([]byte(desired.ValueString()), &want) != nil || json.Unmarshal([]byte(observed.ValueString()), &got) != nil || want == nil || got == nil {
		return
	}
	// Brokers can add defaults and read-only fields. Retaining prior state is a
	// valid Framework plan result when every configured field already matches.
	if configuredParametersMatch(want, got) {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("parameters"), observed)...)
	}
}

func configuredParametersMatch(desired, observed map[string]any) bool {
	for key, want := range desired {
		got, exists := observed[key]
		if !exists {
			return false
		}
		if object, ok := want.(map[string]any); ok {
			actual, ok := got.(map[string]any)
			if !ok || !configuredParametersMatch(object, actual) {
				return false
			}
		} else if !reflect.DeepEqual(want, got) {
			return false
		}
	}
	return true
}
