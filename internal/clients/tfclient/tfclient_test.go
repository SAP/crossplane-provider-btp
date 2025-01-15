package tfclient

import (
	"context"
	"testing"

	v1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type MockManaged struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              v1.ResourceSpec
	Status            v1.ResourceStatus
}

func (m *MockManaged) GetCondition(ct v1.ConditionType) v1.Condition {
	return m.Status.GetCondition(ct)
}

func (m *MockManaged) SetConditions(c ...v1.Condition) {
	m.Status.SetConditions(c...)
}

func (m *MockManaged) GetProviderConfigReference() *v1.Reference {
	return m.Spec.ProviderConfigReference
}

func (m *MockManaged) SetProviderConfigReference(r *v1.Reference) {
	m.Spec.ProviderConfigReference = r
}

func (m *MockManaged) GetWriteConnectionSecretToReference() *v1.SecretReference {
	return m.Spec.WriteConnectionSecretToReference
}

func (m *MockManaged) SetWriteConnectionSecretToReference(r *v1.SecretReference) {
	m.Spec.WriteConnectionSecretToReference = r
}

func (m *MockManaged) GetDeletionPolicy() v1.DeletionPolicy {
	return m.Spec.DeletionPolicy
}

func (m *MockManaged) SetDeletionPolicy(p v1.DeletionPolicy) {
	m.Spec.DeletionPolicy = p
}

func (m *MockManaged) GetManagementPolicies() v1.ManagementPolicies {
	return m.Spec.ManagementPolicies
}

func (m *MockManaged) SetManagementPolicies(p v1.ManagementPolicies) {
	m.Spec.ManagementPolicies = p
}

func (m *MockManaged) GetConditionedStatus() *v1.ConditionedStatus {
	return &m.Status.ConditionedStatus
}

func (m *MockManaged) GetObjectKind() schema.ObjectKind {
	return &m.TypeMeta
}
func (m *MockManaged) GetPublishConnectionDetailsTo() *v1.PublishConnectionDetailsTo {
	return m.Spec.PublishConnectionDetailsTo
}
func (m *MockManaged) SetPublishConnectionDetailsTo(r *v1.PublishConnectionDetailsTo) {
	m.Spec.PublishConnectionDetailsTo = r
}

func (m *MockManaged) DeepCopyObject() runtime.Object {
	return &MockManaged{
		TypeMeta:   m.TypeMeta,
		ObjectMeta: *m.ObjectMeta.DeepCopy(),
		Spec:       m.Spec,
		Status:     m.Status,
	}
}

func TestTerraformSetupBuilder(t *testing.T) {

	kubeStub := func(err error, secretData []byte) client.Client {
		return &test.MockClient{
			MockGet: test.NewMockGetFn(err, func(obj client.Object) error {
				if secret, ok := obj.(*corev1.Secret); ok {
					secret.Data = map[string][]byte{"service-account.json": secretData}
				}
				return nil
			}),
		}
	}

	type args struct {
		version         string
		providerSource  string
		providerVersion string
		disableTracking bool
	}
	type want struct {
		err           error
		setupCreated  bool
		username      string
		password      string
		globalAccount string
		cliServerUrl  string
	}

	cases := map[string]struct {
		args           args
		want           want
		mockSecretData []byte
		mockErr        error
	}{
		"connect tfclient with tracking": {
			args: args{
				version:         "version",
				providerSource:  "source",
				providerVersion: "someVersion",
				disableTracking: false,
			},
			want: want{
				err:           nil,
				setupCreated:  true,
				username:      "testUser",
				password:      "testPassword",
				globalAccount: "testAccount",
				cliServerUrl:  "<https://cli.server.url>",
			},
			mockSecretData: []byte(`{"username":"testUser","password":"testPassword"}`),
		},
		"connect tfclient without tracking": {
			args: args{
				version:         "version",
				providerSource:  "source",
				providerVersion: "someVersion",
				disableTracking: true,
			},
			want: want{
				err:           nil,
				setupCreated:  true,
				username:      "testUser",
				password:      "testPassword",
				globalAccount: "testAccount",
				cliServerUrl:  "<https://cli.server.url>",
			},
			mockSecretData: []byte(`{"username":"testUser","password":"testPassword"}`),
		},
		"failed to resolve provider config reference": {
			args: args{
				version:         "version",
				providerSource:  "source",
				providerVersion: "someVersion",
				disableTracking: false,
			},
			want: want{
				err:           errors.New(errGetProviderConfig),
				setupCreated:  false,
				username:      "testUser",
				password:      "testPassword",
				globalAccount: "testAccount",
				cliServerUrl:  "<https://cli.server.url>",
			},
			mockErr: errors.New(errGetProviderConfig),
		},
		"connect tfclient with tracking and valid secret data": {
			args: args{
				version:         "version",
				providerSource:  "source",
				providerVersion: "someVersion",
				disableTracking: false,
			},
			want: want{
				err:           nil,
				setupCreated:  true,
				username:      "testUser",
				password:      "testPassword",
				globalAccount: "testAccount",
				cliServerUrl:  "<https://cli.server.url>",
			},
			mockSecretData: []byte(`{"username":"testUser","password":"testPassword"}`),
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {

			mockClient := kubeStub(tc.mockErr, tc.mockSecretData)

			// TO DO: mg := managed resource for testing
			mg := &MockManaged{
				TypeMeta: metav1.TypeMeta{
					Kind:       "MockManaged",
					APIVersion: "example.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-managed-res",
				},
				Spec: v1.ResourceSpec{
					ProviderConfigReference: &v1.Reference{Name: "provider-config"},
				},
			}

			tfSetup := TerraformSetupBuilder(tc.args.version, tc.args.providerSource, tc.args.providerVersion, tc.args.disableTracking)
			ctx := context.Background()

			if tc.want.setupCreated != (tfSetup != nil) {
				t.Errorf("expected terraform setup to be created: %t, tfSetup %t", tc.want.setupCreated, tfSetup != nil)
			}

			setup, err := tfSetup(ctx, mockClient, mg)

			if tc.want.setupCreated {
				if setup.Configuration == nil {
					t.Errorf("expected setup to be created with configuration, but got nil")
				} else {
					if username, ok := setup.Configuration["username"]; !ok || username != tc.want.username {
						t.Errorf("expected username: %v, got: %v", tc.want.username, username)
					}
					if password, ok := setup.Configuration["password"]; !ok || password != tc.want.password {
						t.Errorf("expected password: %v, got: %v", tc.want.password, password)
					}
					if globalAccount, ok := setup.Configuration["globalaccount"]; !ok || globalAccount != tc.want.globalAccount {
						t.Errorf("expected globalaccount: %v, got: %v", tc.want.globalAccount, globalAccount)
					}
					if cliServerUrl, ok := setup.Configuration["cli_server_url"]; !ok || cliServerUrl != tc.want.cliServerUrl {
						t.Errorf("expected cli_server_url: %v, got: %v", tc.want.cliServerUrl, cliServerUrl)
					}
				}
			}

			if tc.want.err != nil {
				if err == nil || !errors.Is(err, tc.want.err) {
					t.Errorf("expected error: %v, tfSetup Error: %v", tc.want.err, err)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

		})
	}
}
