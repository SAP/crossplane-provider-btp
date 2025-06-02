package rolecollectionassignment

import (
	"context"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	apiError          = errors.New("apiError")
	notImplementedErr = errors.New(errNotImplemented)
)

func TestObserve(t *testing.T) {
	type args struct {
		cr     *v1alpha1.RoleCollectionAssignment
		client *RoleAssignerMock
	}

	type want struct {
		cr               *v1alpha1.RoleCollectionAssignment
		o                managed.ExternalObservation
		err              error
		CalledIdentifier string
	}

	cases := map[string]struct {
		args args
		want want
	}{
		"LookupError": {
			args: args{
				cr: cr(),
				client: &RoleAssignerMock{
					err: apiError,
				},
			},
			want: want{
				cr:  cr(),
				o:   managed.ExternalObservation{},
				err: apiError,
			},
		},
		"user needs creation": {
			args: args{
				cr: cr(withUser("someUser")),
				client: &RoleAssignerMock{
					hasRole: false,
					err:     nil,
				},
			},
			want: want{
				cr: cr(withUser("someUser")),
				o: managed.ExternalObservation{
					ResourceExists: false,
				},
				CalledIdentifier: "someUser",
			},
		},
		"group needs creation": {
			args: args{
				cr: cr(withGroup("someGroup")),
				client: &RoleAssignerMock{
					hasRole: false,
					err:     nil,
				},
			},
			want: want{
				cr: cr(withGroup("someGroup")),
				o: managed.ExternalObservation{
					ResourceExists: false,
				},
				CalledIdentifier: "someGroup",
			},
		},
		"group available": {
			args: args{
				cr: cr(withGroup("someGroup")),
				client: &RoleAssignerMock{
					hasRole: true,
					err:     nil,
				},
			},
			want: want{
				cr: cr(WithConditions(xpv1.Available()), withGroup("someGroup")),
				o: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  true,
					ConnectionDetails: managed.ConnectionDetails{},
				},
				CalledIdentifier: "someGroup",
			},
		},
		"user available": {
			args: args{
				cr: cr(withUser("someUser")),
				client: &RoleAssignerMock{
					hasRole: true,
					err:     nil,
				},
			},
			want: want{
				cr: cr(WithConditions(xpv1.Available()), withUser("someUser")),
				o: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  true,
					ConnectionDetails: managed.ConnectionDetails{},
				},
				CalledIdentifier: "someUser",
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{client: tc.args.client}
			got, err := e.Observe(context.Background(), tc.args.cr)
			if diff := cmp.Diff(&tc.want.CalledIdentifier, tc.args.client.CalledIdentifier); diff != "" {
				t.Errorf("\n%s\ne.Observe(...): -want, +CalledIdentifier:\n", diff)
			}
			expectedErrorBehaviour(t, tc.want.err, err)
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\n%s\ne.Observe(...): -want, +got:\n", diff)
			}
			if diff := cmp.Diff(tc.want.cr, tc.args.cr); diff != "" {
				t.Errorf("\ne.Observe(): expected cr after operation -want, +got:\n%s\n", diff)
			}
		})
	}
}

func TestCreate(t *testing.T) {
	type args struct {
		cr     *v1alpha1.RoleCollectionAssignment
		client *RoleAssignerMock
	}

	type want struct {
		o                managed.ExternalCreation
		cr               *v1alpha1.RoleCollectionAssignment
		err              error
		CalledIdentifier string
	}

	cases := map[string]struct {
		args args
		want want
	}{
		"ApiError UserAssigner": {
			args: args{
				cr: cr(withCredsCustom(), withUser("someUser")),
				client: &RoleAssignerMock{
					err: apiError,
				},
			},
			want: want{
				cr:               cr(WithConditions(xpv1.Creating()), withCredsCustom(), withUser("someUser")),
				o:                managed.ExternalCreation{},
				err:              apiError,
				CalledIdentifier: "someUser",
			},
		},
		"ApiError GroupAssigner": {
			args: args{
				cr: cr(withGroup("someGroup"), withCredsCustom()),
				client: &RoleAssignerMock{
					err: apiError,
				},
			},
			want: want{
				cr:               cr(WithConditions(xpv1.Creating()), withGroup("someGroup"), withCredsCustom()),
				o:                managed.ExternalCreation{},
				err:              apiError,
				CalledIdentifier: "someGroup",
			},
		},
		"Successful UserAssigner": {
			args: args{
				cr: cr(withUser("someUser"), withCredsCustom()),
				client: &RoleAssignerMock{
					hasRole: true,
					err:     nil,
				},
			},
			want: want{
				cr: cr(WithConditions(xpv1.Creating()), withUser("someUser"), withCredsCustom()),
				o: managed.ExternalCreation{
					ConnectionDetails: managed.ConnectionDetails{},
				},
				CalledIdentifier: "someUser",
			},
		},
		"Successful GroupAssigner": {
			args: args{
				cr: cr(withGroup("someGroup"), withCredsCustom()),
				client: &RoleAssignerMock{
					hasRole: true,
					err:     nil,
				},
			},
			want: want{
				cr: cr(WithConditions(xpv1.Creating()), withGroup("someGroup"), withCredsCustom()),
				o: managed.ExternalCreation{
					ConnectionDetails: managed.ConnectionDetails{},
				},
				CalledIdentifier: "someGroup",
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{client: tc.args.client}
			got, err := e.Create(context.Background(), tc.args.cr)

			if diff := cmp.Diff(&tc.want.CalledIdentifier, tc.args.client.CalledIdentifier); diff != "" {
				t.Errorf("\n%s\ne.Create(...): -want, +CalledIdentifier:\n", diff)
			}
			expectedErrorBehaviour(t, tc.want.err, err)
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\n%s\ne.Create(...): -want, +got:\n", diff)
			}
			if diff := cmp.Diff(tc.want.cr, tc.args.cr); diff != "" {
				t.Errorf("\ne.Create(): expected cr after operation -want, +got:\n%s\n", diff)
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	type args struct {
		cr *v1alpha1.RoleCollectionAssignment
	}
	type want struct {
		o   managed.ExternalUpdate
		cr  *v1alpha1.RoleCollectionAssignment
		err error
	}

	cases := map[string]struct {
		args args
		want want
	}{
		// we always expect an error
		"NotImplemented": {
			args: args{
				cr: cr(),
			},
			want: want{
				cr:  cr(),
				o:   managed.ExternalUpdate{},
				err: notImplementedErr,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{client: nil}
			got, err := e.Update(context.Background(), tc.args.cr)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.Update(...): -want error, +got error:\n", diff)
			}
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\n%s\ne.Update(...): -want, +got:\n", diff)
			}
			if diff := cmp.Diff(tc.want.cr, tc.args.cr); diff != "" {
				t.Errorf("\ne.Update(): expected cr after operation -want, +got:\n%s\n", diff)
			}
		})
	}
}

func TestDelete(t *testing.T) {
	type args struct {
		cr     *v1alpha1.RoleCollectionAssignment
		client *RoleAssignerMock
	}

	type want struct {
		cr               *v1alpha1.RoleCollectionAssignment
		err              error
		CalledIdentifier string
	}

	cases := map[string]struct {
		args args
		want want
	}{
		"ApiError": {
			args: args{
				cr: cr(),
				client: &RoleAssignerMock{
					err: apiError,
				},
			},
			want: want{
				cr:  cr(WithConditions(xpv1.Deleting())),
				err: apiError,
			},
		},
		"Successful UserAssigner": {
			args: args{
				cr: cr(withUser("someUser")),
				client: &RoleAssignerMock{
					err: nil,
				},
			},
			want: want{
				cr:               cr(WithConditions(xpv1.Deleting()), withUser("someUser")),
				CalledIdentifier: "someUser",
			},
		},
		"Successful GroupAssigner": {
			args: args{
				cr: cr(withGroup("someGroup")),
				client: &RoleAssignerMock{
					err: nil,
				},
			},
			want: want{
				cr:               cr(WithConditions(xpv1.Deleting()), withGroup("someGroup")),
				CalledIdentifier: "someGroup",
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{client: tc.args.client}
			err := e.Delete(context.Background(), tc.args.cr)
			if diff := cmp.Diff(&tc.want.CalledIdentifier, tc.args.client.CalledIdentifier); diff != "" {
				t.Errorf("\n%s\ne.Delete(...): -want, +CalledIdentifier:\n", diff)
			}
			expectedErrorBehaviour(t, tc.want.err, err)
			if diff := cmp.Diff(tc.want.cr, tc.args.cr); diff != "" {
				t.Errorf("\ne.Create(): expected cr after operation -want, +got:\n%s\n", diff)
			}
		})
	}
}

func TestConnect(t *testing.T) {
	trackErr := errors.New("trackError")

	kubeStubCustom := func(err error, secretData map[string][]byte) client.Client {
		return &test.MockClient{
			MockGet: test.NewMockGetFn(err, func(obj client.Object) error {
				secret := obj.(*corev1.Secret)
				secret.Data = secretData
				return nil
			}),
		}
	}

	kubeStubUpjet := func(err error, secretObj *corev1.Secret) client.Client {
		return &test.MockClient{
			MockGet: test.NewMockGetFn(err, func(obj client.Object) error {
				if err != nil || secretObj == nil {
					return err
				}
				secret := obj.(*corev1.Secret)
				*secret = *secretObj
				return nil
			}),
		}
	}

	type args struct {
		cr                 *v1alpha1.RoleCollectionAssignment
		track              resource.Tracker
		resourcetracker    tracking.ReferenceResolverTracker
		kube               client.Client
		newUserAssignerFn  func(_ *v1alpha1.XsuaaBinding) (RoleAssigner, error)
		newGroupAssignerFn func(_ *v1alpha1.XsuaaBinding) (RoleAssigner, error)
	}

	type want struct {
		err             error
		externalCreated bool
	}

	cases := map[string]struct {
		args args
		want want
	}{
		"TrackError": {
			args: args{
				cr:              cr(),
				track:           newTracker(trackErr),
				resourcetracker: newResourceTracker(nil),
			},
			want: want{
				err: trackErr,
			},
		},
		// existance of secret ref is enforced on schema level and needs to be verified in e2e tests
		"Not found secret Custom": {
			args: args{
				cr:              cr(withCredsCustom()),
				track:           newTracker(nil),
				resourcetracker: newResourceTracker(nil),
				kube:            kubeStubCustom(v1alpha1.FailedToGetSecret, nil),
			},
			want: want{
				err: v1alpha1.FailedToGetSecret,
			},
		},
		"Not found secret Upjet": {
			args: args{
				cr:              cr(withCredsUpjet()),
				track:           newTracker(nil),
				resourcetracker: newResourceTracker(nil),
				kube:            kubeStubUpjet(v1alpha1.FailedToGetSecret, nil),
			},
			want: want{
				err: v1alpha1.FailedToGetSecret,
			},
		},
		"Secret without key": {
			args: args{
				cr:              cr(withCredsCustom()),
				track:           newTracker(nil),
				resourcetracker: newResourceTracker(nil),
				kube:            kubeStubCustom(nil, nil),
			},
			want: want{
				err: v1alpha1.InvalidXsuaaCredentials,
			},
		},
		"NewUserAssignerFn err Custom": {
			args: args{
				cr:              cr(withCredsCustom(), withUser("someUser")),
				track:           newTracker(nil),
				resourcetracker: newResourceTracker(nil),
				kube: kubeStubCustom(nil, map[string][]byte{
					"credentials": []byte(`{"clientid": "clientid", "clientsecret": "clientsecret", "tokenurl": "tokenurl", "apiurl": "apiurl"}`),
				}),
				newUserAssignerFn: newAssignerStub(v1alpha1.InvalidXsuaaCredentials),
			},
			want: want{
				err: v1alpha1.InvalidXsuaaCredentials,
			},
		},
		"NewUserAssignerFn success Custom": {
			args: args{
				cr:              cr(withCredsCustom(), withUser("someUser")),
				track:           newTracker(nil),
				resourcetracker: newResourceTracker(nil),
				kube: kubeStubCustom(nil, map[string][]byte{
					"credentials": []byte(`{"clientid": "clientid", "clientsecret": "clientsecret", "tokenurl": "tokenurl", "apiurl": "apiurl"}`),
				}),
				newUserAssignerFn: newAssignerStub(nil),
			},
			want: want{
				err:             nil,
				externalCreated: true,
			},
		},
		"NewGroupAssignerFn err Custom": {
			args: args{
				cr:              cr(withCredsCustom(), withGroup("someGroup")),
				track:           newTracker(nil),
				resourcetracker: newResourceTracker(nil),
				kube: kubeStubCustom(nil, map[string][]byte{
					"credentials": []byte(`{"clientid": "clientid", "clientsecret": "clientsecret", "tokenurl": "tokenurl", "apiurl": "apiurl"}`),
				}),
				newGroupAssignerFn: newAssignerStub(v1alpha1.InvalidXsuaaCredentials),
			},
			want: want{
				err: v1alpha1.InvalidXsuaaCredentials,
			},
		},
		"NewGroupAssignerFn success Custom": {
			args: args{
				cr:              cr(withCredsCustom(), withGroup("someGroup")),
				track:           newTracker(nil),
				resourcetracker: newResourceTracker(nil),
				kube: kubeStubCustom(nil, map[string][]byte{
					"credentials": []byte(`{"clientid": "clientid", "clientsecret": "clientsecret", "tokenurl": "tokenurl", "apiurl": "apiurl"}`),
				}),
				newGroupAssignerFn: newAssignerStub(nil),
			},
			want: want{
				err:             nil,
				externalCreated: true,
			},
		},

		"NewUserAssignerFn err Upjet": {
			args: args{
				cr:              cr(withCredsUpjet(), withUser("someUser")),
				track:           newTracker(nil),
				resourcetracker: newResourceTracker(nil),
				kube: kubeStubUpjet(nil, &corev1.Secret{Data: map[string][]byte{
					"attribute.api_url":       []byte("aurl"),
					"attribute.client_id":     []byte("cid"),
					"attribute.client_secret": []byte("csecret"),
					"attribute.token_url":     []byte("turl"),
				}}),
				newUserAssignerFn: newAssignerStub(v1alpha1.InvalidXsuaaCredentials),
			},
			want: want{
				err: v1alpha1.InvalidXsuaaCredentials,
			},
		},
		"NewUserAssignerFn success Upjet": {
			args: args{
				cr:              cr(withCredsUpjet(), withUser("someUser")),
				track:           newTracker(nil),
				resourcetracker: newResourceTracker(nil),
				kube: kubeStubUpjet(nil, &corev1.Secret{Data: map[string][]byte{
					"attribute.api_url":       []byte("aurl"),
					"attribute.client_id":     []byte("cid"),
					"attribute.client_secret": []byte("csecret"),
					"attribute.token_url":     []byte("turl"),
				}}),
				newUserAssignerFn: newAssignerStub(nil),
			},
			want: want{
				err:             nil,
				externalCreated: true,
			},
		},
		"NewGroupAssignerFn err Upjet": {
			args: args{
				cr:              cr(withCredsUpjet(), withGroup("someGroup")),
				track:           newTracker(nil),
				resourcetracker: newResourceTracker(nil),
				kube: kubeStubUpjet(nil, &corev1.Secret{Data: map[string][]byte{
					"attribute.api_url":       []byte("aurl"),
					"attribute.client_id":     []byte("cid"),
					"attribute.client_secret": []byte("csecret"),
					"attribute.token_url":     []byte("turl"),
				}}),
				newGroupAssignerFn: newAssignerStub(v1alpha1.InvalidXsuaaCredentials),
			},
			want: want{
				err: v1alpha1.InvalidXsuaaCredentials,
			},
		},
		"NewGroupAssignerFn success Upjet": {
			args: args{
				cr:              cr(withCredsUpjet(), withGroup("someGroup")),
				track:           newTracker(nil),
				resourcetracker: newResourceTracker(nil),
				kube: kubeStubUpjet(nil, &corev1.Secret{Data: map[string][]byte{
					"attribute.api_url":       []byte("aurl"),
					"attribute.client_id":     []byte("cid"),
					"attribute.client_secret": []byte("csecret"),
					"attribute.token_url":     []byte("turl"),
				}}),
				newGroupAssignerFn: newAssignerStub(nil),
			},
			want: want{
				err:             nil,
				externalCreated: true,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := connector{
				usage:              tc.args.track,
				kube:               tc.args.kube,
				resourcetracker:    tc.args.resourcetracker,
				newUserAssignerFn:  tc.args.newUserAssignerFn,
				newGroupAssignerFn: tc.args.newGroupAssignerFn,
			}
			got, err := c.Connect(context.Background(), tc.args.cr)
			expectedErrorBehaviour(t, tc.want.err, err)
			if tc.want.externalCreated != (got != nil) {
				t.Errorf("expected external to be created: %t, got %t", tc.want.externalCreated, got != nil)
			}
		})
	}
}

func TestConfigureUserAssignerFn(t *testing.T) {
	var tests = map[string]struct {
		binding   *v1alpha1.XsuaaBinding
		expectErr error
	}{
		"NilData": {
			binding:   nil,
			expectErr: errInvalidSecret,
		},
		"ValidCreds": {
			binding: &v1alpha1.XsuaaBinding{
				ApiUrl:       "aurl",
				ClientId:     "cid",
				ClientSecret: "csecret",
				TokenURL:     "turl",
			},
			expectErr: nil,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := configureUserAssignerFn(tc.binding)
			expectedErrorBehaviour(t, tc.expectErr, err)
		})
	}
}
func TestConfigurGroupAssignerFn(t *testing.T) {
	var tests = map[string]struct {
		binding   *v1alpha1.XsuaaBinding
		expectErr error
	}{
		"NilData": {
			binding:   nil,
			expectErr: errInvalidSecret,
		},
		"ValidCreds": {
			binding: &v1alpha1.XsuaaBinding{
				ApiUrl:       "aurl",
				ClientId:     "cid",
				ClientSecret: "csecret",
				TokenURL:     "turl",
			},
			expectErr: nil,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := configureGroupAssignerFn(tc.binding)
			expectedErrorBehaviour(t, tc.expectErr, err)
		})
	}
}

func expectedErrorBehaviour(t *testing.T, expectedErr error, gotErr error) {
	if gotErr != nil {
		assert.Truef(t, errors.Is(gotErr, expectedErr), "expected error %v, got %v", expectedErr, gotErr)
		return
	}
	if expectedErr != nil {
		t.Errorf("expected error %v, got nil", expectedErr.Error())
	}

}

func cr(m ...RoleCollectionModifier) *v1alpha1.RoleCollectionAssignment {
	cr := &v1alpha1.RoleCollectionAssignment{
		Spec:   v1alpha1.RoleCollectionAssignmentSpec{ForProvider: v1alpha1.RoleCollectionAssignmentParameters{}},
		Status: v1alpha1.RoleCollectionAssignmentStatus{},
	}
	for _, f := range m {
		f(cr)
	}
	return cr
}

func withCredsCustom() RoleCollectionModifier {
	return func(assignment *v1alpha1.RoleCollectionAssignment) {
		assignment.Spec.APICredentials = v1alpha1.APICredentials{
			Source: xpv1.CredentialsSourceSecret,
			CommonCredentialSelectors: xpv1.CommonCredentialSelectors{
				SecretRef: &xpv1.SecretKeySelector{
					Key: "credentials",
					SecretReference: xpv1.SecretReference{
						Namespace: "default",
						Name:      "xsuaa-secret",
					},
				},
			},
		}
	}
}

func withCredsUpjet() RoleCollectionModifier {
	return func(roleCollection *v1alpha1.RoleCollectionAssignment) {
		roleCollection.Spec.SubaccountApiCredentialRef = &xpv1.Reference{
			Name: "api-credential-ref"}
		roleCollection.Spec.SubaccountApiCredentialSecret = "xsuaa-secret"
		roleCollection.Spec.SubaccountApiCredentialSecretNamespace = "default"
	}
}

func withUser(username string) RoleCollectionModifier {
	return func(assignment *v1alpha1.RoleCollectionAssignment) {
		assignment.Spec.ForProvider = v1alpha1.RoleCollectionAssignmentParameters{
			UserName: username,
		}
	}
}

func withGroup(groupname string) RoleCollectionModifier {
	return func(assignment *v1alpha1.RoleCollectionAssignment) {
		assignment.Spec.ForProvider = v1alpha1.RoleCollectionAssignmentParameters{
			GroupName: groupname,
		}
	}
}

func newTracker(err error) resource.Tracker {
	return &tracker{err: err}
}

func newResourceTracker(c client.Client) tracking.ReferenceResolverTracker {
	return &ReferenceResolverTrackerMock{}

}

func WithConditions(c ...xpv1.Condition) RoleCollectionModifier {
	return func(r *v1alpha1.RoleCollectionAssignment) { r.Status.ConditionedStatus.Conditions = c }
}

type tracker struct {
	err error
}

func (t *tracker) Track(ctx context.Context, mg resource.Managed) error {
	return t.err
}

func newAssignerStub(err error) func(_ *v1alpha1.XsuaaBinding) (RoleAssigner, error) {
	return func(_ *v1alpha1.XsuaaBinding) (RoleAssigner, error) {
		if err != nil {
			return nil, err
		}
		return &RoleAssignerMock{}, nil
	}
}

type RoleCollectionModifier func(dirEnvironment *v1alpha1.RoleCollectionAssignment)
