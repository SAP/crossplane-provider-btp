package directory

import (
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	v2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/SAP/xp-clifford/yaml"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/hierarchy"
)

const (
	globalAccountGUID = "ga-00000000-0000-0000-0000-000000000000"
	parentGUID        = "dir-parent-1111-1111-1111-111111111111"
	parentName        = "parent-directory"
	childGUID         = "dir-child-2222-2222-2222-222222222222"
	childName         = "child-directory"
)

var (
	parentResourceName = hierarchy.K8sResourceName(parentGUID, parentName)
	childResourceName  = hierarchy.K8sResourceName(childGUID, childName)
)

// newDirectory builds a *directory test fixture from a btpcli.Child.
func newDirectory(c *btpcli.Child) *directory {
	return &directory{
		Child:               c,
		ResourceWithComment: yaml.NewResourceWithComment(nil),
	}
}

// newDirectoryWithAdmins builds a *directory test fixture with the given admins.
func newDirectoryWithAdmins(c *btpcli.Child, admins []string) *directory {
	d := newDirectory(c)
	d.Admins = admins
	return d
}

// stubEventHandler is a minimal export.EventHandler that records warnings so
// tests can assert that resolution failures are surfaced.
type stubEventHandler struct {
	warnings []error
}

func (s *stubEventHandler) Warn(err error)           { s.warnings = append(s.warnings, err) }
func (s *stubEventHandler) Resource(resource.Object) {}
func (s *stubEventHandler) Stop()                    {}

// seedCache replaces the package-level directoryCache with the given directories
// and restores the previous value when the test finishes. Because the cache is a
// package global, tests that use it must not run in parallel.
func seedCache(t *testing.T, dirs ...*directory) {
	t.Helper()
	prev := directoryCache
	t.Cleanup(func() { directoryCache = prev })

	cache := resources.NewResourceCache[*directory]()
	cache.Store(dirs...)
	directoryCache = cache
}

func TestConvertDirectoryResource(t *testing.T) {
	r := require.New(t)

	tests := []struct {
		name              string
		dir               *directory
		resolveReferences bool
		// seed holds the directories that are considered "selected/exported",
		// i.e. present in the cache for reference resolution.
		seed []*directory
		// wantWarn is true when a Warn event is expected (reference resolution failures).
		// The directoryAdmins comment alone does not emit a Warn event.
		wantWarn bool
		want     *yaml.ResourceWithComment
	}{
		{
			name: "all fields present, no resolution",
			dir: newDirectory(&btpcli.Child{
				GUID:              childGUID,
				DisplayName:       childName,
				Description:       "a child directory",
				Subdomain:         "child-subdomain",
				ParentGUID:        parentGUID,
				GlobalAccountGUID: globalAccountGUID,
				DirectoryType:     "DIRECTORY",
				DirectoryFeatures: []string{"DEFAULT"},
				Labels:            map[string][]string{"env": {"test"}},
			}),
			resolveReferences: false,
			want: func() *yaml.ResourceWithComment {
				desc := "a child directory"
				subdomain := "child-subdomain"
				dn := childName
				rwc := yaml.NewResourceWithComment(&v1alpha1.Directory{
					TypeMeta: metav1.TypeMeta{
						Kind:       v1alpha1.DirectoryKind,
						APIVersion: v1alpha1.CRDGroupVersion.String(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: childResourceName,
						Annotations: map[string]string{
							"crossplane.io/external-name": childGUID,
						},
					},
					Spec: v1alpha1.DirectorySpec{
						ClusterManagedResourceSpec: v2.ClusterManagedResourceSpec{
							ManagementPolicies: []v2.ManagementAction{v2.ManagementActionObserve},
						},
						ForProvider: v1alpha1.DirectoryParameters{
							DisplayName:       &dn,
							DirectoryFeatures: []string{"DEFAULT"},
							DirectoryAdmins:   []string{},
							Description:       &desc,
							Subdomain:         &subdomain,
							// Parent is another directory -> raw GUID kept when not resolving.
							DirectoryGuid: parentGUID,
							Labels:        map[string][]string{"env": {"test"}},
						},
					},
				})
				// DEFAULT directory has no AUTHORIZATIONS feature -> no admins -> commented out.
				rwc.AddComment(warnMissingAdmins)
				return rwc
			}(),
		},
		{
			name: "parent is global account, no reference set",
			dir: newDirectory(&btpcli.Child{
				GUID:              parentGUID,
				DisplayName:       parentName,
				ParentGUID:        globalAccountGUID,
				GlobalAccountGUID: globalAccountGUID,
				DirectoryType:     "DIRECTORY",
				DirectoryFeatures: []string{"DEFAULT"},
			}),
			resolveReferences: true,
			seed: []*directory{
				newDirectory(&btpcli.Child{GUID: parentGUID, DisplayName: parentName, DirectoryType: "DIRECTORY"}),
			},
			want: func() *yaml.ResourceWithComment {
				dn := parentName
				rwc := yaml.NewResourceWithComment(&v1alpha1.Directory{
					TypeMeta: metav1.TypeMeta{
						Kind:       v1alpha1.DirectoryKind,
						APIVersion: v1alpha1.CRDGroupVersion.String(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: parentResourceName,
						Annotations: map[string]string{
							"crossplane.io/external-name": parentGUID,
						},
					},
					Spec: v1alpha1.DirectorySpec{
						ClusterManagedResourceSpec: v2.ClusterManagedResourceSpec{
							ManagementPolicies: []v2.ManagementAction{v2.ManagementActionObserve},
						},
						ForProvider: v1alpha1.DirectoryParameters{
							DisplayName:       &dn,
							DirectoryFeatures: []string{"DEFAULT"},
							DirectoryAdmins:   []string{},
							// No DirectoryGuid and no DirectoryRef: parent is the global account.
						},
					},
				})
				rwc.AddComment(warnMissingAdmins)
				return rwc
			}(),
		},
		{
			name: "parent directory resolved to reference",
			dir: newDirectory(&btpcli.Child{
				GUID:              childGUID,
				DisplayName:       childName,
				ParentGUID:        parentGUID,
				GlobalAccountGUID: globalAccountGUID,
				DirectoryType:     "DIRECTORY",
				DirectoryFeatures: []string{"DEFAULT"},
			}),
			resolveReferences: true,
			seed: []*directory{
				// Parent is in the exported set, so it resolves.
				newDirectory(&btpcli.Child{GUID: parentGUID, DisplayName: parentName, DirectoryType: "DIRECTORY"}),
			},
			want: func() *yaml.ResourceWithComment {
				dn := childName
				rwc := yaml.NewResourceWithComment(&v1alpha1.Directory{
					TypeMeta: metav1.TypeMeta{
						Kind:       v1alpha1.DirectoryKind,
						APIVersion: v1alpha1.CRDGroupVersion.String(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: childResourceName,
						Annotations: map[string]string{
							"crossplane.io/external-name": childGUID,
						},
					},
					Spec: v1alpha1.DirectorySpec{
						ClusterManagedResourceSpec: v2.ClusterManagedResourceSpec{
							ManagementPolicies: []v2.ManagementAction{v2.ManagementActionObserve},
						},
						ForProvider: v1alpha1.DirectoryParameters{
							DisplayName:       &dn,
							DirectoryFeatures: []string{"DEFAULT"},
							DirectoryAdmins:   []string{},
							// DirectoryGuid cleared, ref points at the parent's k8s name.
							DirectoryRef: &v2.Reference{Name: parentResourceName},
						},
					},
				})
				rwc.AddComment(warnMissingAdmins)
				return rwc
			}(),
		},
		{
			name: "parent not in export set, warning and GUID kept",
			dir: newDirectory(&btpcli.Child{
				GUID:              childGUID,
				DisplayName:       childName,
				ParentGUID:        parentGUID,
				GlobalAccountGUID: globalAccountGUID,
				DirectoryType:     "DIRECTORY",
				DirectoryFeatures: []string{"DEFAULT"},
			}),
			resolveReferences: true,
			// Only the child itself is in the cache; the parent was not selected.
			seed: []*directory{
				newDirectory(&btpcli.Child{GUID: childGUID, DisplayName: childName, DirectoryType: "DIRECTORY"}),
			},
			wantWarn: true,
			want: func() *yaml.ResourceWithComment {
				dn := childName
				rwc := yaml.NewResourceWithComment(&v1alpha1.Directory{
					TypeMeta: metav1.TypeMeta{
						Kind:       v1alpha1.DirectoryKind,
						APIVersion: v1alpha1.CRDGroupVersion.String(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: childResourceName,
						Annotations: map[string]string{
							"crossplane.io/external-name": childGUID,
						},
					},
					Spec: v1alpha1.DirectorySpec{
						ClusterManagedResourceSpec: v2.ClusterManagedResourceSpec{
							ManagementPolicies: []v2.ManagementAction{v2.ManagementActionObserve},
						},
						ForProvider: v1alpha1.DirectoryParameters{
							DisplayName:       &dn,
							DirectoryFeatures: []string{"DEFAULT"},
							DirectoryAdmins:   []string{},
							// Unresolved: GUID retained as fallback, no ref.
							DirectoryGuid: parentGUID,
						},
					},
				})
				rwc.AddComment(warnMissingAdmins)
				rwc.AddComment(resources.WarnCannotResolveDirectory + ": " + parentGUID)
				return rwc
			}(),
		},
		{
			name: "missing display name and guid add warnings",
			dir: newDirectory(&btpcli.Child{
				DirectoryType:     "DIRECTORY",
				DirectoryFeatures: []string{"DEFAULT"},
			}),
			resolveReferences: false,
			want: func() *yaml.ResourceWithComment {
				dn := ""
				rwc := yaml.NewResourceWithComment(&v1alpha1.Directory{
					TypeMeta: metav1.TypeMeta{
						Kind:       v1alpha1.DirectoryKind,
						APIVersion: v1alpha1.CRDGroupVersion.String(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: resources.UndefinedName,
						Annotations: map[string]string{
							"crossplane.io/external-name": "",
						},
					},
					Spec: v1alpha1.DirectorySpec{
						ClusterManagedResourceSpec: v2.ClusterManagedResourceSpec{
							ManagementPolicies: []v2.ManagementAction{v2.ManagementActionObserve},
						},
						ForProvider: v1alpha1.DirectoryParameters{
							DisplayName:       &dn,
							DirectoryFeatures: []string{"DEFAULT"},
							DirectoryAdmins:   []string{},
						},
					},
				})
				rwc.AddComment(warnMissingDisplayName)
				rwc.AddComment(warnMissingGuid)
				rwc.AddComment(warnMissingAdmins)
				return rwc
			}(),
		},
		{
			name: "authorizations feature with two admins is emitted cleanly",
			dir: newDirectoryWithAdmins(&btpcli.Child{
				GUID:              childGUID,
				DisplayName:       childName,
				ParentGUID:        globalAccountGUID,
				GlobalAccountGUID: globalAccountGUID,
				DirectoryType:     "DIRECTORY",
				DirectoryFeatures: []string{"DEFAULT", "ENTITLEMENTS", "AUTHORIZATIONS"},
			}, []string{"admin1@example.com", "admin2@example.com"}),
			resolveReferences: false,
			want: func() *yaml.ResourceWithComment {
				dn := childName
				// No comment: the resource satisfies the CRD and is emitted as a manifest.
				return yaml.NewResourceWithComment(&v1alpha1.Directory{
					TypeMeta: metav1.TypeMeta{
						Kind:       v1alpha1.DirectoryKind,
						APIVersion: v1alpha1.CRDGroupVersion.String(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: childResourceName,
						Annotations: map[string]string{
							"crossplane.io/external-name": childGUID,
						},
					},
					Spec: v1alpha1.DirectorySpec{
						ClusterManagedResourceSpec: v2.ClusterManagedResourceSpec{
							ManagementPolicies: []v2.ManagementAction{v2.ManagementActionObserve},
						},
						ForProvider: v1alpha1.DirectoryParameters{
							DisplayName:       &dn,
							DirectoryFeatures: []string{"DEFAULT", "ENTITLEMENTS", "AUTHORIZATIONS"},
							DirectoryAdmins:   []string{"admin1@example.com", "admin2@example.com"},
							// Parent is the global account: no DirectoryGuid, no ref.
						},
					},
				})
			}(),
		},
		{
			name: "authorizations feature with a single admin is commented out",
			dir: newDirectoryWithAdmins(&btpcli.Child{
				GUID:              childGUID,
				DisplayName:       childName,
				ParentGUID:        globalAccountGUID,
				GlobalAccountGUID: globalAccountGUID,
				DirectoryType:     "DIRECTORY",
				DirectoryFeatures: []string{"DEFAULT", "ENTITLEMENTS", "AUTHORIZATIONS"},
			}, []string{"admin1@example.com"}),
			resolveReferences: false,
			want: func() *yaml.ResourceWithComment {
				dn := childName
				rwc := yaml.NewResourceWithComment(&v1alpha1.Directory{
					TypeMeta: metav1.TypeMeta{
						Kind:       v1alpha1.DirectoryKind,
						APIVersion: v1alpha1.CRDGroupVersion.String(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: childResourceName,
						Annotations: map[string]string{
							"crossplane.io/external-name": childGUID,
						},
					},
					Spec: v1alpha1.DirectorySpec{
						ClusterManagedResourceSpec: v2.ClusterManagedResourceSpec{
							ManagementPolicies: []v2.ManagementAction{v2.ManagementActionObserve},
						},
						ForProvider: v1alpha1.DirectoryParameters{
							DisplayName:       &dn,
							DirectoryFeatures: []string{"DEFAULT", "ENTITLEMENTS", "AUTHORIZATIONS"},
							DirectoryAdmins:   []string{"admin1@example.com"},
						},
					},
				})
				// Fewer than two admins -> cannot satisfy minItems: 2 -> commented out.
				rwc.AddComment(warnMissingAdmins)
				return rwc
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.seed != nil {
				seedCache(t, tt.seed...)
			}

			eh := &stubEventHandler{}

			// btpClient is never dereferenced: GetK8sResourceNameByID short-circuits
			// on the already-populated package cache, so nil is safe here.
			result := convertDirectoryResource(t.Context(), nil, tt.dir, eh, tt.resolveReferences)
			r.NotNil(result)

			// Verify comments.
			gotComment, gotHasComment := result.Comment()
			wantComment, wantHasComment := tt.want.Comment()
			r.Equal(wantHasComment, gotHasComment, "comment presence mismatch")
			if wantHasComment {
				r.Equal(wantComment, gotComment, "comment mismatch")
			}

			// A resolution-failure comment must be accompanied by a Warn event.
			if tt.wantWarn {
				r.NotEmpty(eh.warnings, "expected a warning to be emitted on failed resolution")
			} else {
				r.Empty(eh.warnings, "did not expect a warning")
			}

			// Verify resource type and contents.
			gotDir, gotOk := result.Resource().(*v1alpha1.Directory)
			wantDir, wantOk := tt.want.Resource().(*v1alpha1.Directory)
			r.True(gotOk && wantOk, "expected resource type to be *v1alpha1.Directory")
			r.NotNil(gotDir.Spec.ForProvider.DirectoryAdmins, "DirectoryAdmins must not be nil")
			r.Equal(wantDir, gotDir)
		})
	}
}

func TestResolveReference(t *testing.T) {
	r := require.New(t)

	t.Run("empty guid is a no-op (parent is global account)", func(t *testing.T) {
		spec := &v1alpha1.DirectoryParameters{}
		err := resolveReference(t.Context(), nil, spec)
		r.NoError(err)
		r.Nil(spec.DirectoryRef)
		r.Empty(spec.DirectoryGuid)
	})

	t.Run("resolvable parent sets ref and clears guid", func(t *testing.T) {
		seedCache(t, newDirectory(&btpcli.Child{
			GUID: parentGUID, DisplayName: parentName, DirectoryType: "DIRECTORY",
		}))

		spec := &v1alpha1.DirectoryParameters{DirectoryGuid: parentGUID}
		err := resolveReference(t.Context(), nil, spec)
		r.NoError(err)
		r.Equal(&v2.Reference{Name: parentResourceName}, spec.DirectoryRef)
		r.Empty(spec.DirectoryGuid)
	})

	t.Run("unresolvable parent returns error and leaves guid", func(t *testing.T) {
		seedCache(t, newDirectory(&btpcli.Child{
			GUID: childGUID, DisplayName: childName, DirectoryType: "DIRECTORY",
		}))

		spec := &v1alpha1.DirectoryParameters{DirectoryGuid: parentGUID}
		err := resolveReference(t.Context(), nil, spec)
		r.Error(err)
		r.Nil(spec.DirectoryRef)
		r.Equal(parentGUID, spec.DirectoryGuid)
	})
}
