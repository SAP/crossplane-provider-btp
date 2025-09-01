package servicebindingclient

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/pkg/errors"
	"github.com/spf13/afero"

	"github.com/crossplane/upjet/pkg/resource"
	"github.com/crossplane/upjet/pkg/terraform"
)

// Layout under rootDir:
//   rootDir/
//     .multiworkspace/
//       control.json      -> { "createNew": bool, "target": "key", "gc": { "deleteKeys": ["k1","k2"], "pruneBefore": "RFC3339" } }
//       instances.json    -> { "keys": ["k1","k2"], "current": "k2" }
//     instances/
//       <key>/            -> a standalone terraform workspace (main.tf.json, .terraform, terraform.tfstate, ...)

type mwGC struct {
	DeleteKeys  []string `json:"deleteKeys,omitempty"`
	PruneBefore string   `json:"pruneBefore,omitempty"` // Optional. If set, prune any keys older than this timestamp (RFC3339).
}

type mwControl struct {
	CreateNew bool    `json:"createNew,omitempty"`
	Target    string  `json:"target,omitempty"`
	GC        *mwGC   `json:"gc,omitempty"`
}

type mwInstances struct {
	Keys    []string `json:"keys,omitempty"`
	Current string   `json:"current,omitempty"`
}

// MultiWorkspace implements controller.Workspace and ProviderSharer
// but fans out to per-instance child terraform.Workspace objects
// under rootDir/instances/<key>.
type MultiWorkspace struct {
	logger logging.Logger
	fs     afero.Afero

	rootDir      string
	instancesDir string
	metaDir      string // rootDir/.multiworkspace

	children map[string]*terraform.Workspace

	// Provider sharing info supplied by the scheduler via UseProvider.
	inuse          terraform.InUse
	reattachConfig string

	// Optional: redaction of sensitive logs.
	filterSensitive func(string) string

	// Extra workspace options to apply when instantiating child workspaces
	// (e.g., WithExecutor, WithAferoFs in tests).
	childOpts []terraform.WorkspaceOption

	mu *sync.Mutex
}

type MultiWorkspaceOption func(*MultiWorkspace)

func WithLogger(l logging.Logger) MultiWorkspaceOption {
	return func(m *MultiWorkspace) { m.logger = l }
}

func WithAferoFs(fs afero.Fs) MultiWorkspaceOption {
	return func(m *MultiWorkspace) { m.fs = afero.Afero{Fs: fs} }
}

func WithFilterFn(fn func(string) string) MultiWorkspaceOption {
	return func(m *MultiWorkspace) { m.filterSensitive = fn }
}

func WithChildWorkspaceOptions(opts ...terraform.WorkspaceOption) MultiWorkspaceOption {
	return func(m *MultiWorkspace) { m.childOpts = append(m.childOpts, opts...) }
}

// NewMultiWorkspace constructs the wrapper over a root workspace directory.
// Your Store must pass the correct rootDir (the same one it would give to terraform.NewWorkspace).
func NewMultiWorkspace(rootDir string, opts ...MultiWorkspaceOption) *MultiWorkspace {
	m := &MultiWorkspace{
		logger:       logging.NewNopLogger(),
		fs:           afero.Afero{Fs: afero.NewOsFs()},
		rootDir:      rootDir,
		instancesDir: filepath.Join(rootDir, "instances"),
		metaDir:      filepath.Join(rootDir, ".multiworkspace"),
		children:     map[string]*terraform.Workspace{},
		mu:           &sync.Mutex{},
	}
	for _, o := range opts {
		o(m)
	}
	if m.filterSensitive == nil {
		m.filterSensitive = func(s string) string { return s }
	}
	return m
}

// Implement ProviderSharer: propagate the native provider to children when they run.
func (m *MultiWorkspace) UseProvider(inuse terraform.InUse, attachmentConfig string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inuse = inuse
	m.reattachConfig = attachmentConfig
}

// ApplyAsync: create new instance if requested, then apply it,
// then trigger GC of expired instances in background.
func (m *MultiWorkspace) ApplyAsync(cb terraform.CallbackFn) error {
	child, _, err := m.ensureTargetChildForApply(context.TODO())
	if err != nil {
		return err
	}
	m.ensureProviderOn(child)

	wrapped := func(applyErr error, ctx context.Context) error {
		// Let the upstream callback persist status first.
		var cbErr error
		if cb != nil {
			cbErr = cb(applyErr, ctx)
		}
		// After a successful apply, run GC of expired instances in the background.
		if applyErr == nil {
			go func() {
				_ = m.garbageCollect(context.TODO())
			}()
		}
		return cbErr
	}
	return child.ApplyAsync(wrapped)
}

// Apply: create new instance if requested, then apply it,
// then prune any expired instances (synchronously) if configured.
func (m *MultiWorkspace) Apply(ctx context.Context) (terraform.ApplyResult, error) {
	child, _, err := m.ensureTargetChildForApply(ctx)
	if err != nil {
		return terraform.ApplyResult{}, err
	}
	m.ensureProviderOn(child)

	res, err := child.Apply(ctx)
	if err != nil {
		return res, err
	}
	// After a successful apply, GC expired instances.
	if gcErr := m.garbageCollect(ctx); gcErr != nil {
		// Log and continue; don't turn a successful apply into a failure.
		m.logger.Info("multiworkspace GC failed after Apply", "error", gcErr.Error())
	}
	return res, nil
}

// Refresh: report the current/target child state and also prune expired instances if configured.
func (m *MultiWorkspace) Refresh(ctx context.Context) (terraform.RefreshResult, error) {
	// Always attempt GC so you can prune without a spec-change.
	if gcErr := m.garbageCollect(ctx); gcErr != nil {
		m.logger.Info("multiworkspace GC failed in Refresh", "error", gcErr.Error())
	}
	child, _, err := m.ensureCurrentChild(ctx)
	if err != nil {
		return terraform.RefreshResult{}, err
	}
	if child == nil {
		return terraform.RefreshResult{Exists: false}, nil
	}
	m.ensureProviderOn(child)
	return child.Refresh(ctx)
}

// Import is routed to the current/target child only.
func (m *MultiWorkspace) Import(ctx context.Context, tr resource.Terraformed) (terraform.ImportResult, error) {
	child, _, err := m.ensureCurrentChild(ctx)
	if err != nil {
		return terraform.ImportResult{}, err
	}
	if child == nil {
		return terraform.ImportResult{Exists: false}, nil
	}
	m.ensureProviderOn(child)
	return child.Import(ctx, tr)
}

// Plan runs against the current/target child only.
func (m *MultiWorkspace) Plan(ctx context.Context) (terraform.PlanResult, error) {
	child, _, err := m.ensureCurrentChild(ctx)
	if err != nil {
		return terraform.PlanResult{}, err
	}
	if child == nil {
		return terraform.PlanResult{Exists: false, UpToDate: true}, nil
	}
	m.ensureProviderOn(child)
	return child.Plan(ctx)
}

// DestroyAsync: tear down all instances in parallel (used when the CR is deleted).
func (m *MultiWorkspace) DestroyAsync(cb terraform.CallbackFn) error {
	children, err := m.loadAllChildren(context.TODO())
	if err != nil {
		return err
	}
	if len(children) == 0 {
		return nil
	}
	var wg sync.WaitGroup
	var firstErr error
	var mu sync.Mutex

	for _, ch := range children {
		m.ensureProviderOn(ch)
		wg.Add(1)
		go func(cw *terraform.Workspace) {
			defer wg.Done()
			if err := cw.DestroyAsync(func(e error, ctx context.Context) error {
				mu.Lock()
				if firstErr == nil && e != nil {
					firstErr = e
				}
				mu.Unlock()
				if cb != nil {
					return cb(e, ctx)
				}
				return nil
			}); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
			}
		}(ch)
	}
	wg.Wait()
	return firstErr
}

// Destroy: tear down all instances (used when the CR is deleted).
func (m *MultiWorkspace) Destroy(ctx context.Context) error {
	children, err := m.loadAllChildren(ctx)
	if err != nil {
		return err
	}
	for _, ch := range children {
		m.ensureProviderOn(ch)
		if err := ch.Destroy(ctx); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------- internals --------------------------

func (m *MultiWorkspace) ensureTargetChildForApply(ctx context.Context) (*terraform.Workspace, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.ensureDirs(); err != nil {
		return nil, "", err
	}
	ctrl, err := m.readControl()
	if err != nil {
		return nil, "", err
	}
	inst, err := m.readInstances()
	if err != nil {
		return nil, "", err
	}

	// Create-new flow: allocate a new key and set as current/target.
	if ctrl.CreateNew || len(inst.Keys) == 0 {
		key, err := m.allocateKey(inst)
		if err != nil {
			return nil, "", err
		}
		childDir := filepath.Join(m.instancesDir, key)
		if err := m.fs.MkdirAll(childDir, os.ModePerm); err != nil {
			return nil, "", errors.Wrap(err, "cannot create instance directory")
		}
		child := terraform.NewWorkspace(childDir,
			terraform.WithLogger(m.logger.WithValues("instance", key)),
			terraform.WithFilterFn(m.filterSensitive),
		)
		m.children[key] = child

		inst.Keys = append(inst.Keys, key)
		inst.Current = key
		if err := m.writeInstances(inst); err != nil {
			return nil, "", err
		}
		// Clear the flag and update target.
		ctrl.CreateNew = false
		ctrl.Target = key
		if err := m.writeControl(ctrl); err != nil {
			return nil, "", err
		}
		return child, key, nil
	}

	// Otherwise use target->current->latest
	target := firstNonEmpty(ctrl.Target, inst.Current)
	if target == "" && len(inst.Keys) > 0 {
		target = latestKey(inst.Keys)
		inst.Current = target
		if err := m.writeInstances(inst); err != nil {
			return nil, "", err
		}
	}
	if target == "" {
		return nil, "", errors.New("no target instance and no existing instances")
	}
	child, err := m.getOrCreateChild(target)
	return child, target, err
}

func (m *MultiWorkspace) ensureCurrentChild(ctx context.Context) (*terraform.Workspace, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.ensureDirs(); err != nil {
		return nil, "", err
	}
	ctrl, err := m.readControl()
	if err != nil {
		return nil, "", err
	}
	inst, err := m.readInstances()
	if err != nil {
		return nil, "", err
	}

	target := firstNonEmpty(ctrl.Target, inst.Current)
	if target == "" && len(inst.Keys) > 0 {
		target = latestKey(inst.Keys)
		inst.Current = target
		if err := m.writeInstances(inst); err != nil {
			return nil, "", err
		}
	}
	if target == "" {
		return nil, "", nil
	}
	child, err := m.getOrCreateChild(target)
	return child, target, err
}

func (m *MultiWorkspace) getOrCreateChild(key string) (*terraform.Workspace, error) {
	if key == "" {
		return nil, errors.New("empty instance key")
	}
	if cw, ok := m.children[key]; ok {
		return cw, nil
	}
	childDir := filepath.Join(m.instancesDir, key)
	if _, err := m.fs.Stat(childDir); err != nil {
		return nil, errors.Wrap(err, "instance directory does not exist")
	}
	opts := []terraform.WorkspaceOption{
		terraform.WithLogger(m.logger.WithValues("instance", key)),
		terraform.WithFilterFn(m.filterSensitive),
	}
	opts = append(opts, m.childOpts...)
	cw := terraform.NewWorkspace(childDir, opts...)
	m.children[key] = cw
	return cw, nil
}

func (m *MultiWorkspace) loadAllChildren(ctx context.Context) (map[string]*terraform.Workspace, error) {
	if err := m.ensureDirs(); err != nil {
		return nil, err
	}
	inst, err := m.readInstances()
	if err != nil {
		return nil, err
	}
	children := map[string]*terraform.Workspace{}
	for _, k := range inst.Keys {
		cw, err := m.getOrCreateChild(k)
		if err != nil {
			return nil, err
		}
		children[k] = cw
	}
	return children, nil
}

func (m *MultiWorkspace) ensureProviderOn(cw *terraform.Workspace) {
	if m.inuse == nil || m.reattachConfig == "" {
		return
	}
	// terraform.Workspace implements UseProvider(inuse, cfg).
	cw.UseProvider(m.inuse, m.reattachConfig)
}

func (m *MultiWorkspace) ensureDirs() error {
	if err := m.fs.MkdirAll(m.instancesDir, os.ModePerm); err != nil {
		return errors.Wrap(err, "cannot create instances directory")
	}
	if err := m.fs.MkdirAll(m.metaDir, os.ModePerm); err != nil {
		return errors.Wrap(err, "cannot create multiworkspace meta directory")
	}
	return nil
}

func (m *MultiWorkspace) readControl() (*mwControl, error) {
	path := filepath.Join(m.metaDir, "control.json")
	ctrl := &mwControl{}
	b, err := m.fs.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ctrl, nil
		}
		return nil, errors.Wrap(err, "cannot read control.json")
	}
	if err := json.Unmarshal(b, ctrl); err != nil {
		return nil, errors.Wrap(err, "cannot unmarshal control.json")
	}
	return ctrl, nil
}

func (m *MultiWorkspace) writeControl(ctrl *mwControl) error {
	path := filepath.Join(m.metaDir, "control.json")
	b, err := json.MarshalIndent(ctrl, "", "  ")
	if err != nil {
		return errors.Wrap(err, "cannot marshal control.json")
	}
	if err := m.fs.WriteFile(path, b, 0o644); err != nil {
		return errors.Wrap(err, "cannot write control.json")
	}
	return nil
}

func (m *MultiWorkspace) readInstances() (*mwInstances, error) {
	path := filepath.Join(m.metaDir, "instances.json")
	inst := &mwInstances{}
	b, err := m.fs.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return inst, nil
		}
		return nil, errors.Wrap(err, "cannot read instances.json")
	}
	if err := json.Unmarshal(b, inst); err != nil {
		return nil, errors.Wrap(err, "cannot unmarshal instances.json")
	}
	return inst, nil
}

func (m *MultiWorkspace) writeInstances(inst *mwInstances) error {
	inst.Keys = dedupe(inst.Keys)
	sort.Strings(inst.Keys)

	path := filepath.Join(m.metaDir, "instances.json")
	b, err := json.MarshalIndent(inst, "", "  ")
	if err != nil {
		return errors.Wrap(err, "cannot marshal instances.json")
	}
	if err := m.fs.WriteFile(path, b, 0o644); err != nil {
		return errors.Wrap(err, "cannot write instances.json")
	}
	return nil
}

func (m *MultiWorkspace) allocateKey(inst *mwInstances) (string, error) {
	// Default to timestamp-based keys. You can replace this by writing a key
	// in control.Target before calling Apply.
	base := time.Now().UTC().Format(time.RFC3339Nano)
	base = strings.ReplaceAll(base, ":", "-")
	base = strings.ReplaceAll(base, ".", "-")
	key := base
	i := 1
	for {
		_, err := m.fs.Stat(filepath.Join(m.instancesDir, key))
		if os.IsNotExist(err) {
			return key, nil
		}
		if err != nil {
			return "", errors.Wrap(err, "cannot stat instance directory")
		}
		key = fmt.Sprintf("%s-%d", base, i)
		i++
	}
}

func (m *MultiWorkspace) garbageCollect(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.ensureDirs(); err != nil {
		return err
	}
	ctrl, err := m.readControl()
	if err != nil {
		return err
	}
	if ctrl.GC == nil || (len(ctrl.GC.DeleteKeys) == 0 && ctrl.GC.PruneBefore == "") {
		return nil
	}
	inst, err := m.readInstances()
	if err != nil {
		return err
	}

	// Build deletion set.
	toDelete := map[string]struct{}{}
	for _, k := range ctrl.GC.DeleteKeys {
		toDelete[k] = struct{}{}
	}
	// Optional prune-before cutoff
	if ctrl.GC.PruneBefore != "" {
		if cutoff, err := time.Parse(time.RFC3339, ctrl.GC.PruneBefore); err == nil {
			for _, k := range inst.Keys {
				// If keys are timestamp-based (default here), lexical parse can work.
				// If you use your own keys, decide deletion in your Store and pass via DeleteKeys.
				kt := strings.ReplaceAll(strings.ReplaceAll(k, "-", ":"), ":", ":") // no-op fallback
				_ = kt
				// Best effort: compare by directory mtime if key not parseable.
				fi, statErr := m.fs.Stat(filepath.Join(m.instancesDir, k))
				if statErr == nil {
					if fi.ModTime().Before(cutoff) {
						toDelete[k] = struct{}{}
					}
				}
			}
		}
	}

	// Destroy and remove directories.
	updatedKeys := make([]string, 0, len(inst.Keys))
	for _, k := range inst.Keys {
		if _, doomed := toDelete[k]; !doomed {
			updatedKeys = append(updatedKeys, k)
			continue
		}
		// Perform destroy on that child if it exists or can be constructed.
		cw, err := m.getOrCreateChild(k)
		if err != nil {
			// If there's no workspace, best-effort: remove dir if it exists.
			_ = m.fs.RemoveAll(filepath.Join(m.instancesDir, k))
			continue
		}
		// Share provider runner
		m.ensureProviderOn(cw)
		if err := cw.Destroy(ctx); err != nil {
			m.logger.Info("failed to destroy expired instance", "instance", k, "error", err.Error())
			// Keep it in the list; skip removal
			updatedKeys = append(updatedKeys, k)
			continue
		}
		// Remove directory on success.
		if rmErr := m.fs.RemoveAll(filepath.Join(m.instancesDir, k)); rmErr != nil {
			m.logger.Info("failed to remove instance directory after destroy", "instance", k, "error", rmErr.Error())
		}
		// Drop from cache.
		delete(m.children, k)
		// If we deleted the current selection, clear it.
		if inst.Current == k {
			inst.Current = ""
		}
	}

	inst.Keys = updatedKeys
	if inst.Current == "" && len(inst.Keys) > 0 {
		inst.Current = latestKey(inst.Keys)
	}
	if err := m.writeInstances(inst); err != nil {
		return err
	}

	// Clear GC section so we don't re-delete.
	ctrl.GC = nil
	if err := m.writeControl(ctrl); err != nil {
		return err
	}
	return nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func latestKey(keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	sorted := append([]string{}, keys...)
	sort.Strings(sorted)
	return sorted[len(sorted)-1]
}

func dedupe(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, k := range in {
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	return out
}
