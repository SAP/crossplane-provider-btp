package tfclient

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
)

// tfstateOneInstance is the canonical single-resource tfstate fixture.
func tfstateOneInstance(typ string, attrs map[string]any) map[string]any {
	return map[string]any{
		"version":           float64(4),
		"terraform_version": "1.3.9",
		"serial":            float64(1),
		"lineage":           "test-lineage",
		"outputs":           map[string]any{},
		"resources": []any{
			map[string]any{
				"mode":     "managed",
				"type":     typ,
				"name":     "main",
				"provider": "provider[\"registry.terraform.io/sap/btp\"]",
				"instances": []any{
					map[string]any{
						"schema_version": float64(0),
						"attributes":     attrs,
					},
				},
			},
		},
	}
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func firstInstance(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var s map[string]any
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return s["resources"].([]any)[0].(map[string]any)["instances"].([]any)[0].(map[string]any)
}

func TestMutateState_PerResourceType(t *testing.T) {
	cases := []struct {
		name     string
		typ      string
		attrs    map[string]any
		wantID   map[string]any
		wantSkip bool
	}{
		{
			name: "SubaccountTrustConfiguration",
			typ:  "btp_subaccount_trust_configuration",
			attrs: map[string]any{
				"subaccount_id": "sub-uuid",
				"id":            "ldap-corp", // origin is sourced from `id`
				"description":   "anything",
			},
			wantID: map[string]any{"subaccount_id": "sub-uuid", "origin": "ldap-corp"},
		},
		{
			name:   "GlobalaccountTrustConfiguration",
			typ:    "btp_globalaccount_trust_configuration",
			attrs:  map[string]any{"id": "ga-ldap", "name": "global"}, // origin sources from id
			wantID: map[string]any{"origin": "ga-ldap"},
		},
		{
			name: "DirectoryEntitlement",
			typ:  "btp_directory_entitlement",
			attrs: map[string]any{
				"directory_id": "dir-uuid",
				"service_name": "feature-flags",
				"plan_name":    "standard",
				"amount":       float64(1),
			},
			wantID: map[string]any{"directory_id": "dir-uuid", "service_name": "feature-flags", "plan_name": "standard"},
		},
		{
			name:   "ServiceInstance",
			typ:    "btp_subaccount_service_instance",
			attrs:  map[string]any{"subaccount_id": "sub-uuid", "id": "svc-uuid", "name": "svc"},
			wantID: map[string]any{"subaccount_id": "sub-uuid", "id": "svc-uuid"},
		},
		{
			name:   "ServiceBinding",
			typ:    "btp_subaccount_service_binding",
			attrs:  map[string]any{"subaccount_id": "sub-uuid", "id": "bind-uuid"},
			wantID: map[string]any{"subaccount_id": "sub-uuid", "id": "bind-uuid"},
		},
		{
			name:   "ServiceBroker",
			typ:    "btp_subaccount_service_broker",
			attrs:  map[string]any{"subaccount_id": "sub-uuid", "id": "brk-uuid"},
			wantID: map[string]any{"subaccount_id": "sub-uuid", "id": "brk-uuid"},
		},
		{
			name:     "ApiCredential_NoOp",
			typ:      "btp_subaccount_api_credential",
			attrs:    map[string]any{"subaccount_id": "sub-uuid", "name": "creds"},
			wantSkip: true,
		},
		{
			name:     "UnknownType_NoOp",
			typ:      "btp_some_resource_we_dont_wrap",
			attrs:    map[string]any{"id": "x"},
			wantSkip: true,
		},
	}

	log := logging.NewNopLogger()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := mustMarshal(t, tfstateOneInstance(tc.typ, tc.attrs))
			got, changed := mutateState(input, log)

			if tc.wantSkip {
				if changed {
					t.Fatalf("expected no mutation for %s; got %s", tc.typ, got)
				}
				return
			}
			if !changed {
				t.Fatalf("expected mutation for %s; got passthrough", tc.typ)
			}
			inst := firstInstance(t, got)
			gotMap, ok := inst["identity"].(map[string]any)
			if !ok {
				t.Fatalf("identity missing or not an object: %v", inst["identity"])
			}
			for k, want := range tc.wantID {
				if gotV := gotMap[k]; gotV != want {
					t.Errorf("identity[%q] = %v, want %v", k, gotV, want)
				}
			}
			if len(gotMap) != len(tc.wantID) {
				t.Errorf("identity has %d keys, want %d (%v)", len(gotMap), len(tc.wantID), gotMap)
			}
			if v, ok := inst["identity_schema_version"]; !ok || v != float64(0) {
				t.Errorf("identity_schema_version = %v (present=%v), want 0", v, ok)
			}
		})
	}
}

func TestMutateState_MalformedJSON(t *testing.T) {
	got, changed := mutateState([]byte("{not json"), logging.NewNopLogger())
	if changed {
		t.Fatal("malformed input should not report mutation")
	}
	if string(got) != "{not json" {
		t.Errorf("malformed input should pass through unchanged, got %q", got)
	}
}

func TestMutateState_EmptyIdentityAttrsStillInject(t *testing.T) {
	// Empty source attribute should still be emitted into identity (e.g.
	// upjet's placeholder phase before Create — per upstream guidance in
	// SAP/terraform-provider-btp#1532, the framework wants the block even
	// with placeholder values).
	input := mustMarshal(t, tfstateOneInstance("btp_subaccount_trust_configuration", map[string]any{
		"subaccount_id": "",
		"id":            "ldap-corp",
	}))
	got, changed := mutateState(input, logging.NewNopLogger())
	if !changed {
		t.Fatalf("expected mutation even with empty subaccount_id, got passthrough")
	}
	inst := firstInstance(t, got)
	identity, ok := inst["identity"].(map[string]any)
	if !ok {
		t.Fatalf("identity missing: %v", inst)
	}
	if identity["subaccount_id"] != "" {
		t.Errorf("subaccount_id should pass empty string through, got %v", identity["subaccount_id"])
	}
	if identity["origin"] != "ldap-corp" {
		t.Errorf("origin should source from id, got %v", identity["origin"])
	}
}

func TestMutateState_MissingSourceAttrBecomesEmptyString(t *testing.T) {
	// Missing source attribute (key not in state at all) should be emitted
	// as empty string rather than skipping the whole resource.
	input := mustMarshal(t, tfstateOneInstance("btp_directory_entitlement", map[string]any{
		"directory_id": "dir-uuid",
		"service_name": "feature-flags",
		// plan_name absent
	}))
	got, changed := mutateState(input, logging.NewNopLogger())
	if !changed {
		t.Fatalf("expected mutation, got passthrough")
	}
	inst := firstInstance(t, got)
	identity, ok := inst["identity"].(map[string]any)
	if !ok {
		t.Fatalf("identity missing: %v", inst)
	}
	if identity["plan_name"] != "" {
		t.Errorf("plan_name should be empty string when source attr missing, got %v", identity["plan_name"])
	}
}

func TestMutateState_MultipleInstances(t *testing.T) {
	state := map[string]any{
		"version":           float64(4),
		"terraform_version": "1.3.9",
		"serial":            float64(1),
		"lineage":           "test",
		"outputs":           map[string]any{},
		"resources": []any{
			map[string]any{
				"mode": "managed", "type": "btp_subaccount_trust_configuration", "name": "main",
				"provider": "p",
				"instances": []any{
					map[string]any{
						"schema_version": float64(0),
						"attributes":     map[string]any{"subaccount_id": "sub-A", "id": "ldap-A"},
					},
					map[string]any{
						"schema_version": float64(0),
						"attributes":     map[string]any{"subaccount_id": "sub-B", "id": "ldap-B"},
					},
				},
			},
		},
	}
	got, changed := mutateState(mustMarshal(t, state), logging.NewNopLogger())
	if !changed {
		t.Fatal("expected mutation")
	}
	var out map[string]any
	_ = json.Unmarshal(got, &out)
	instances := out["resources"].([]any)[0].(map[string]any)["instances"].([]any)
	for i, instAny := range instances {
		inst := instAny.(map[string]any)
		id, ok := inst["identity"].(map[string]any)
		if !ok || id["origin"] == nil || id["subaccount_id"] == nil {
			t.Errorf("instance %d: incomplete identity %v", i, inst["identity"])
		}
	}
}

func TestMutateState_MixedResources(t *testing.T) {
	state := map[string]any{
		"version":           float64(4),
		"terraform_version": "1.3.9",
		"serial":            float64(1),
		"lineage":           "test",
		"outputs":           map[string]any{},
		"resources": []any{
			map[string]any{
				"mode": "managed", "type": "btp_subaccount_trust_configuration", "name": "trust",
				"provider": "p",
				"instances": []any{map[string]any{
					"schema_version": float64(0),
					"attributes":     map[string]any{"subaccount_id": "s", "id": "o"},
				}},
			},
			map[string]any{
				"mode": "managed", "type": "btp_subaccount_api_credential", "name": "creds",
				"provider": "p",
				"instances": []any{map[string]any{
					"schema_version": float64(0),
					"attributes":     map[string]any{"id": "x"},
				}},
			},
			map[string]any{
				"mode": "managed", "type": "some_unknown_type", "name": "u",
				"provider": "p",
				"instances": []any{map[string]any{
					"schema_version": float64(0),
					"attributes":     map[string]any{"id": "y"},
				}},
			},
		},
	}
	got, _ := mutateState(mustMarshal(t, state), logging.NewNopLogger())
	var out map[string]any
	_ = json.Unmarshal(got, &out)
	resources := out["resources"].([]any)
	if _, ok := resources[0].(map[string]any)["instances"].([]any)[0].(map[string]any)["identity"]; !ok {
		t.Error("trust config: identity missing")
	}
	if _, ok := resources[1].(map[string]any)["instances"].([]any)[0].(map[string]any)["identity"]; ok {
		t.Error("api_credential: identity should not be present")
	}
	if _, ok := resources[2].(map[string]any)["instances"].([]any)[0].(map[string]any)["identity"]; ok {
		t.Error("unknown type: identity should not be present")
	}
}

func TestMutateState_Idempotent(t *testing.T) {
	// Re-mutating an already-injected state must report no change and return
	// the bytes unchanged, so reconciles don't keep rewriting the file.
	input := mustMarshal(t, tfstateOneInstance("btp_subaccount_trust_configuration", map[string]any{
		"subaccount_id": "sub-uuid",
		"id":            "ldap",
	}))
	first, changed := mutateState(input, logging.NewNopLogger())
	if !changed {
		t.Fatal("first run should mutate")
	}
	second, changedAgain := mutateState(first, logging.NewNopLogger())
	if changedAgain {
		t.Errorf("second run reported change; mutation is not idempotent")
	}
	if string(first) != string(second) {
		t.Errorf("second-run bytes differ:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
}

func TestEqualIdentity(t *testing.T) {
	a := map[string]any{"x": "1", "y": float64(2)}
	b := map[string]any{"x": "1", "y": float64(2)}
	c := map[string]any{"x": "1", "y": float64(3)}
	d := map[string]any{"x": "1"}
	if !equalIdentity(a, b) {
		t.Error("equal maps should compare equal")
	}
	if equalIdentity(a, c) {
		t.Error("differing value should compare unequal")
	}
	if equalIdentity(a, d) {
		t.Error("differing length should compare unequal")
	}
}

// On-disk integration tests: patchStateFile reads tfstate from disk, runs
// mutateState, and writes back. Verify end-to-end through the real
// filesystem (t.TempDir()) — same path the runtime uses.

func TestPatchStateFile_InjectsIntoExistingFile(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "terraform.tfstate")
	input := mustMarshal(t, tfstateOneInstance("btp_subaccount_trust_configuration", map[string]any{
		"subaccount_id": "sub-uuid",
		"id":            "ldap-corp",
	}))
	if err := os.WriteFile(statePath, input, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := patchStateFile(statePath, logging.NewNopLogger()); err != nil {
		t.Fatalf("patchStateFile: %v", err)
	}

	got, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	inst := firstInstance(t, got)
	if _, ok := inst["identity"]; !ok {
		t.Errorf("identity not injected on disk; stored = %s", got)
	}
}

func TestPatchStateFile_MissingFileIsNoError(t *testing.T) {
	// Workspace() may be invoked before EnsureTFState writes the state file;
	// no file → no error, the next reconcile will pick it up.
	dir := t.TempDir()
	missing := filepath.Join(dir, "terraform.tfstate")
	if err := patchStateFile(missing, logging.NewNopLogger()); err != nil {
		t.Errorf("missing state should not error, got %v", err)
	}
}

func TestPatchStateFile_NoChangeLeavesFileUntouched(t *testing.T) {
	// A tfstate whose only resource has no IdentitySchema should be a no-op:
	// patchStateFile must not rewrite the file (preserves serial / mtime
	// semantics for unrelated code paths).
	dir := t.TempDir()
	statePath := filepath.Join(dir, "terraform.tfstate")
	input := mustMarshal(t, tfstateOneInstance("btp_subaccount_api_credential", map[string]any{
		"id": "creds",
	}))
	if err := os.WriteFile(statePath, input, 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(statePath)
	if err != nil {
		t.Fatal(err)
	}

	if err := patchStateFile(statePath, logging.NewNopLogger()); err != nil {
		t.Fatalf("patchStateFile: %v", err)
	}

	got, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(input) {
		t.Errorf("no-change path rewrote file; before=%q after=%q", input, got)
	}
	after, err := os.Stat(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if !before.ModTime().Equal(after.ModTime()) {
		t.Errorf("mtime changed despite no-op patch: before=%v after=%v", before.ModTime(), after.ModTime())
	}
}

func TestPatchStateFile_MalformedJSONIsNoOp(t *testing.T) {
	// Junk bytes must not be overwritten. patchStateFile should swallow the
	// parse failure and return nil (best-effort contract).
	dir := t.TempDir()
	statePath := filepath.Join(dir, "terraform.tfstate")
	junk := []byte("{not json")
	if err := os.WriteFile(statePath, junk, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := patchStateFile(statePath, logging.NewNopLogger()); err != nil {
		t.Errorf("malformed state should be swallowed, got %v", err)
	}
	got, _ := os.ReadFile(statePath)
	if string(got) != string(junk) {
		t.Errorf("malformed file was rewritten: got %q, want %q", got, junk)
	}
}

func TestPatchStateFile_PreservesPermissions(t *testing.T) {
	// patchStateFile should overwrite with the original file's mode, not the
	// default umask. tfstate files live at 0600 in production; we don't want
	// to loosen that on rewrite.
	dir := t.TempDir()
	statePath := filepath.Join(dir, "terraform.tfstate")
	input := mustMarshal(t, tfstateOneInstance("btp_subaccount_service_instance", map[string]any{
		"subaccount_id": "sub-uuid",
		"id":            "svc-uuid",
	}))
	if err := os.WriteFile(statePath, input, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := patchStateFile(statePath, logging.NewNopLogger()); err != nil {
		t.Fatalf("patchStateFile: %v", err)
	}

	info, err := os.Stat(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("mode = %v, want 0600", got)
	}
}
