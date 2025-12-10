package directory

import (
	"context"
	"fmt"
	"testing"

	v1_crossplane "github.com/crossplane/crossplane-runtime/apis/common/v1"
	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/internal"
	"github.com/sap/crossplane-provider-btp/internal/clients/directory"
	"github.com/sap/crossplane-provider-btp/internal/testutils"
	tracking_test "github.com/sap/crossplane-provider-btp/internal/tracking/test"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestConnect(t *testing.T) {
	type args struct {
		cr              resource.Managed
		kubeObjects     []client.Object
		serviceFnErr    error
		serviceFnReturn *btp.Client
	}
	type newServiceArgs struct {
		cisCreds []byte
		saCreds  []byte
	}
	type want struct {
		err            error
		newServiceArgs newServiceArgs
	}
	tests := map[string]struct {
		args args
		want want
	}{
		"NilResource": {
			args: args{
				cr:          nil,
				kubeObjects: []client.Object{},
			},
			want: want{
				err: errors.New(errNotDirectory),
			},
		},
		"NoProviderConfig": {
			args: args{
				cr: &v1alpha1.Directory{
					Spec: v1alpha1.DirectorySpec{
						ResourceSpec: xpv1.ResourceSpec{
							ProviderConfigReference: &v1_crossplane.Reference{
								Name: "pc-reference",
							}},
					},
				},
				kubeObjects: []client.Object{},
			},
			want: want{
				err: errors.New("cannot get ProviderConfig"),
			},
		},
		"NoCISCredentials": {
			args: args{
				cr: &v1alpha1.Directory{
					Spec: v1alpha1.DirectorySpec{
						ResourceSpec: xpv1.ResourceSpec{
							ProviderConfigReference: &v1_crossplane.Reference{
								Name: "pc-reference",
							}},
					},
				},
				kubeObjects: []client.Object{
					testutils.NewProviderConfig("pc-reference", "cis-provider-secret", "sa-provider-secret"),
				},
			},
			want: want{
				err: errors.New("cannot get CIS credentials"),
			},
		},
		"NoSACredentials": {
			args: args{
				cr: &v1alpha1.Directory{
					Spec: v1alpha1.DirectorySpec{
						ResourceSpec: xpv1.ResourceSpec{
							ProviderConfigReference: &v1_crossplane.Reference{
								Name: "pc-reference",
							}},
					},
				},
				kubeObjects: []client.Object{
					testutils.NewProviderConfig("pc-reference", "cis-provider-secret", "sa-provider-secret"),
					testutils.NewSecret("cis-provider-secret", map[string][]byte{"data": []byte("someCISCreds")}),
				},
			},
			want: want{
				err: errors.New("cannot get Service Account credentials"),
			},
		},
		"EmptyCISSecret": {
			args: args{
				cr: &v1alpha1.Directory{
					Spec: v1alpha1.DirectorySpec{
						ResourceSpec: xpv1.ResourceSpec{
							ProviderConfigReference: &v1_crossplane.Reference{
								Name: "pc-reference",
							}},
					},
				},
				kubeObjects: []client.Object{
					testutils.NewProviderConfig("pc-reference", "cis-provider-secret", "sa-provider-secret"),
					testutils.NewSecret("cis-provider-secret", nil),
					testutils.NewSecret("sa-provider-secret", nil),
				},
			},
			want: want{
				err: errors.New("CIS Secret is empty or nil, please check config & secrets referenced in provider config"),
			},
		},
		"NewServiceFnError": {
			args: args{
				cr: &v1alpha1.Directory{
					Spec: v1alpha1.DirectorySpec{
						ResourceSpec: xpv1.ResourceSpec{
							ProviderConfigReference: &v1_crossplane.Reference{
								Name: "pc-reference",
							}},
					},
				},
				kubeObjects: []client.Object{
					testutils.NewProviderConfig("pc-reference", "cis-provider-secret", "sa-provider-secret"),
					testutils.NewSecret("cis-provider-secret", map[string][]byte{"data": []byte("someCISCreds")}),
					testutils.NewSecret("sa-provider-secret", map[string][]byte{"credentials": []byte("someSACreds")}),
				},
				serviceFnReturn: &btp.Client{},
				serviceFnErr:    errors.New("serviceFnError"),
			},
			want: want{
				newServiceArgs: newServiceArgs{
					cisCreds: []byte("someCISCreds"),
					saCreds:  []byte("someSACreds"),
				},
				err: errors.New("serviceFnError"),
			},
		},
		"Successful": {
			args: args{
				cr: &v1alpha1.Directory{
					Spec: v1alpha1.DirectorySpec{
						ResourceSpec: xpv1.ResourceSpec{
							ProviderConfigReference: &v1_crossplane.Reference{
								Name: "pc-reference",
							}},
					},
				},
				kubeObjects: []client.Object{
					testutils.NewProviderConfig("pc-reference", "cis-provider-secret", "sa-provider-secret"),
					testutils.NewSecret("cis-provider-secret", map[string][]byte{"data": []byte("someCISCreds")}),
					testutils.NewSecret("sa-provider-secret", map[string][]byte{"credentials": []byte("someSACreds")}),
				},
				serviceFnReturn: &btp.Client{},
			},
			want: want{
				newServiceArgs: newServiceArgs{
					cisCreds: []byte("someCISCreds"),
					saCreds:  []byte("someSACreds"),
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			kube := testutils.NewFakeKubeClientBuilder().
				AddResources(tc.args.kubeObjects...).
				Build()
			c := connector{
				kube:            &kube,
				usage:           tracking_test.NoOpReferenceResolverTracker{},
				resourcetracker: tracking_test.NoOpReferenceResolverTracker{},
				newServiceFn: func(cisSecretData []byte, serviceAccountSecretData []byte) (*btp.Client, error) {
					if tc.want.newServiceArgs.cisCreds != nil && string(tc.want.newServiceArgs.cisCreds) != string(cisSecretData) {
						t.Errorf("Passed CIS Creds to newServiceFN do not match; Passed: %v, Expected: %v", cisSecretData, tc.want.newServiceArgs.cisCreds)
					}
					if tc.want.newServiceArgs.saCreds != nil && string(tc.want.newServiceArgs.saCreds) != string(serviceAccountSecretData) {
						t.Errorf("Passed SA Creds to newServiceFN do not match; Passed: %v, Expected: %v", serviceAccountSecretData, tc.want.newServiceArgs.saCreds)
					}
					return tc.args.serviceFnReturn, tc.args.serviceFnErr
				},
			}

			connect, err := c.Connect(context.Background(), tc.args.cr)

			if contained := testutils.ContainsError(err, tc.want.err); !contained {
				t.Errorf("\ne.Connect(...): error \"%v\" not part of \"%v\"", err, tc.want.err)
			}
			if tc.want.err == nil {
				if connect == nil {
					t.Errorf("Expected connector to be != nil")
				}
			}
		})
	}
}

func TestObserve(t *testing.T) {
	type args struct {
		cr         resource.Managed
		mockClient MockClient
	}
	type want struct {
		err error
		o   managed.ExternalObservation
		cr  resource.Managed
	}
	tests := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"NilResource": {
			reason: "Expect error if used with another resource type",
			args: args{
				cr: nil,
			},
			want: want{
				err: errors.New(errNotDirectory),
			},
		},
		"APIErrorOnRead": {
			reason: "When needsCreation can't be determined we can't proceed",
			args: args{
				cr:         testutils.NewDirectory("dir-unittests", testutils.WithStatus(v1alpha1.DirectoryObservation{Guid: internal.Ptr("123")})),
				mockClient: MockClient{needsCreation: true, needsCreationErr: errors.New("internalServerError")},
			},
			want: want{
				o:   managed.ExternalObservation{},
				err: errors.New("internalServerError"),
				cr:  testutils.NewDirectory("dir-unittests", testutils.WithStatus(v1alpha1.DirectoryObservation{Guid: internal.Ptr("123")})),
			},
		},
		"RequiresCreation": {
			reason: "If client requires it we need to trigger a creation",
			args: args{
				cr:         testutils.NewDirectory("dir-unittests", testutils.WithStatus(v1alpha1.DirectoryObservation{Guid: internal.Ptr("123")})),
				mockClient: MockClient{needsCreation: true, needsCreationErr: nil},
			},
			want: want{
				o:  managed.ExternalObservation{ResourceExists: false},
				cr: testutils.NewDirectory("dir-unittests", testutils.WithStatus(v1alpha1.DirectoryObservation{Guid: internal.Ptr("123")})),
			},
		},
		"SyncError": {
			reason: "If client requires it we need to trigger an update",
			args: args{
				cr:         testutils.NewDirectory("dir-unittests", testutils.WithStatus(v1alpha1.DirectoryObservation{Guid: internal.Ptr("123")})),
				mockClient: MockClient{needsCreation: false, needsUpdate: true, syncErr: errors.New("syncError")},
			},
			want: want{
				o:   managed.ExternalObservation{},
				cr:  testutils.NewDirectory("dir-unittests", testutils.WithStatus(v1alpha1.DirectoryObservation{Guid: internal.Ptr("123")})),
				err: errors.New("syncError"),
			},
		},
		"Unavailable": {
			reason: "If state does not indicate OK its unavailable",
			args: args{
				cr: testutils.NewDirectory("dir-unittests",
					testutils.WithStatus(v1alpha1.DirectoryObservation{
						Guid:        internal.Ptr("123"),
						EntityState: internal.Ptr("CREATION"),
					}),
				),
				mockClient: MockClient{needsCreation: false, needsUpdate: false, syncErr: nil},
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  true,
					ConnectionDetails: managed.ConnectionDetails{},
				},
				cr: testutils.NewDirectory("dir-unittests",
					testutils.WithStatus(v1alpha1.DirectoryObservation{
						Guid:        internal.Ptr("123"),
						EntityState: internal.Ptr("CREATION"),
					}),
					testutils.WithConditions(xpv1.Unavailable())),
			},
		},
		"RequiresUpdate": {
			reason: "If client requires it we need to trigger an update",
			args: args{
				cr:         testutils.NewDirectory("dir-unittests", testutils.WithStatus(v1alpha1.DirectoryObservation{Guid: internal.Ptr("123")})),
				mockClient: MockClient{needsCreation: false, needsUpdate: true, syncErr: nil, available: true},
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  false,
					ConnectionDetails: managed.ConnectionDetails{},
				},
				cr: testutils.NewDirectory("dir-unittests", testutils.WithConditions(xpv1.Available()), testutils.WithStatus(v1alpha1.DirectoryObservation{Guid: internal.Ptr("123")})),
			},
		},
		"UpToDate": {
			reason: "If client determines everything is up to date we don't need to do anything",
			args: args{
				cr: testutils.NewDirectory("dir-unittests",
					testutils.WithStatus(v1alpha1.DirectoryObservation{Guid: internal.Ptr("123")}),
				),
				mockClient: MockClient{needsCreation: false, needsUpdate: false, syncErr: nil, available: true},
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  true,
					ConnectionDetails: managed.ConnectionDetails{},
				},
				cr: testutils.NewDirectory("dir-unittests",
					testutils.WithStatus(v1alpha1.DirectoryObservation{Guid: internal.Ptr("123")}),
					testutils.WithConditions(xpv1.Available())),
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := external{
				newDirHandlerFn: func(client2 *btp.Client, cr *v1alpha1.Directory) directory.DirectoryClientI {
					return tc.args.mockClient
				},
				tracker: nil,
			}
			got, err := ctrl.Observe(context.Background(), tc.args.cr)

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.Observe(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\n%s\ne.Observe(...): -want, +got:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.cr, tc.args.cr); diff != "" {
				t.Errorf("\n%s\ne.Observe(...): -want cr, +got cr:\n%s\n", tc.reason, diff)
			}

		})
	}
}

func TestCreate(t *testing.T) {
	type args struct {
		cr            resource.Managed
		mockClient    MockClient
		kubeUpdateErr error
	}
	type want struct {
		err error
		o   managed.ExternalCreation
		cr  resource.Managed
	}
	tests := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"NilResource": {
			reason: "Expect error if used with another resource type",
			args: args{
				cr: nil,
			},
			want: want{
				err: errors.New(errNotDirectory),
			},
		},
		"Failure": {
			reason: "We expect to return an error if Create fails",
			args: args{
				cr:         testutils.NewDirectory("dir-unittests", testutils.WithStatus(v1alpha1.DirectoryObservation{Guid: internal.Ptr("123")})),
				mockClient: MockClient{createErr: errors.New("CreateError")},
			},
			want: want{
				o:   managed.ExternalCreation{},
				cr:  testutils.NewDirectory("dir-unittests", testutils.WithConditions(xpv1.Creating()), testutils.WithStatus(v1alpha1.DirectoryObservation{Guid: internal.Ptr("123")})),
				err: errors.New("CreateError"),
			},
		},
		"Success": {
			reason: "We expect to finish gracefully if no error happened during create",
			args: args{
				cr: testutils.NewDirectory("dir-unittests", testutils.WithStatus(v1alpha1.DirectoryObservation{Guid: internal.Ptr("123")})),
				mockClient: MockClient{
					createErr: nil,
					createResult: *testutils.NewDirectory("dir-unittests",
						testutils.WithStatus(v1alpha1.DirectoryObservation{Guid: internal.Ptr("123")})),
				}},
			want: want{
				o:   managed.ExternalCreation{ConnectionDetails: managed.ConnectionDetails{}},
				cr:  testutils.NewDirectory("dir-unittests", testutils.WithConditions(xpv1.Creating()), testutils.WithStatus(v1alpha1.DirectoryObservation{Guid: internal.Ptr("123")})),
				err: nil,
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mockKube := testutils.NewFakeKubeClientBuilder().Build()
			mockKube.MockStatusUpdate = func(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
				return tc.args.kubeUpdateErr
			}
			ctrl := external{
				newDirHandlerFn: func(client2 *btp.Client, cr *v1alpha1.Directory) directory.DirectoryClientI {
					return tc.args.mockClient
				},
				tracker: nil,
				kube:    &mockKube,
			}
			got, err := ctrl.Create(context.Background(), tc.args.cr)

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.Create(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\n%s\ne.Create(...): -want, +got:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.cr, tc.args.cr); diff != "" {
				t.Errorf("\n%s\ne.Create(...): -want cr, +got cr:\n%s\n", tc.reason, diff)
			}

		})
	}
}

func TestDelete(t *testing.T) {
	type args struct {
		cr         resource.Managed
		mockClient MockClient
	}
	type want struct {
		err error
		cr  resource.Managed
	}
	tests := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"NilResource": {
			reason: "Expect error if used with another resource type",
			args: args{
				cr: nil,
			},
			want: want{
				err: errors.New(errNotDirectory),
			},
		},
		"Failure": {
			reason: "We expect to return an error if Delete fails",
			args: args{
				cr:         testutils.NewDirectory("dir-unittests", testutils.WithStatus(v1alpha1.DirectoryObservation{Guid: internal.Ptr("123")})),
				mockClient: MockClient{deleteErr: errors.New("DeleteError")},
			},
			want: want{
				cr:  testutils.NewDirectory("dir-unittests", testutils.WithConditions(xpv1.Deleting()), testutils.WithStatus(v1alpha1.DirectoryObservation{Guid: internal.Ptr("123")})),
				err: errors.New("DeleteError"),
			},
		},
		"Success": {
			reason: "We expect to finish gracefully if no error happened during create",
			args: args{
				cr: testutils.NewDirectory("dir-unittests", testutils.WithStatus(v1alpha1.DirectoryObservation{Guid: internal.Ptr("123")})),
				mockClient: MockClient{
					deleteErr: nil,
				}},
			want: want{
				cr:  testutils.NewDirectory("dir-unittests", testutils.WithConditions(xpv1.Deleting()), testutils.WithStatus(v1alpha1.DirectoryObservation{Guid: internal.Ptr("123")})),
				err: nil,
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mockKube := testutils.NewFakeKubeClientBuilder().Build()
			ctrl := external{
				newDirHandlerFn: func(client2 *btp.Client, cr *v1alpha1.Directory) directory.DirectoryClientI {
					return tc.args.mockClient
				},
				tracker: nil,
				kube:    &mockKube,
			}
			_, err := ctrl.Delete(context.Background(), tc.args.cr)

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.Delete(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.cr, tc.args.cr); diff != "" {
				t.Errorf("\n%s\ne.Delete(...): -want cr, +got cr:\n%s\n", tc.reason, diff)
			}

		})
	}
}

func TestUpdate(t *testing.T) {
	type args struct {
		cr         resource.Managed
		mockClient MockClient
	}
	type want struct {
		err error
		o   managed.ExternalUpdate
		cr  resource.Managed
	}
	tests := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"NilResource": {
			reason: "Expect error if used with another resource type",
			args: args{
				cr: nil,
			},
			want: want{
				err: errors.New(errNotDirectory),
			},
		},
		"Failure": {
			reason: "",
			args: args{
				cr:         testutils.NewDirectory("dir-unittests", testutils.WithStatus(v1alpha1.DirectoryObservation{Guid: internal.Ptr("123")})),
				mockClient: MockClient{updateErr: errors.New("updateError")},
			},
			want: want{
				o:   managed.ExternalUpdate{},
				cr:  testutils.NewDirectory("dir-unittests", testutils.WithStatus(v1alpha1.DirectoryObservation{Guid: internal.Ptr("123")})),
				err: errors.New("updateError"),
			},
		},
		"Success": {
			reason: "We expect to finish gracefully if no error happened during create",
			args: args{
				cr: testutils.NewDirectory("dir-unittests", testutils.WithStatus(v1alpha1.DirectoryObservation{Guid: internal.Ptr("123")})),
				mockClient: MockClient{
					createErr: nil,
					createResult: *testutils.NewDirectory("dir-unittests",
						testutils.WithStatus(v1alpha1.DirectoryObservation{Guid: internal.Ptr("123")})),
				}},
			want: want{
				o:   managed.ExternalUpdate{ConnectionDetails: managed.ConnectionDetails{}},
				cr:  testutils.NewDirectory("dir-unittests", testutils.WithStatus(v1alpha1.DirectoryObservation{Guid: internal.Ptr("123")})),
				err: nil,
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mockKube := testutils.NewFakeKubeClientBuilder().Build()
			ctrl := external{
				newDirHandlerFn: func(client2 *btp.Client, cr *v1alpha1.Directory) directory.DirectoryClientI {
					return tc.args.mockClient
				},
				tracker: nil,
				kube:    &mockKube,
			}
			got, err := ctrl.Update(context.Background(), tc.args.cr)

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.Update(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\n%s\ne.Update(...): -want, +got:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.cr, tc.args.cr); diff != "" {
				t.Errorf("\n%s\ne.Update(...): -want cr, +got cr:\n%s\n", tc.reason, diff)
			}

		})
	}
}

// TestObserveADRCompliance tests ADR-compliant external name handling in Observe
func TestObserveADRCompliance(t *testing.T) {
	const validGUID = "12345678-1234-1234-1234-123456789012"
	const invalidGUID = "not-a-valid-guid"

	type args struct {
		cr         resource.Managed
		mockClient MockClient
		mockKube   test.MockClient
	}
	type want struct {
		err error
		o   managed.ExternalObservation
		cr  resource.Managed
	}
	tests := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"EmptyExternalNameNeedsCreation": {
			reason: "ADR: Empty external-name indicates resource needs creation",
			args: args{
				cr: testutils.NewDirectory("dir-unittests"),
				mockClient: MockClient{
					needsCreation: true,
				},
			},
			want: want{
				o:  managed.ExternalObservation{ResourceExists: false},
				cr: testutils.NewDirectory("dir-unittests"),
			},
		},
		"InvalidGUIDFormat": {
			reason: "ADR: External-name with invalid GUID format should return error",
			args: args{
				cr: testutils.NewDirectory("dir-unittests",
					testutils.WithExternalName(invalidGUID),
				),
				mockClient: MockClient{},
			},
			want: want{
				err: errors.New(fmt.Sprintf("external-name '%s' is not a valid GUID format", invalidGUID)),
				cr: testutils.NewDirectory("dir-unittests",
					testutils.WithExternalName(invalidGUID),
				),
			},
		},
		"ValidGUIDNotExists404": {
			reason: "ADR: Valid GUID that doesn't exist (404 response) should return ResourceExists: false",
			args: args{
				cr: testutils.NewDirectory("dir-unittests",
					testutils.WithExternalName(validGUID),
				),
				mockClient: MockClient{
					needsCreation:    true,
					needsCreationErr: nil,
				},
			},
			want: want{
				o: managed.ExternalObservation{ResourceExists: false},
				cr: testutils.NewDirectory("dir-unittests",
					testutils.WithExternalName(validGUID),
				),
			},
		},
		"ValidGUIDExists": {
			reason: "ADR: Valid GUID that exists should observe normally",
			args: args{
				cr: testutils.NewDirectory("dir-unittests",
					testutils.WithExternalName(validGUID),
					testutils.WithStatus(v1alpha1.DirectoryObservation{
						Guid:        internal.Ptr(validGUID),
						EntityState: internal.Ptr(v1alpha1.DirectoryEntityStateOk),
					}),
				),
				mockClient: MockClient{
					needsCreation: false,
					needsUpdate:   false,
					available:     true,
				},
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  true,
					ConnectionDetails: managed.ConnectionDetails{},
				},
				cr: testutils.NewDirectory("dir-unittests",
					testutils.WithExternalName(validGUID),
					testutils.WithStatus(v1alpha1.DirectoryObservation{
						Guid:        internal.Ptr(validGUID),
						EntityState: internal.Ptr(v1alpha1.DirectoryEntityStateOk),
					}),
					testutils.WithConditions(xpv1.Available()),
				),
			},
		},
		"DriftDetectionNeedsUpdate": {
			reason: "ADR: Drift detection should identify when update is needed",
			args: args{
				cr: testutils.NewDirectory("dir-unittests",
					testutils.WithExternalName(validGUID),
					testutils.WithStatus(v1alpha1.DirectoryObservation{
						Guid:        internal.Ptr(validGUID),
						EntityState: internal.Ptr(v1alpha1.DirectoryEntityStateOk),
					}),
				),
				mockClient: MockClient{
					needsCreation: false,
					needsUpdate:   true,
					available:     true,
				},
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  false,
					ConnectionDetails: managed.ConnectionDetails{},
				},
				cr: testutils.NewDirectory("dir-unittests",
					testutils.WithExternalName(validGUID),
					testutils.WithStatus(v1alpha1.DirectoryObservation{
						Guid:        internal.Ptr(validGUID),
						EntityState: internal.Ptr(v1alpha1.DirectoryEntityStateOk),
					}),
					testutils.WithConditions(xpv1.Available()),
				),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mockKube := testutils.NewFakeKubeClientBuilder().Build()
			ctrl := external{
				newDirHandlerFn: func(client2 *btp.Client, cr *v1alpha1.Directory) directory.DirectoryClientI {
					return tc.args.mockClient
				},
				tracker:   nil,
				kube:      &mockKube,
				btpClient: &btp.Client{},
			}
			got, err := ctrl.Observe(context.Background(), tc.args.cr)

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.Observe(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\n%s\ne.Observe(...): -want, +got:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.cr, tc.args.cr); diff != "" {
				t.Errorf("\n%s\ne.Observe(...): -want cr, +got cr:\n%s\n", tc.reason, diff)
			}
		})
	}
}

// TestCreateADRCompliance tests ADR-compliant external name handling in Create
func TestCreateADRCompliance(t *testing.T) {
	const createdGUID = "98765432-4321-4321-4321-098765432109"

	type args struct {
		cr         resource.Managed
		mockClient MockClient
	}
	type want struct {
		err              error
		o                managed.ExternalCreation
		cr               resource.Managed
		externalNameSet  bool
		expectedExternal string
	}
	tests := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"SuccessfulCreationSetsExternalName": {
			reason: "ADR: Successful creation should set external-name to returned GUID",
			args: args{
				cr: testutils.NewDirectory("dir-unittests"),
				mockClient: MockClient{
					createErr: nil,
					createResult: *testutils.NewDirectory("dir-unittests",
						testutils.WithExternalName(createdGUID),
					),
				},
			},
			want: want{
				o:                managed.ExternalCreation{ConnectionDetails: managed.ConnectionDetails{}},
				externalNameSet:  true,
				expectedExternal: createdGUID,
				cr: testutils.NewDirectory("dir-unittests",
					testutils.WithConditions(xpv1.Creating()),
				),
			},
		},
		"AlreadyExistsErrorDoesNotSetExternalName": {
			reason: "ADR: 'Resource already exists' error should NOT set external-name",
			args: args{
				cr: testutils.NewDirectory("dir-unittests"),
				mockClient: MockClient{
					createErr: errors.New("creation failed - directory already exists"),
				},
			},
			want: want{
				err:             errors.New("creation failed - directory already exists"),
				externalNameSet: false,
				cr: testutils.NewDirectory("dir-unittests",
					testutils.WithConditions(xpv1.Creating()),
				),
			},
		},
		"OtherErrorDoesNotSetExternalName": {
			reason: "ADR: Other creation errors should NOT set external-name",
			args: args{
				cr: testutils.NewDirectory("dir-unittests"),
				mockClient: MockClient{
					createErr: errors.New("internal server error"),
				},
			},
			want: want{
				err:             errors.New("internal server error"),
				externalNameSet: false,
				cr: testutils.NewDirectory("dir-unittests",
					testutils.WithConditions(xpv1.Creating()),
				),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mockKube := testutils.NewFakeKubeClientBuilder().Build()
			ctrl := external{
				newDirHandlerFn: func(client2 *btp.Client, cr *v1alpha1.Directory) directory.DirectoryClientI {
					return tc.args.mockClient
				},
				tracker: nil,
				kube:    &mockKube,
			}
			got, err := ctrl.Create(context.Background(), tc.args.cr)

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.Create(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if tc.want.err == nil {
				if diff := cmp.Diff(tc.want.o, got); diff != "" {
					t.Errorf("\n%s\ne.Create(...): -want, +got:\n%s\n", tc.reason, diff)
				}
			}
			if diff := cmp.Diff(tc.want.cr, tc.args.cr); diff != "" {
				t.Errorf("\n%s\ne.Create(...): -want cr, +got cr:\n%s\n", tc.reason, diff)
			}

			// Verify external-name handling
			dir := tc.args.cr.(*v1alpha1.Directory)
			actualExternal := meta.GetExternalName(dir)
			if tc.want.externalNameSet && actualExternal != tc.want.expectedExternal {
				t.Errorf("\n%s\nExpected external-name to be set to %s, got %s\n", tc.reason, tc.want.expectedExternal, actualExternal)
			}
			if !tc.want.externalNameSet && actualExternal != "" && actualExternal != dir.Name {
				t.Errorf("\n%s\nExpected external-name NOT to be set, but got %s\n", tc.reason, actualExternal)
			}
		})
	}
}

// TestDeleteADRCompliance tests ADR-compliant external name handling in Delete
func TestDeleteADRCompliance(t *testing.T) {
	const validGUID = "12345678-1234-1234-1234-123456789012"

	type args struct {
		cr         resource.Managed
		mockClient MockClient
	}
	type want struct {
		err error
		cr  resource.Managed
	}
	tests := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"404NotFoundNotError": {
			reason: "ADR: 404 response should NOT be treated as error",
			args: args{
				cr: testutils.NewDirectory("dir-unittests",
					testutils.WithExternalName(validGUID),
				),
				mockClient: MockClient{
					deleteErr: errors.New("directory not found"),
				},
			},
			want: want{
				err: nil, // Should not be an error
				cr: testutils.NewDirectory("dir-unittests",
					testutils.WithExternalName(validGUID),
					testutils.WithConditions(xpv1.Deleting()),
				),
			},
		},
		"DeletingStateSkipsDelete": {
			reason: "ADR: Resource in DELETING state should skip deletion call",
			args: args{
				cr: testutils.NewDirectory("dir-unittests",
					testutils.WithExternalName(validGUID),
					testutils.WithStatus(v1alpha1.DirectoryObservation{
						Guid:        internal.Ptr(validGUID),
						EntityState: internal.Ptr("DELETING"),
					}),
				),
				mockClient: MockClient{
					deleteErr: errors.New("should not be called"),
				},
			},
			want: want{
				err: nil,
				cr: testutils.NewDirectory("dir-unittests",
					testutils.WithExternalName(validGUID),
					testutils.WithStatus(v1alpha1.DirectoryObservation{
						Guid:        internal.Ptr(validGUID),
						EntityState: internal.Ptr("DELETING"),
					}),
				),
			},
		},
		"SuccessfulDeletion": {
			reason: "ADR: Successful deletion should complete without error",
			args: args{
				cr: testutils.NewDirectory("dir-unittests",
					testutils.WithExternalName(validGUID),
					testutils.WithStatus(v1alpha1.DirectoryObservation{
						Guid:        internal.Ptr(validGUID),
						EntityState: internal.Ptr(v1alpha1.DirectoryEntityStateOk),
					}),
				),
				mockClient: MockClient{
					deleteErr: nil,
				},
			},
			want: want{
				err: nil,
				cr: testutils.NewDirectory("dir-unittests",
					testutils.WithExternalName(validGUID),
					testutils.WithConditions(xpv1.Deleting()),
					testutils.WithStatus(v1alpha1.DirectoryObservation{
						Guid:        internal.Ptr(validGUID),
						EntityState: internal.Ptr(v1alpha1.DirectoryEntityStateOk),
					}),
				),
			},
		},
		"OtherDeleteError": {
			reason: "ADR: Other deletion errors should be returned",
			args: args{
				cr: testutils.NewDirectory("dir-unittests",
					testutils.WithExternalName(validGUID),
					testutils.WithStatus(v1alpha1.DirectoryObservation{
						Guid:        internal.Ptr(validGUID),
						EntityState: internal.Ptr(v1alpha1.DirectoryEntityStateOk),
					}),
				),
				mockClient: MockClient{
					deleteErr: errors.New("internal server error"),
				},
			},
			want: want{
				err: errors.New("internal server error"),
				cr: testutils.NewDirectory("dir-unittests",
					testutils.WithExternalName(validGUID),
					testutils.WithConditions(xpv1.Deleting()),
					testutils.WithStatus(v1alpha1.DirectoryObservation{
						Guid:        internal.Ptr(validGUID),
						EntityState: internal.Ptr(v1alpha1.DirectoryEntityStateOk),
					}),
				),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mockKube := testutils.NewFakeKubeClientBuilder().Build()
			ctrl := external{
				newDirHandlerFn: func(client2 *btp.Client, cr *v1alpha1.Directory) directory.DirectoryClientI {
					return tc.args.mockClient
				},
				tracker: nil,
				kube:    &mockKube,
			}
			_, err := ctrl.Delete(context.Background(), tc.args.cr)

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.Delete(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.cr, tc.args.cr); diff != "" {
				t.Errorf("\n%s\ne.Delete(...): -want cr, +got cr:\n%s\n", tc.reason, diff)
			}
		})
	}
}
