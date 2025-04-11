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
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
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
				cr: cr(withCreds(), withUser("someUser")),
				client: &RoleAssignerMock{
					err: apiError,
				},
			},
			want: want{
				cr:               cr(WithConditions(xpv1.Creating()), withCreds(), withUser("someUser")),
				o:                managed.ExternalCreation{},
				err:              apiError,
				CalledIdentifier: "someUser",
			},
		},
		"ApiError GroupAssigner": {
			args: args{
				cr: cr(withGroup("someGroup"), withCreds()),
				client: &RoleAssignerMock{
					err: apiError,
				},
			},
			want: want{
				cr:               cr(WithConditions(xpv1.Creating()), withGroup("someGroup"), withCreds()),
				o:                managed.ExternalCreation{},
				err:              apiError,
				CalledIdentifier: "someGroup",
			},
		},
		"Successful UserAssigner": {
			args: args{
				cr: cr(withUser("someUser"), withCreds()),
				client: &RoleAssignerMock{
					hasRole: true,
					err:     nil,
				},
			},
			want: want{
				cr: cr(WithConditions(xpv1.Creating()), withUser("someUser"), withCreds()),
				o: managed.ExternalCreation{
					ConnectionDetails: managed.ConnectionDetails{},
				},
				CalledIdentifier: "someUser",
			},
		},
		"Successful GroupAssigner": {
			args: args{
				cr: cr(withGroup("someGroup"), withCreds()),
				client: &RoleAssignerMock{
					hasRole: true,
					err:     nil,
				},
			},
			want: want{
				cr: cr(WithConditions(xpv1.Creating()), withGroup("someGroup"), withCreds()),
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
			_, err := e.Delete(context.Background(), tc.args.cr)
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
	noSecretErr := errors.New("no secret err")
	createServiceErr := errors.New("error creating service")

	kubeStub := func(err error, secretData map[string][]byte) client.Client {
		return &test.MockClient{
			MockGet: test.NewMockGetFn(err, func(obj client.Object) error {
				secret := obj.(*corev1.Secret)
				secret.Data = secretData
				return nil
			}),
		}
	}

	type args struct {
		cr                 *v1alpha1.RoleCollectionAssignment
		track              resource.Tracker
		kube               client.Client
		newUserAssignerFn  func(_ []byte) (RoleAssigner, error)
		newGroupAssignerFn func(_ []byte) (RoleAssigner, error)
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
				cr:    cr(),
				track: newTracker(trackErr),
			},
			want: want{
				err: trackErr,
			},
		},
		// existance of secret ref is enforced on schema level and needs to be verified in e2e tests
		"Not found secret": {
			args: args{
				cr:    cr(withCreds()),
				track: newTracker(nil),
				kube:  kubeStub(noSecretErr, nil),
			},
			want: want{
				err: noSecretErr,
			},
		},
		"Secret without key": {
			args: args{
				cr:    cr(withCreds()),
				track: newTracker(nil),
				kube:  kubeStub(nil, nil),
			},
			want: want{
				err: errInvalidSecret,
			},
		},
		"NewUserAssignerFn err": {
			args: args{
				cr:                cr(withCreds(), withUser("someUser")),
				track:             newTracker(nil),
				kube:              kubeStub(nil, map[string][]byte{cr(withCreds()).Spec.APICredentials.SecretRef.Key: []byte("secret")}),
				newUserAssignerFn: newAssignerStub(createServiceErr),
			},
			want: want{
				err: createServiceErr,
			},
		},
		"NewUserAssignerFn success": {
			args: args{
				cr:                cr(withCreds(), withUser("someUser")),
				track:             newTracker(nil),
				kube:              kubeStub(nil, map[string][]byte{cr(withCreds()).Spec.APICredentials.SecretRef.Key: []byte("secret")}),
				newUserAssignerFn: newAssignerStub(nil),
			},
			want: want{
				err:             nil,
				externalCreated: true,
			},
		},
		"NewGroupAssignerFn err": {
			args: args{
				cr:                 cr(withCreds(), withGroup("someGroup")),
				track:              newTracker(nil),
				kube:               kubeStub(nil, map[string][]byte{cr(withCreds()).Spec.APICredentials.SecretRef.Key: []byte("secret")}),
				newGroupAssignerFn: newAssignerStub(createServiceErr),
			},
			want: want{
				err: createServiceErr,
			},
		},
		"NewGroupAssignerFn success": {
			args: args{
				cr:                 cr(withCreds(), withGroup("someGroup")),
				track:              newTracker(nil),
				kube:               kubeStub(nil, map[string][]byte{cr(withCreds()).Spec.APICredentials.SecretRef.Key: []byte("secret")}),
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
		json      string
		expectErr error
	}{
		"InvalidFormat": {
			json:      `"some invalid json"}`,
			expectErr: v1alpha1.ErrInvalidXsuaaCredentials,
		},
		"MissingRequiredCreds": {
			json:      `{"clientid": "clientid", "tokenurl": "tokenurl", "apiurl": "apiurl"}`,
			expectErr: v1alpha1.ErrInvalidXsuaaCredentials,
		},
		"ValidCreds": {
			json:      `{"clientid": "clientid", "clientsecret": "clientsecret", "tokenurl": "tokenurl", "apiurl": "apiurl"}`,
			expectErr: nil,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := configureUserAssignerFn([]byte(tc.json))
			expectedErrorBehaviour(t, tc.expectErr, err)
		})
	}
}

func TestConfigureGroupAssignerFn(t *testing.T) {
	var tests = map[string]struct {
		json      string
		expectErr error
	}{
		"InvalidFormat": {
			json:      `"some invalid json"}`,
			expectErr: v1alpha1.ErrInvalidXsuaaCredentials,
		},
		"MissingRequiredCreds": {
			json:      `{"clientid": "clientid", "tokenurl": "tokenurl", "apiurl": "apiurl"}`,
			expectErr: v1alpha1.ErrInvalidXsuaaCredentials,
		},
		"ValidCreds": {
			json:      `{"clientid": "clientid", "clientsecret": "clientsecret", "tokenurl": "tokenurl", "apiurl": "apiurl"}`,
			expectErr: nil,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := configureGroupAssignerFn([]byte(tc.json))
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

func withCreds() RoleCollectionModifier {
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

func WithConditions(c ...xpv1.Condition) RoleCollectionModifier {
	return func(r *v1alpha1.RoleCollectionAssignment) { r.Status.ConditionedStatus.Conditions = c }
}

type tracker struct {
	err error
}

func (t *tracker) Track(ctx context.Context, mg resource.Managed) error {
	return t.err
}

func newAssignerStub(err error) func(_ []byte) (RoleAssigner, error) {
	return func(_ []byte) (RoleAssigner, error) {
		if err != nil {
			return nil, err
		}
		return &RoleAssignerMock{}, nil
	}
}

type RoleCollectionModifier func(dirEnvironment *v1alpha1.RoleCollectionAssignment)
