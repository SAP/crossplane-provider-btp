// Package tfclient — identity_injector.go patches an `identity` block into
// the on-disk `terraform.tfstate` for each upjet-managed BTP resource so the
// terraform-plugin-framework's post-Read identity check is satisfied.
// Workaround for https://github.com/SAP/crossplane-provider-btp/issues/521.
//
// Why this exists: starting in BTP TF provider v1.19.0 every managed resource
// declares an IdentitySchema(). plugin-framework checks after every Read that
// resp.NewIdentity is non-nil; it pre-seeds NewIdentity from the prior
// state's `identity` block (req.CurrentIdentity). Upjet's pinned StateV4
// struct has no identity field, so the state upjet writes never carries
// identity, the framework check fires with "Missing Resource Identity After
// Read", and every refresh fails. See ISSUE-521-tracking.md for the full
// diagnosis.
//
// How it works: we wrap upjet's `controller.Store` (a single-method
// interface — `Workspace(ctx, …) (*terraform.Workspace, error)`). On every
// Workspace() call we delegate, then patch `terraform.tfstate` on disk at
// `filepath.Join(os.TempDir(), string(tr.GetUID()))` via plain `os.ReadFile`
// + `os.WriteFile`. The terraform CLI subprocess reads the patched file when
// it runs refresh/apply, so req.CurrentIdentity is populated and the
// framework auto-seeds resp.NewIdentity. Whatever the CLI writes back after
// refresh gets re-patched on the next reconcile.
//
// Why not an afero.Fs middleware via `terraform.WithFs(...)`: in upjet v2.2.0,
// `WithFs` only sets `WorkspaceStore.fs` — used for MkdirAll/Stat/RemoveAll
// on the workspace dir and for *reading* tfstate back. The actual write of
// `terraform.tfstate` happens in `FileProducer.EnsureTFState()`, and
// `WorkspaceStore.Workspace()` constructs the FileProducer without passing
// the Fs through (`NewFileProducer` is called without `WithFs`). FileProducer
// falls back to `afero.NewOsFs()` and the wrapper is never invoked. Plus the
// CLI subprocess writes go directly to the OS filesystem regardless. The
// Store-level hook is the only path that works without forking upjet.
//
// Best-effort: any failure (malformed JSON, partial identity attrs, missing
// state file) logs a Debug line and the reconcile proceeds. Worst case is
// the same framework error we'd hit without the injector.
//
// Removal: when no-fork (PR #680, issue #207) lands, this file and the two
// call-site wraps can be deleted outright. Nothing else depends on it.
package tfclient

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/crossplane/upjet/v2/pkg/config"
	tjcontroller "github.com/crossplane/upjet/v2/pkg/controller"
	"github.com/crossplane/upjet/v2/pkg/resource"
	"github.com/crossplane/upjet/v2/pkg/terraform"
)

// identityFields lists, per terraform resource type, the framework-declared
// resource identity for that resource: a map from each identity attribute
// name to the state attribute name that supplies the value. Most fields are
// trivial passthroughs (key == value); the exceptions are the trust
// configuration resources, where the identity field `origin` is sourced from
// the state attribute `id` (the BTP provider treats them as the same logical
// identifier — schema: `origin` is Optional + Computed and the Read path sets
// state.Origin = updatedState.Id).
//
// Resources absent from the map have no IdentitySchema and are skipped
// (e.g. btp_subaccount_api_credential).
//
// Why we emit identity even when source attributes are empty / placeholder
// values: per the upstream maintainer's guidance in
// https://github.com/SAP/terraform-provider-btp/issues/1532, the framework's
// "Missing Resource Identity After Read" check is satisfied by any identity
// block, including ones with placeholder values like `"id": "not-to-be-found"`
// that upjet writes when the resource doesn't yet exist. Skipping on empty
// values would mean the first reconcile (where attrs haven't been populated
// by a successful Read yet) never gets an identity block injected — exactly
// the case where the check fires.
var identityFields = map[string]map[string]string{
	"btp_subaccount_trust_configuration":    {"subaccount_id": "subaccount_id", "origin": "id"},
	"btp_globalaccount_trust_configuration": {"origin": "id"},
	"btp_directory_entitlement":             {"directory_id": "directory_id", "service_name": "service_name", "plan_name": "plan_name"},
	"btp_subaccount_service_instance":       {"subaccount_id": "subaccount_id", "id": "id"},
	"btp_subaccount_service_binding":        {"subaccount_id": "subaccount_id", "id": "id"},
	"btp_subaccount_service_broker":         {"subaccount_id": "subaccount_id", "id": "id"},
}

// tfStateFilename is the basename upjet uses for state in every workspace
// (see github.com/crossplane/upjet/v2/pkg/terraform/files.go).
const tfStateFilename = "terraform.tfstate"

// identityInjectingStore wraps a controller.Store and patches the on-disk
// terraform.tfstate after each Workspace() call.
type identityInjectingStore struct {
	inner  tjcontroller.Store
	logger logging.Logger
}

// NewIdentityInjectingStore wraps inner so that every Workspace() call
// patches `identity` into terraform.tfstate before returning. Pass the
// result wherever a tjcontroller.Store is expected (e.g.
// tjcontroller.NewConnector's second arg).
func NewIdentityInjectingStore(inner tjcontroller.Store, log logging.Logger) tjcontroller.Store {
	return &identityInjectingStore{inner: inner, logger: log}
}

// Workspace delegates to the wrapped store, then patches the workspace's
// terraform.tfstate. Failures in the patch step are logged and swallowed —
// the workspace pointer (and any error from the delegate) is returned
// unchanged so callers see the same behaviour as without the injector.
func (s *identityInjectingStore) Workspace(ctx context.Context, c resource.SecretClient, tr resource.Terraformed, ts terraform.Setup, cfg *config.Resource) (*terraform.Workspace, error) {
	ws, err := s.inner.Workspace(ctx, c, tr, ts, cfg)
	if err != nil {
		return ws, err
	}
	// Workspace dir layout from upjet's WorkspaceStore.Workspace() (store.go:223):
	// filepath.Join(os.TempDir(), string(tr.GetUID())). We can't read the dir
	// off the returned *Workspace (private field) but we can reconstruct it
	// from the same inputs.
	dir := filepath.Join(os.TempDir(), string(tr.GetUID()))
	if patchErr := patchStateFile(filepath.Join(dir, tfStateFilename), s.logger); patchErr != nil {
		s.logger.Debug("identity injector: patch failed, proceeding without injection",
			"workspace", dir, "error", patchErr.Error())
	}
	return ws, nil
}

// patchStateFile reads the tfstate at path, runs mutateState, and writes
// back only if the bytes actually changed. Missing files are not an error —
// EnsureTFState writes the initial state, but Workspace() may be invoked
// before that for some code paths; in that case there's nothing to patch.
func patchStateFile(path string, log logging.Logger) error {
	raw, err := os.ReadFile(path) //nolint:gosec // path is within our managed tmpdir
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// Nothing to patch yet; injector will run again on the next reconcile.
			return nil
		}
		return err
	}
	mutated, changed := mutateState(raw, log)
	if !changed {
		return nil
	}
	// Preserve the original file permissions where possible. os.WriteFile would
	// drop them on overwrite; using OpenFile + Write keeps the existing mode.
	info, err := os.Stat(path)
	mode := os.FileMode(0o600)
	if err == nil {
		mode = info.Mode().Perm()
	}
	if err := os.WriteFile(path, mutated, mode); err != nil {
		return err
	}
	log.Debug("identity injector: patched state", "path", path, "before_bytes", len(raw), "after_bytes", len(mutated))
	return nil
}

// mutateState parses raw as a tfstate v4 document, injects identity blocks
// for every instance whose resource type appears in identityFields, and
// returns the marshalled result. Returns (raw, false) on any failure or
// when nothing needed to change — callers should pass the original bytes
// through in that case.
func mutateState(raw []byte, log logging.Logger) ([]byte, bool) {
	var state map[string]any
	if err := json.Unmarshal(raw, &state); err != nil {
		log.Debug("identity injector: state malformed, passing through", "error", err.Error())
		return raw, false
	}
	resources, ok := state["resources"].([]any)
	if !ok {
		return raw, false
	}

	changed := false
	for _, rAny := range resources {
		r, ok := rAny.(map[string]any)
		if !ok {
			continue
		}
		typ, _ := r["type"].(string)
		fieldMap, known := identityFields[typ]
		if !known {
			continue
		}
		instances, ok := r["instances"].([]any)
		if !ok {
			continue
		}
		for _, instAny := range instances {
			inst, ok := instAny.(map[string]any)
			if !ok {
				continue
			}
			if injectInstance(inst, fieldMap) {
				changed = true
			}
		}
	}

	if !changed {
		return raw, false
	}
	out, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		log.Debug("identity injector: marshal failed, passing through", "error", err.Error())
		return raw, false
	}
	return out, true
}

// injectInstance adds the identity block to a single instance map. Returns
// true if the in-memory map was modified.
//
// fieldMap maps identity-attribute-name → state-attribute-name. The state
// attribute value (whatever it is — including empty string or placeholder)
// becomes the identity value. We deliberately do NOT skip empty values: per
// upstream guidance (SAP/terraform-provider-btp#1532), the framework wants
// an identity block even with placeholder values for the not-yet-found case.
//
// A missing source attribute (not even present as a key) is treated as an
// empty string. Idempotent: if the existing identity block already equals
// what we'd write (and identity_schema_version is correct), returns false.
func injectInstance(inst map[string]any, fieldMap map[string]string) bool {
	attrs, ok := inst["attributes"].(map[string]any)
	if !ok {
		return false
	}
	identity := make(map[string]any, len(fieldMap))
	for idField, srcAttr := range fieldMap {
		v, present := attrs[srcAttr]
		if !present || v == nil {
			v = ""
		}
		identity[idField] = v
	}
	if existing, ok := inst["identity"].(map[string]any); ok && equalIdentity(existing, identity) {
		if v, ok := inst["identity_schema_version"]; ok {
			if n, ok := v.(float64); ok && n == 0 {
				return false
			}
		}
	}
	inst["identity"] = identity
	inst["identity_schema_version"] = float64(0)
	return true
}

func equalIdentity(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		bv, ok := b[k]
		if !ok || av != bv {
			return false
		}
	}
	return true
}
