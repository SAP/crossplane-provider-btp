package cloudfoundry

import (
	"context"
	"net/http"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	environments "github.com/sap/crossplane-provider-btp/internal/clients/cfenvironment"
	"github.com/sap/crossplane-provider-btp/internal/controller/environment/cloudfoundry/fake"
	provisioningclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-provisioning-service-api-go/pkg"
)

// Unlike many Kubernetes projects Crossplane does not use third party testing
// libraries, per the common Go test review comments. Crossplane encourages the
// use of table driven unit tests. The tests of the crossplane-runtime project
// are representative of the testing style Crossplane encourages.
//
// https://github.com/golang/go/wiki/TestComments
// https://github.com/crossplane/crossplane/blob/master/CONTRIBUTING.md#contributing-code

var aUser = v1alpha1.User{Username: "aaa@bbb.com"}

func TestObserve(t *testing.T) {
	type args struct {
		cr     resource.Managed
		client environments.Client
	}

	type want struct {
		o   managed.ExternalObservation
		cr  resource.Managed
		err error
	}

	var cases = map[string]struct {
		args args
		want want
	}{
		"NilManaged": {
			args: args{
				client: fake.MockClient{},
				cr:     nil,
			},
			want: want{
				o:   managed.ExternalObservation{},
				err: errors.New(errNotEnvironment),
			},
		},
		"EmptyExternalName_NoResourceExists": {
			args: args{
				client: fake.MockClient{},
				cr:     environment(), // No external-name annotation
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists: false,
				},
				err: nil,
				cr:  environment(),
			},
		},
		"ValidGUID_ResourceNotFound_Drift": {
			args: args{
				client: fake.MockClient{MockDescribeCluster: func(cr v1alpha1.CloudFoundryEnvironment) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, []v1alpha1.User, environments.NeedsExternalNameFormatMigration, error) {
					return nil, nil, false, nil // Resource not found
				}},
				cr: environment(withAnnotaions(map[string]string{"crossplane.io/external-name": "550e8400-e29b-41d4-a716-446655440000"})),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists: false,
				},
				err: nil,
				cr:  environment(withAnnotaions(map[string]string{"crossplane.io/external-name": "550e8400-e29b-41d4-a716-446655440000"})),
			},
		},
		"ValidGUID_SuccessfulObservation": {
			args: args{
				client: fake.MockClient{MockDescribeCluster: func(cr v1alpha1.CloudFoundryEnvironment) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, []v1alpha1.User, environments.NeedsExternalNameFormatMigration, error) {
					return &provisioningclient.BusinessEnvironmentInstanceResponseObject{
						Id:     internal.Ptr("550e8400-e29b-41d4-a716-446655440000"),
						State:  internal.Ptr("OK"),
						Labels: internal.Ptr("{\"Org Name\":\"test-org\"}"),
					}, []v1alpha1.User{aUser}, false, nil
				}},
				cr: environment(withAnnotaions(map[string]string{"crossplane.io/external-name": "550e8400-e29b-41d4-a716-446655440000"})),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  true,
					ConnectionDetails: managed.ConnectionDetails{"__raw": []byte("{\"Org Name\":\"test-org\"}"), "orgName": []byte("test-org")},
				},
				err: nil,
				cr: environment(
					withAnnotaions(map[string]string{"crossplane.io/external-name": "550e8400-e29b-41d4-a716-446655440000"}),
					withConditions(xpv1.Available()),
					withStatus(v1alpha1.CfEnvironmentObservation{
						EnvironmentObservation: v1alpha1.EnvironmentObservation{
							State:  internal.Ptr("OK"),
							Labels: internal.Ptr("{\"Org Name\":\"test-org\"}"),
						},
						Managers: []v1alpha1.User{aUser},
					}),
				),
			},
		},
		"LegacyFormat_SuccessfulMigrationToGUID": {
			args: args{
				client: fake.MockClient{MockDescribeCluster: func(cr v1alpha1.CloudFoundryEnvironment) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, []v1alpha1.User, environments.NeedsExternalNameFormatMigration, error) {
					return &provisioningclient.BusinessEnvironmentInstanceResponseObject{
						Id:     internal.Ptr("550e8400-e29b-41d4-a716-446655440000"),
						State:  internal.Ptr("OK"),
						Labels: internal.Ptr("{\"Org Name\":\"legacy-org\"}"),
					}, []v1alpha1.User{}, true, nil // needsExternalNameFormatMigration = true
				}},
				cr: environment(withAnnotaions(map[string]string{"crossplane.io/external-name": "legacy-org-name"})),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  true,
					ConnectionDetails: managed.ConnectionDetails{"__raw": []byte("{\"Org Name\":\"legacy-org\"}"), "orgName": []byte("legacy-org")},
				},
				err: nil,
				cr: environment(
					// External-name should be migrated to GUID
					withAnnotaions(map[string]string{"crossplane.io/external-name": "550e8400-e29b-41d4-a716-446655440000"}),
					withConditions(xpv1.Available()),
					withStatus(v1alpha1.CfEnvironmentObservation{
						EnvironmentObservation: v1alpha1.EnvironmentObservation{
							State:  internal.Ptr("OK"),
							Labels: internal.Ptr("{\"Org Name\":\"legacy-org\"}"),
						},
					}),
				),
			},
		},
		"ErrorGettingCFEnvironment": {
			args: args{
				client: fake.MockClient{MockDescribeCluster: func(cr v1alpha1.CloudFoundryEnvironment) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, []v1alpha1.User, environments.NeedsExternalNameFormatMigration, error) {
					return nil, nil, false, errors.New("Could not call backend")
				}},
				cr: environment(),
			},
			want: want{
				o:   managed.ExternalObservation{},
				err: errors.New("Could not call backend"),
				cr:  environment(),
			},
		},
		"NeedsCreate": {
			args: args{
				client: fake.MockClient{MockDescribeCluster: func(cr v1alpha1.CloudFoundryEnvironment) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, []v1alpha1.User, environments.NeedsExternalNameFormatMigration, error) {
					return nil, nil, false, nil
				}},
				cr: environment(),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists: false,
				},
				err: nil,
				cr:  environment(withConditions(xpv1.Unavailable())),
			},
		},
		"SuccessfulAvailableAndUpToDate": {
			args: args{
				client: fake.MockClient{MockDescribeCluster: func(cr v1alpha1.CloudFoundryEnvironment) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, []v1alpha1.User, environments.NeedsExternalNameFormatMigration, error) {
					return &provisioningclient.BusinessEnvironmentInstanceResponseObject{
						State:  internal.Ptr("OK"),
						Labels: internal.Ptr("{\"Org Name\":\"test-org\"}"),
					}, []v1alpha1.User{aUser}, false, nil
				}, MockNeedsUpdate: func(cr v1alpha1.CloudFoundryEnvironment) bool {
					return false
				}},
				cr: environment(withUID("1234"),
					withData(v1alpha1.CfEnvironmentParameters{OrgName: "test-org", Managers: []string{aUser.Username}}),
					withStatus(v1alpha1.CfEnvironmentObservation{
						EnvironmentObservation: v1alpha1.EnvironmentObservation{
							State:  internal.Ptr("OK"),
							Labels: internal.Ptr("{\"Org Name\":\"test-org\"}"),
						},
						Managers: []v1alpha1.User{aUser},
					})),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  true,
					ConnectionDetails: managed.ConnectionDetails{"__raw": []byte("{\"Org Name\":\"test-org\"}"), "orgName": []byte("test-org")},
				},
				err: nil,
				cr: environment(withUID("1234"), withConditions(xpv1.Available()),
					withData(v1alpha1.CfEnvironmentParameters{OrgName: "test-org", Managers: []string{aUser.Username}}),
					withStatus(v1alpha1.CfEnvironmentObservation{
						EnvironmentObservation: v1alpha1.EnvironmentObservation{
							State:  internal.Ptr("OK"),
							Labels: internal.Ptr("{\"Org Name\":\"test-org\"}"),
						},
						Managers: []v1alpha1.User{aUser},
					},
					)),
			},
		},
		"ExistingButNotAvailable": {
			args: args{
				client: fake.MockClient{MockDescribeCluster: func(cr v1alpha1.CloudFoundryEnvironment) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, []v1alpha1.User, environments.NeedsExternalNameFormatMigration, error) {
					return &provisioningclient.BusinessEnvironmentInstanceResponseObject{
						State:  internal.Ptr("CREATING"),
						Labels: internal.Ptr("{}"),
					}, []v1alpha1.User{aUser}, false, nil
				}, MockNeedsUpdate: func(cr v1alpha1.CloudFoundryEnvironment) bool {
					return false
				}},
				cr: environment(withUID("1234"),
					withData(v1alpha1.CfEnvironmentParameters{Managers: []string{aUser.Username}}),
					withStatus(v1alpha1.CfEnvironmentObservation{
						EnvironmentObservation: v1alpha1.EnvironmentObservation{
							State:  internal.Ptr("CREATING"),
							Labels: internal.Ptr("{}"),
						},
						Managers: []v1alpha1.User{aUser},
					})),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  true,
					ConnectionDetails: managed.ConnectionDetails{"__raw": []byte("{}")},
				},
				err: nil,
				cr: environment(withUID("1234"), withConditions(xpv1.Unavailable()),
					withData(v1alpha1.CfEnvironmentParameters{Managers: []string{aUser.Username}}),
					withStatus(v1alpha1.CfEnvironmentObservation{
						EnvironmentObservation: v1alpha1.EnvironmentObservation{
							State:  internal.Ptr("CREATING"),
							Labels: internal.Ptr("{}"),
						},
						Managers: []v1alpha1.User{aUser},
					},
					)),
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{client: tc.args.client, kube: &test.MockClient{MockUpdate: test.NewMockUpdateFn(nil)}}
			got, err := e.Observe(context.Background(), tc.args.cr)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\ne.Observe(...): -want error, +got error:\n%s\n", diff)
			}
			if diff := cmp.Diff(tc.want.cr, tc.args.cr, test.EquateConditions()); diff != "" {
				t.Errorf("\ne.Observe(...): -want error, +got error:\n%s\n", diff)
			}
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\ne.Observe(...): -want, +got:\n%s\n", diff)
			}
		})
	}
}

func TestCreate(t *testing.T) {
	type args struct {
		cr     resource.Managed
		client environments.Client
	}

	type want struct {
		o   managed.ExternalCreation
		cr  resource.Managed
		err error
	}

	var cases = map[string]struct {
		args args
		want want
	}{
		"NilManaged": {
			args: args{
				client: fake.MockClient{},
				cr:     nil,
			},
			want: want{
				o:   managed.ExternalCreation{},
				err: errors.New(errNotEnvironment),
			},
		},
		"CreateError": {
			args: args{
				client: fake.MockClient{MockCreate: func(cr v1alpha1.CloudFoundryEnvironment) (string, error) {
					return "", errors.New("Could not call backend")
				}},
				cr: environment(),
			},
			want: want{
				o:   managed.ExternalCreation{},
				err: errors.New("Could not call backend"),
				cr:  environment(),
			},
		},
		"Successful_SetsExternalNameToGUID": {
			args: args{
				client: fake.MockClient{MockCreate: func(cr v1alpha1.CloudFoundryEnvironment) (string, error) {
					return "550e8400-e29b-41d4-a716-446655440000", nil // Return GUID
				},
				},
				cr: environment(withData(v1alpha1.CfEnvironmentParameters{OrgName: "test-org", EnvironmentName: "test-env"})),
			},
			want: want{
				o:   managed.ExternalCreation{ConnectionDetails: managed.ConnectionDetails{}},
				err: nil,
				cr: environment(withData(v1alpha1.CfEnvironmentParameters{OrgName: "test-org", EnvironmentName: "test-env"}),
					withAnnotaions(map[string]string{
						"crossplane.io/external-name": "550e8400-e29b-41d4-a716-446655440000",
					})),
			},
		},
		"CreateError_AlreadyExists_DoesNotSetExternalName": {
			args: args{
				client: fake.MockClient{MockCreate: func(cr v1alpha1.CloudFoundryEnvironment) (string, error) {
					return "", errors.New("resource already exists")
				}},
				cr: environment(),
			},
			want: want{
				o:   managed.ExternalCreation{},
				err: errors.New("resource already exists"),
				cr:  environment(), // No external-name should be set
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{client: tc.args.client, kube: &test.MockClient{MockUpdate: test.NewMockUpdateFn(nil)}}
			got, err := e.Create(context.Background(), tc.args.cr)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\ne.Observe(...): -want error, +got error:\n%s\n", diff)
			}
			if diff := cmp.Diff(tc.want.cr, tc.args.cr, test.EquateConditions()); diff != "" {
				t.Errorf("\ne.Observe(...): -want error, +got error:\n%s\n", diff)
			}
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\ne.Observe(...): -want, +got:\n%s\n", diff)
			}
		})
	}
}

func TestDelete(t *testing.T) {
	type args struct {
		cr     resource.Managed
		client environments.Client
	}

	type want struct {
		cr  resource.Managed
		err error
	}

	var cases = map[string]struct {
		args args
		want want
	}{
		"NilManaged": {
			args: args{
				client: fake.MockClient{},
				cr:     nil,
			},
			want: want{
				err: errors.New(errNotEnvironment),
			},
		},
		"DeleteError": {
			args: args{
				client: fake.MockClient{MockDelete: func(cr v1alpha1.CloudFoundryEnvironment) (*http.Response, error) {
					return nil, errors.New("Could not call backend")
				}},
				cr: environment(),
			},
			want: want{
				err: errors.New("Could not call backend"),
				cr:  environment(withConditions(xpv1.Deleting())),
			},
		},
		"DeleteError-404 not found": {
			args: args{
				client: fake.MockClient{MockDelete: func(cr v1alpha1.CloudFoundryEnvironment) (*http.Response, error) {
					return &http.Response{StatusCode: 404}, errors.New("404 not found")
				}},
				cr: environment(),
			},
			want: want{
				err: nil,
				cr:  environment(withConditions(xpv1.Deleting())), //TODO what to do here?
			},
		},
		"Successful": {
			args: args{
				client: fake.MockClient{MockDelete: func(cr v1alpha1.CloudFoundryEnvironment) (*http.Response, error) {
					return nil, nil
				},
				},
				cr: environment(),
			},
			want: want{
				err: nil,
				cr:  environment(withConditions(xpv1.Deleting())),
			},
		},
		"AlreadyDeleting_SkipsDeleteCall": {
			args: args{
				client: fake.MockClient{
					MockDelete: func(cr v1alpha1.CloudFoundryEnvironment) (*http.Response, error) {
						// This should NOT be called
						panic("Delete should not be called when resource is already deleting")
					},
				},
				cr: environment(
					withStatus(v1alpha1.CfEnvironmentObservation{
						EnvironmentObservation: v1alpha1.EnvironmentObservation{
							State: internal.Ptr(v1alpha1.InstanceStateDeleting),
						},
					}),
				),
			},
			want: want{
				err: nil,
				cr: environment(
					withConditions(xpv1.Deleting()),
					withStatus(v1alpha1.CfEnvironmentObservation{
						EnvironmentObservation: v1alpha1.EnvironmentObservation{
							State: internal.Ptr(v1alpha1.InstanceStateDeleting),
						},
					}),
				),
			},
		},
		"UsesExternalNameForDeletion": {
			args: args{
				client: fake.MockClient{MockDelete: func(cr v1alpha1.CloudFoundryEnvironment) (*http.Response, error) {
					// Verify external-name is accessible (implicitly tested by not panicking)
					return &http.Response{StatusCode: 200}, nil
				}},
				cr: environment(withAnnotaions(map[string]string{"crossplane.io/external-name": "550e8400-e29b-41d4-a716-446655440000"})),
			},
			want: want{
				err: nil,
				cr:  environment(withAnnotaions(map[string]string{"crossplane.io/external-name": "550e8400-e29b-41d4-a716-446655440000"}), withConditions(xpv1.Deleting())),
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{client: tc.args.client}
			_, err := e.Delete(context.Background(), tc.args.cr)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\ne.Observe(...): -want error, +got error:\n%s\n", diff)
			}
			if diff := cmp.Diff(tc.want.cr, tc.args.cr, test.EquateConditions()); diff != "" {
				t.Errorf("\ne.Observe(...): -want error, +got error:\n%s\n", diff)
			}
		})
	}
}

type environmentModifier func(foundryEnvironment *v1alpha1.CloudFoundryEnvironment)

func withConditions(c ...xpv1.Condition) environmentModifier {
	return func(r *v1alpha1.CloudFoundryEnvironment) { r.Status.Conditions = c }
}
func withUID(uid types.UID) environmentModifier {
	return func(r *v1alpha1.CloudFoundryEnvironment) { r.UID = uid }
}
func withStatus(status v1alpha1.CfEnvironmentObservation) environmentModifier {
	return func(r *v1alpha1.CloudFoundryEnvironment) {
		r.Status.AtProvider = status
	}
}
func withData(data v1alpha1.CfEnvironmentParameters) environmentModifier {
	return func(r *v1alpha1.CloudFoundryEnvironment) {
		r.Spec.ForProvider = data
	}
}

func withAnnotaions(annotations map[string]string) environmentModifier {
	return func(r *v1alpha1.CloudFoundryEnvironment) {
		r.Annotations = annotations
	}
}

func environment(m ...environmentModifier) *v1alpha1.CloudFoundryEnvironment {
	cr := &v1alpha1.CloudFoundryEnvironment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cf",
		},
	}
	for _, f := range m {
		f(cr)
	}
	return cr
}
