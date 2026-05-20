package rolecollection

import (
	"context"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	base "github.com/sap/crossplane-provider-btp/apis/base/security/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	legacyv1alpha1 "github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
)

var apiError = errors.New("apiError")

// RoleMaintainerMock implements RoleCollectionMaintainer for testing.
type RoleMaintainerMock struct {
	generateObservation base.BaseRoleCollectionObservation
	needsCreation       bool
	needsUpdate         bool
	err                 error
	CalledIdentifier    string
}

var _ RoleCollectionMaintainer = &RoleMaintainerMock{}

func (r *RoleMaintainerMock) GenerateObservation(_ context.Context, roleCollectionName string) (base.BaseRoleCollectionObservation, error) {
	r.CalledIdentifier = roleCollectionName
	return r.generateObservation, r.err
}

func (r *RoleMaintainerMock) NeedsCreation(observation base.BaseRoleCollectionObservation) bool {
	return r.needsCreation
}

func (r *RoleMaintainerMock) NeedsUpdate(_ base.BaseRoleCollectionParameters, _ base.BaseRoleCollectionObservation) bool {
	return r.needsUpdate
}

func (r *RoleMaintainerMock) Create(_ context.Context, _ base.BaseRoleCollectionParameters) (string, error) {
	return r.CalledIdentifier, r.err
}

func (r *RoleMaintainerMock) Update(_ context.Context, roleCollectionName string, _ base.BaseRoleCollectionParameters, _ base.BaseRoleCollectionObservation) error {
	r.CalledIdentifier = roleCollectionName
	return r.err
}

func (r *RoleMaintainerMock) Delete(_ context.Context, roleCollectionName string) error {
	r.CalledIdentifier = roleCollectionName
	return r.err
}

// withMockMaintainer replaces the global configureRoleCollectionMaintainerFn for tests.
func withMockMaintainer(mock *RoleMaintainerMock) func() {
	original := configureRoleCollectionMaintainerFn
	configureRoleCollectionMaintainerFn = func(_ context.Context, _ *legacyv1alpha1.XsuaaBinding) (RoleCollectionMaintainer, error) {
		return mock, nil
	}
	return func() { configureRoleCollectionMaintainerFn = original }
}

// testClient returns a Client with a mock kube that always succeeds credential resolution.
func testClient() Client {
	return Client{
		kube: &test.MockClient{
			MockGet: test.NewMockGetFn(nil, func(obj client.Object) error {
				secret := obj.(*corev1.Secret)
				secret.Data = map[string][]byte{
					"credentials": []byte(`{"clientid":"id","clientsecret":"secret","tokenurl":"http://token","apiurl":"http://api"}`),
				}
				return nil
			}),
		},
	}
}

func TestObserve(t *testing.T) {
	type args struct {
		cr     *base.BaseRoleCollection
		client *RoleMaintainerMock
	}

	type want struct {
		cr               *base.BaseRoleCollection
		o                managed.ExternalObservation
		err              error
		CalledIdentifier string
	}

	generatedObservation := base.BaseRoleCollectionObservation{
		Name: internal.Ptr("generated"),
	}

	cases := map[string]struct {
		args args
		want want
	}{
		"EmptyExternalName": {
			args: args{
				cr:     cr("spec-subaccount-admin-co", WithExternalName("")),
				client: &RoleMaintainerMock{},
			},
			want: want{
				cr:               cr("spec-subaccount-admin-co", WithExternalName("")),
				o:                managed.ExternalObservation{ResourceExists: false},
				err:              nil,
				CalledIdentifier: "",
			},
		},
		"ValidNameDoesNotExist404": {
			args: args{
				cr: cr("spec-subaccount-admin-co", WithExternalName("ext-subaccount-admin-co")),
				client: &RoleMaintainerMock{
					err:                 nil,
					needsCreation:       true,
					generateObservation: base.BaseRoleCollectionObservation{Name: nil},
				},
			},
			want: want{
				cr:               cr("spec-subaccount-admin-co", WithExternalName("ext-subaccount-admin-co"), WithObservation(base.BaseRoleCollectionObservation{Name: nil})),
				o:                managed.ExternalObservation{ResourceExists: false},
				err:              nil,
				CalledIdentifier: "ext-subaccount-admin-co",
			},
		},
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
			restore := withMockMaintainer(tc.args.client)
			defer restore()

			c := testClient()
			got, err := Observe(c, context.Background(), nil, tc.args.cr)
			expectedErrorBehaviour(t, tc.want.err, err)
			if diff := cmp.Diff(tc.want.CalledIdentifier, tc.args.client.CalledIdentifier); diff != "" {
				t.Errorf("\n%s\nObserve(...): -want, +CalledIdentifier:\n", diff)
			}
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\n%s\nObserve(...): -want, +got:\n", diff)
			}
			if diff := cmp.Diff(tc.want.cr, tc.args.cr); diff != "" {
				t.Errorf("\nObserve(): expected cr after operation -want, +got:\n%s\n", diff)
			}
		})
	}
}

func TestCreate(t *testing.T) {
	type args struct {
		cr     *base.BaseRoleCollection
		client *RoleMaintainerMock
	}

	type want struct {
		o   managed.ExternalCreation
		cr  *base.BaseRoleCollection
		err error
	}

	alreadyExistsErr := errors.New("resource already exists")

	cases := map[string]struct {
		args args
		want want
	}{
		"SuccessfulCreationSetsExternalName": {
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
		"ResourceAlreadyExistsErrorDoesNotSetExternalName": {
			args: args{
				cr: cr("subaccount-admin-co"),
				client: &RoleMaintainerMock{
					err: alreadyExistsErr,
				},
			},
			want: want{
				cr:  cr("subaccount-admin-co", WithConditions(xpv1.Creating())),
				o:   managed.ExternalCreation{},
				err: alreadyExistsErr,
			},
		},
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
			restore := withMockMaintainer(tc.args.client)
			defer restore()

			c := testClient()
			got, err := Create(c, context.Background(), nil, tc.args.cr)

			expectedErrorBehaviour(t, tc.want.err, err)
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\n%s\nCreate(...): -want, +got:\n", diff)
			}
			if diff := cmp.Diff(tc.want.cr, tc.args.cr); diff != "" {
				t.Errorf("\nCreate(): expected cr after operation -want, +got:\n%s\n", diff)
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	type args struct {
		cr     *base.BaseRoleCollection
		client *RoleMaintainerMock
	}

	type want struct {
		o                managed.ExternalUpdate
		cr               *base.BaseRoleCollection
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
		"update successful": {
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
			restore := withMockMaintainer(tc.args.client)
			defer restore()

			c := testClient()
			got, err := Update(c, context.Background(), nil, tc.args.cr)

			expectedErrorBehaviour(t, tc.want.err, err)
			if diff := cmp.Diff(tc.want.CalledIdentifier, tc.args.client.CalledIdentifier); diff != "" {
				t.Errorf("\n%s\nUpdate(...): -want, +CalledIdentifier:\n", diff)
			}
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\n%s\nUpdate(...): -want, +got:\n", diff)
			}
			if diff := cmp.Diff(tc.want.cr, tc.args.cr); diff != "" {
				t.Errorf("\nUpdate(): expected cr after operation -want, +got:\n%s\n", diff)
			}
		})
	}
}

func TestDelete(t *testing.T) {
	type args struct {
		cr     *base.BaseRoleCollection
		client *RoleMaintainerMock
	}

	type want struct {
		cr               *base.BaseRoleCollection
		err              error
		CalledIdentifier string
	}

	cases := map[string]struct {
		args args
		want want
	}{
		"DeletionWithValidExternalName": {
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
		"404ResponseNotTreatedAsError": {
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
			restore := withMockMaintainer(tc.args.client)
			defer restore()

			c := testClient()
			_, err := Delete(c, context.Background(), nil, tc.args.cr)
			if diff := cmp.Diff(tc.want.CalledIdentifier, tc.args.client.CalledIdentifier); diff != "" {
				t.Errorf("\n%s\nDelete(...): -want, +CalledIdentifier:\n", diff)
			}
			expectedErrorBehaviour(t, tc.want.err, err)
			if diff := cmp.Diff(tc.want.cr, tc.args.cr); diff != "" {
				t.Errorf("\nDelete(): expected cr after operation -want, +got:\n%s\n", diff)
			}
		})
	}
}

func TestConnect(t *testing.T) {
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
		cr           *base.BaseRoleCollection
		kube         client.Client
		newServiceFn func(ctx context.Context, binding *legacyv1alpha1.XsuaaBinding) (RoleCollectionMaintainer, error)
	}

	type want struct {
		err error
	}

	cases := map[string]struct {
		args args
		want want
	}{
		"Not found secret Upjet": {
			args: args{
				cr:   cr("test-collection", withCredsUpjet(), WithExternalName("test-collection")),
				kube: kubeStubUpjet(legacyv1alpha1.FailedToGetSecret, nil),
			},
			want: want{
				err: legacyv1alpha1.FailedToGetSecret,
			},
		},
		"Secret without key": {
			args: args{
				cr:   cr("test-collection", withCredsCustom(), WithExternalName("test-collection")),
				kube: kubeStubCustom(nil, nil),
			},
			want: want{
				err: legacyv1alpha1.InvalidXsuaaCredentials,
			},
		},
		"Not found secret Custom": {
			args: args{
				cr:   cr("test-collection", withCredsCustom(), WithExternalName("test-collection")),
				kube: kubeStubCustom(legacyv1alpha1.FailedToGetSecret, nil),
			},
			want: want{
				err: legacyv1alpha1.FailedToGetSecret,
			},
		},
		"NewServiceFn err Custom": {
			args: args{
				cr: cr("test-collection", withCredsCustom(), WithExternalName("test-collection")),
				kube: kubeStubCustom(nil, map[string][]byte{
					"credentials": []byte(`{"clientid": "clientid", "clientsecret": "clientsecret", "tokenurl": "tokenurl", "apiurl": "apiurl"}`),
				}),
				newServiceFn: newMaintainerStub(legacyv1alpha1.InvalidXsuaaCredentials),
			},
			want: want{
				err: legacyv1alpha1.InvalidXsuaaCredentials,
			},
		},
		"NewServiceFn success Custom": {
			args: args{
				cr: cr("test-collection", withCredsCustom(), WithExternalName("test-collection")),
				kube: kubeStubCustom(nil, map[string][]byte{
					"credentials": []byte(`{"clientid": "clientid", "clientsecret": "clientsecret", "tokenurl": "tokenurl", "apiurl": "apiurl"}`),
				}),
				newServiceFn: newMaintainerStub(nil),
			},
			want: want{
				err: nil,
			},
		},
		"NewServiceFn err Upjet": {
			args: args{
				cr: cr("test-collection", withCredsUpjet(), WithExternalName("test-collection")),
				kube: kubeStubUpjet(nil, &corev1.Secret{Data: map[string][]byte{
					"attribute.api_url":       []byte("aurl"),
					"attribute.client_id":     []byte("cid"),
					"attribute.client_secret": []byte("csecret"),
					"attribute.token_url":     []byte("turl"),
				}}),
				newServiceFn: newMaintainerStub(legacyv1alpha1.InvalidXsuaaCredentials),
			},
			want: want{
				err: legacyv1alpha1.InvalidXsuaaCredentials,
			},
		},
		"NewServiceFn success Upjet": {
			args: args{
				cr: cr("test-collection", withCredsUpjet(), WithExternalName("test-collection")),
				kube: kubeStubUpjet(nil, &corev1.Secret{Data: map[string][]byte{
					"attribute.api_url":       []byte("aurl"),
					"attribute.client_id":     []byte("cid"),
					"attribute.client_secret": []byte("csecret"),
					"attribute.token_url":     []byte("turl"),
				}}),
				newServiceFn: newMaintainerStub(nil),
			},
			want: want{
				err: nil,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if tc.args.newServiceFn != nil {
				original := configureRoleCollectionMaintainerFn
				configureRoleCollectionMaintainerFn = tc.args.newServiceFn
				defer func() { configureRoleCollectionMaintainerFn = original }()
			}

			c := Client{kube: tc.args.kube}
			_, err := Observe(c, context.Background(), nil, tc.args.cr)
			expectedErrorBehaviour(t, tc.want.err, err)
		})
	}
}

func TestReadCustomSecret(t *testing.T) {
	tests := map[string]struct {
		json      string
		expectErr error
	}{
		"InvalidFormat": {
			json:      `"some invalid json"}`,
			expectErr: legacyv1alpha1.InvalidXsuaaCredentials,
		},
		"MissingRequiredCreds": {
			json:      `{"clientid": "clientid", "tokenurl": "tokenurl", "apiurl": "apiurl"}`,
			expectErr: legacyv1alpha1.InvalidXsuaaCredentials,
		},
		"ValidCreds": {
			json:      `{"clientid": "clientid", "clientsecret": "clientsecret", "tokenurl": "tokenurl", "apiurl": "apiurl"}`,
			expectErr: nil,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := legacyv1alpha1.ReadXsuaaCredentialsCustom([]byte(tc.json))
			expectedErrorBehaviour(t, tc.expectErr, err)
		})
	}
}

func TestReadXsuaaCredentialsUpjet(t *testing.T) {
	tests := map[string]struct {
		creds     corev1.Secret
		expect    *legacyv1alpha1.XsuaaBinding
		expectErr error
	}{
		"NilData": {
			creds:     corev1.Secret{Data: nil},
			expect:    nil,
			expectErr: legacyv1alpha1.InvalidXsuaaCredentials,
		},
		"MissingClientSecret": {
			creds: corev1.Secret{Data: map[string][]byte{
				"attribute.api_url":   []byte("aurl"),
				"attribute.client_id": []byte("cid"),
				"attribute.token_url": []byte("turl"),
			}},
			expect:    nil,
			expectErr: legacyv1alpha1.InvalidXsuaaCredentials,
		},
		"AllFieldsPresent": {
			creds: corev1.Secret{Data: map[string][]byte{
				"attribute.api_url":       []byte("aurl"),
				"attribute.client_id":     []byte("cid"),
				"attribute.client_secret": []byte("csecret"),
				"attribute.token_url":     []byte("turl"),
			}},
			expect: &legacyv1alpha1.XsuaaBinding{
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
			got, err := legacyv1alpha1.ReadXsuaaCredentialsUpjet(tc.creds)
			expectedErrorBehaviour(t, tc.expectErr, err)
			if tc.expectErr == nil {
				assert.Equal(t, tc.expect, got)
			} else {
				assert.Nil(t, got)
			}
		})
	}
}

func expectedErrorBehaviour(t *testing.T, expectedErr error, gotErr error) {
	t.Helper()
	if gotErr != nil {
		assert.Truef(t, errors.Is(gotErr, expectedErr), "expected error %v, got %v", expectedErr, gotErr)
		return
	}
	if expectedErr != nil {
		t.Errorf("expected error %v, got nil", expectedErr.Error())
	}
}

type RoleCollectionModifier func(*base.BaseRoleCollection)

func cr(name string, m ...RoleCollectionModifier) *base.BaseRoleCollection {
	cr := &base.BaseRoleCollection{
		Spec: base.BaseRoleCollectionSpec{
			ForProvider: base.BaseRoleCollectionParameters{
				Name: name,
			},
			XSUAACredentialsReference: base.XSUAACredentialsReference{
				APICredentials: base.APICredentials{
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
				},
			},
		},
		Status: base.BaseRoleCollectionStatus{},
	}
	for _, f := range m {
		f(cr)
	}
	return cr
}

func withCredsCustom() RoleCollectionModifier {
	return func(rc *base.BaseRoleCollection) {
		rc.Spec.APICredentials = base.APICredentials{
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
	return func(rc *base.BaseRoleCollection) {
		rc.Spec.APICredentials = base.APICredentials{}
		rc.Spec.SubaccountApiCredentialRef = &xpv1.Reference{Name: "api-credential-ref"}
		rc.Spec.SubaccountApiCredentialSecret = "xsuaa-secret"
		rc.Spec.SubaccountApiCredentialSecretNamespace = "default"
	}
}

func WithConditions(c ...xpv1.Condition) RoleCollectionModifier {
	return func(r *base.BaseRoleCollection) { r.Status.Conditions = c }
}

func WithExternalName(externalName string) RoleCollectionModifier {
	return func(r *base.BaseRoleCollection) { meta.SetExternalName(r, externalName) }
}

func WithObservation(o base.BaseRoleCollectionObservation) RoleCollectionModifier {
	return func(r *base.BaseRoleCollection) { r.Status.AtProvider = o }
}

func newMaintainerStub(err error) func(context.Context, *legacyv1alpha1.XsuaaBinding) (RoleCollectionMaintainer, error) {
	return func(_ context.Context, _ *legacyv1alpha1.XsuaaBinding) (RoleCollectionMaintainer, error) {
		if err != nil {
			return nil, err
		}
		return &RoleMaintainerMock{}, nil
	}
}
