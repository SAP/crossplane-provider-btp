package rolecollection

import (
	"context"
	"github.com/sap/crossplane-provider-btp/internal"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	apiError = errors.New("apiError")
)

func TestObserve(t *testing.T) {
	type args struct {
		cr     *v1alpha1.RoleCollection
		client *RoleMaintainerMock
	}

	type want struct {
		cr  *v1alpha1.RoleCollection
		o   managed.ExternalObservation
		err error

		CalledIdentifier string
	}

	generatedObservation := v1alpha1.RoleCollectionObservation{
		Name: internal.Ptr("generated"),
	}

	cases := map[string]struct {
		args args
		want want
	}{
		"LookupError": {
			args: args{
				cr: cr("spec-subaccount-admin-co", WithExternalName("ext-subaccount-admin-co")),
				client: &RoleMaintainerMock{
					err: apiError,
				},
			},
			want: want{
				cr:               cr("spec-subaccount-admin-co", WithExternalName("ext-subaccount-admin-co")),
				o:                managed.ExternalObservation{},
				err:              apiError,
				CalledIdentifier: "ext-subaccount-admin-co",
			},
		},
		"needs creation": {
			args: args{
				cr: cr("spec-subaccount-admin-co", WithExternalName("ext-subaccount-admin-co")),
				client: &RoleMaintainerMock{
					err:                 nil,
					needsCreation:       true,
					generateObservation: generatedObservation,
				},
			},
			want: want{
				cr:               cr("spec-subaccount-admin-co", WithExternalName("ext-subaccount-admin-co"), WithObservation(generatedObservation)),
				o:                managed.ExternalObservation{ResourceExists: false},
				err:              nil,
				CalledIdentifier: "ext-subaccount-admin-co",
			},
		},
		"needs update": {
			args: args{
				cr: cr("spec-subaccount-admin-co", WithExternalName("ext-subaccount-admin-co")),
				client: &RoleMaintainerMock{
					needsUpdate:         true,
					generateObservation: generatedObservation,
				},
			},
			want: want{
				cr: cr("spec-subaccount-admin-co", WithExternalName("ext-subaccount-admin-co"), WithObservation(generatedObservation), WithConditions(xpv1.Available())),
				o: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: false,
				},
				CalledIdentifier: "ext-subaccount-admin-co",
			},
		},
		"available": {
			args: args{
				cr: cr("spec-subaccount-admin-co", WithExternalName("ext-subaccount-admin-co")),
				client: &RoleMaintainerMock{
					err:                 nil,
					generateObservation: generatedObservation,
				},
			},
			want: want{
				cr: cr("spec-subaccount-admin-co", WithExternalName("ext-subaccount-admin-co"), WithObservation(generatedObservation), WithConditions(xpv1.Available())),
				o: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  true,
					ConnectionDetails: managed.ConnectionDetails{},
				},
				CalledIdentifier: "ext-subaccount-admin-co",
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{client: tc.args.client}
			got, err := e.Observe(context.Background(), tc.args.cr)
			internal.VerifyTestError(t, tc.want.err, err)
			if diff := cmp.Diff(tc.want.CalledIdentifier, tc.args.client.CalledIdentifier); diff != "" {
				t.Errorf("\n%s\ne.Observe(...): -want, +CalledIdentifier:\n", diff)
			}
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
		cr     *v1alpha1.RoleCollection
		client *RoleMaintainerMock
	}

	type want struct {
		o   managed.ExternalCreation
		cr  *v1alpha1.RoleCollection
		err error
	}

	cases := map[string]struct {
		args args
		want want
	}{
		"api error": {
			args: args{
				cr: cr("subaccount-admin-co"),
				client: &RoleMaintainerMock{
					err: apiError,
				},
			},
			want: want{
				cr:  cr("subaccount-admin-co", WithConditions(xpv1.Creating())),
				o:   managed.ExternalCreation{},
				err: apiError,
			},
		},
		"create successful": {
			args: args{
				cr: cr("subaccount-admin-co"),
				client: &RoleMaintainerMock{
					err:              nil,
					CalledIdentifier: "subaccount-admin-co",
				},
			},
			want: want{
				cr: cr("subaccount-admin-co", WithExternalName("subaccount-admin-co"), WithConditions(xpv1.Creating())),
				o: managed.ExternalCreation{
					ConnectionDetails: managed.ConnectionDetails{},
				},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{client: tc.args.client}
			got, err := e.Create(context.Background(), tc.args.cr)

			internal.VerifyTestError(t, tc.want.err, err)
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
		cr     *v1alpha1.RoleCollection
		client *RoleMaintainerMock
	}

	type want struct {
		o                managed.ExternalUpdate
		cr               *v1alpha1.RoleCollection
		err              error
		CalledIdentifier string
	}

	cases := map[string]struct {
		args args
		want want
	}{
		"api error": {
			args: args{
				cr: cr("subaccount-admin-co", WithExternalName("ext-subaccount-admin-co")),
				client: &RoleMaintainerMock{
					err: apiError,
				},
			},
			want: want{
				cr:               cr("subaccount-admin-co", WithExternalName("ext-subaccount-admin-co")),
				o:                managed.ExternalUpdate{},
				err:              apiError,
				CalledIdentifier: "ext-subaccount-admin-co",
			},
		},
		"create successful": {
			args: args{
				cr: cr("subaccount-admin-co", WithExternalName("ext-subaccount-admin-co")),
				client: &RoleMaintainerMock{
					err: nil,
				},
			},
			want: want{
				cr: cr("subaccount-admin-co", WithExternalName("ext-subaccount-admin-co")),
				o: managed.ExternalUpdate{
					ConnectionDetails: managed.ConnectionDetails{},
				},
				CalledIdentifier: "ext-subaccount-admin-co",
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{client: tc.args.client}
			got, err := e.Update(context.Background(), tc.args.cr)

			internal.VerifyTestError(t, tc.want.err, err)
			if diff := cmp.Diff(tc.want.CalledIdentifier, tc.args.client.CalledIdentifier); diff != "" {
				t.Errorf("\n%s\ne.Observe(...): -want, +CalledIdentifier:\n", diff)
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
		cr     *v1alpha1.RoleCollection
		client *RoleMaintainerMock
	}

	type want struct {
		cr               *v1alpha1.RoleCollection
		err              error
		CalledIdentifier string
	}

	cases := map[string]struct {
		args args
		want want
	}{
		"api error": {
			args: args{
				cr: cr("subaccount-admin-co", WithExternalName("ext-subaccount-admin-co")),
				client: &RoleMaintainerMock{
					err: apiError,
				},
			},
			want: want{
				cr:               cr("subaccount-admin-co", WithExternalName("ext-subaccount-admin-co"), WithConditions(xpv1.Deleting())),
				err:              apiError,
				CalledIdentifier: "ext-subaccount-admin-co",
			},
		},
		"successfully deleted": {
			args: args{
				cr: cr("subaccount-admin-co", WithExternalName("ext-subaccount-admin-co")),
				client: &RoleMaintainerMock{
					err: nil,
				},
			},
			want: want{
				cr:               cr("subaccount-admin-co", WithExternalName("ext-subaccount-admin-co"), WithConditions(xpv1.Deleting())),
				CalledIdentifier: "ext-subaccount-admin-co",
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{client: tc.args.client}
			err := e.Delete(context.Background(), tc.args.cr)
			if diff := cmp.Diff(tc.want.CalledIdentifier, tc.args.client.CalledIdentifier); diff != "" {
				t.Errorf("\n%s\ne.Observe(...): -want, +CalledIdentifier:\n", diff)
			}
			internal.VerifyTestError(t, tc.want.err, err)
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
		cr           *v1alpha1.RoleCollection
		track        resource.Tracker
		kube         client.Client
		newServiceFn func(_ []byte) (RoleCollectionMaintainer, error)
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
				cr:    cr("test-collection"),
				track: newTracker(trackErr),
			},
			want: want{
				err: trackErr,
			},
		},
		// existance of secret ref is enforced on schema level and needs to be verified in e2e tests
		"Not found secret": {
			args: args{
				cr:    cr("test-collection", withCreds()),
				track: newTracker(nil),
				kube:  kubeStub(noSecretErr, nil),
			},
			want: want{
				err: noSecretErr,
			},
		},
		"Secret without key": {
			args: args{
				cr:    cr("test-collection", withCreds()),
				track: newTracker(nil),
				kube:  kubeStub(nil, nil),
			},
			want: want{
				err: errInvalidSecret,
			},
		},
		"NewServiceFn err": {
			args: args{
				cr:           cr("test-collection", withCreds()),
				track:        newTracker(nil),
				kube:         kubeStub(nil, map[string][]byte{cr("test-collection", withCreds()).Spec.APICredentials.SecretRef.Key: []byte("secret")}),
				newServiceFn: newMaintainerStub(createServiceErr),
			},
			want: want{
				err: createServiceErr,
			},
		},
		"NewServiceFn success": {
			args: args{
				cr:           cr("test-collection", withCreds()),
				track:        newTracker(nil),
				kube:         kubeStub(nil, map[string][]byte{cr("test-collection", withCreds()).Spec.APICredentials.SecretRef.Key: []byte("secret")}),
				newServiceFn: newMaintainerStub(nil),
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
				usage:        tc.args.track,
				kube:         tc.args.kube,
				newServiceFn: tc.args.newServiceFn,
			}
			got, err := c.Connect(context.Background(), tc.args.cr)
			internal.VerifyTestError(t, tc.want.err, err)
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
			_, err := configureRoleCollectionMaintainerFn([]byte(tc.json))
			internal.VerifyTestError(t, tc.expectErr, err)
		})
	}
}

func cr(name string, m ...RoleCollectionModifier) *v1alpha1.RoleCollection {
	cr := &v1alpha1.RoleCollection{
		Spec: v1alpha1.RoleCollectionSpec{ForProvider: v1alpha1.RoleCollectionParameters{
			Name: name,
		}},
		Status: v1alpha1.RoleCollectionStatus{},
	}
	for _, f := range m {
		f(cr)
	}
	return cr
}

func newTracker(err error) resource.Tracker {
	return &tracker{err: err}
}

func withCreds() RoleCollectionModifier {
	return func(roleCollection *v1alpha1.RoleCollection) {
		roleCollection.Spec.APICredentials = v1alpha1.APICredentials{
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

func WithConditions(c ...xpv1.Condition) RoleCollectionModifier {
	return func(r *v1alpha1.RoleCollection) { r.Status.ConditionedStatus.Conditions = c }
}

func WithExternalName(externalName string) RoleCollectionModifier {
	return func(r *v1alpha1.RoleCollection) { meta.SetExternalName(r, externalName) }
}

func WithObservation(o v1alpha1.RoleCollectionObservation) RoleCollectionModifier {
	return func(r *v1alpha1.RoleCollection) { r.Status.AtProvider = o }
}

type tracker struct {
	err error
}

func (t *tracker) Track(ctx context.Context, mg resource.Managed) error {
	return t.err
}

type RoleCollectionModifier func(dirEnvironment *v1alpha1.RoleCollection)

func newMaintainerStub(err error) func(_ []byte) (RoleCollectionMaintainer, error) {
	return func(_ []byte) (RoleCollectionMaintainer, error) {
		if err != nil {
			return nil, err
		}
		return &RoleMaintainerMock{}, nil
	}
}
