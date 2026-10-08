package tfclient

import (
	"bytes"
	"context"
	"encoding/json"
	"math/big"
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

// ParameterReadbackAnnotation is an explicit opt-out for write-only offerings.
// It preserves Terraform's configuration tracking, without claiming broker
// verification. The default is authoritative readback.
const ParameterReadbackAnnotation = "serviceinstance.account.btp.crossplane.io/parameter-readback"

type writeOnlyProvider struct{ *cachingProvider }

func (p *writeOnlyProvider) Resources(ctx context.Context) []func() resource.Resource {
	return p.Provider.Resources(ctx)
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
	readback := &parameterReadback{}
	r.Resource.Read(context.WithValue(ctx, parameterReadbackKey{}, readback), req, resp)
	if resp.Diagnostics.HasError() || resp.State.Raw.IsNull() {
		return
	}
	if readback.parameters == nil {
		// There is nothing to verify when no parameter fields are configured.
		if prior.IsNull() || prior.ValueString() == "{}" {
			resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("parameters"), prior)...)
			return
		}
		detail := "No successful parameter response was received."
		if readback.err != nil {
			detail = readback.err.Error()
		}
		resp.Diagnostics.AddError("Cannot verify service instance parameters", detail)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("parameters"), jsontypes.NewNormalizedValue(string(readback.parameters)))...)
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
	if decodeParameterObject(desired.ValueString(), &want) != nil || decodeParameterObject(observed.ValueString(), &got) != nil || want == nil || got == nil {
		return
	}
	// Brokers can add defaults and read-only fields. Retaining prior state is a
	// valid Framework plan result when every configured field already matches.
	if configuredParametersMatch(want, got) {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("parameters"), observed)...)
	}
}

func decodeParameterObject(value string, object *map[string]any) error {
	decoder := json.NewDecoder(bytes.NewBufferString(value))
	decoder.UseNumber()
	return decoder.Decode(object)
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
		} else if !parameterValuesEqual(want, got) {
			return false
		}
	}
	return true
}

func parameterValuesEqual(want, got any) bool {
	// Normalized JSON numbers compare by value, including 2 versus 2.0,
	// without rounding large integers through float64.
	if a, ok := want.(json.Number); ok {
		b, ok := got.(json.Number)
		if !ok {
			return false
		}
		x, xok := new(big.Rat).SetString(a.String())
		y, yok := new(big.Rat).SetString(b.String())
		return xok && yok && x.Cmp(y) == 0
	}
	return reflect.DeepEqual(want, got)
}
