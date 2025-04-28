package cloudfoundry

import (
	"context"
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
		"ErrorGettingCFEnvironment": {
			args: args{
				client: fake.MockClient{MockDescribeCluster: func(cr v1alpha1.CloudFoundryEnvironment) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, []v1alpha1.User, error) {
					return nil, nil, errors.New("Could not call backend")
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
				client: fake.MockClient{MockDescribeCluster: func(cr v1alpha1.CloudFoundryEnvironment) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, []v1alpha1.User, error) {
					return nil, nil, nil
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
				client: fake.MockClient{MockDescribeCluster: func(cr v1alpha1.CloudFoundryEnvironment) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, []v1alpha1.User, error) {
					return &provisioningclient.BusinessEnvironmentInstanceResponseObject{
						State:  internal.Ptr("OK"),
						Labels: internal.Ptr("{\"Org Name\":\"test-org\"}"),
					}, []v1alpha1.User{aUser}, nil
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
				client: fake.MockClient{MockDescribeCluster: func(cr v1alpha1.CloudFoundryEnvironment) (*provisioningclient.BusinessEnvironmentInstanceResponseObject, []v1alpha1.User, error) {
					return &provisioningclient.BusinessEnvironmentInstanceResponseObject{
						State:  internal.Ptr("CREATING"),
						Labels: internal.Ptr("{}"),
					}, []v1alpha1.User{aUser}, nil
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
		"Successful": {
			args: args{
				client: fake.MockClient{MockCreate: func(cr v1alpha1.CloudFoundryEnvironment) (string, error) {
					return "test-org",nil
				},
				},
				cr: environment(withData(v1alpha1.CfEnvironmentParameters{OrgName: "test-org", EnvironmentName: "test-env"})),
			},
			want: want{
				o:   managed.ExternalCreation{ConnectionDetails: managed.ConnectionDetails{}},
				err: nil,
				cr:  environment(withData(v1alpha1.CfEnvironmentParameters{OrgName: "test-org", EnvironmentName: "test-env"}),
								withAnnotaions(map[string]string{
									"crossplane.io/external-name": "test-org",
								}),),
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
				client: fake.MockClient{MockDelete: func(cr v1alpha1.CloudFoundryEnvironment) error {
					return errors.New("Could not call backend")
				}},
				cr: environment(),
			},
			want: want{
				err: errors.New("Could not call backend"),
				cr:  environment(withConditions(xpv1.Deleting())),
			},
		},
		"Successful": {
			args: args{
				client: fake.MockClient{MockDelete: func(cr v1alpha1.CloudFoundryEnvironment) error {
					return nil
				},
				},
				cr: environment(),
			},
			want: want{
				err: nil,
				cr:  environment(withConditions(xpv1.Deleting())),
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{client: tc.args.client}
			err := e.Delete(context.Background(), tc.args.cr)
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
	return func(r *v1alpha1.CloudFoundryEnvironment) { r.Status.ConditionedStatus.Conditions = c }
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
		r.ObjectMeta.Annotations = annotations
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
