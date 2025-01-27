package tfclient

import (
	"context"
	"strings"
	"testing"

	v1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/sap/crossplane-provider-btp/internal"
	"github.com/sap/crossplane-provider-btp/internal/testutils"
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

	type args struct {
		version         string
		providerSource  string
		providerVersion string
		disableTracking bool
	}
	type want struct {
		err           *string
		setupCreated  bool
		username      string
		password      string
		globalAccount string
		cliServerUrl  string
	}

	cases := map[string]struct {
		args           args
		want           want
		kubeObjects    []client.Object
		mockSecretData []byte
	}{
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
				globalAccount: "123",
				cliServerUrl:  "<https://cli.server.url>",
			},
			kubeObjects: []client.Object{
				testutils.NewProviderConfigFull("pc-reference", "cis-provider-secret", "sa-provider-secret", "123", "<https://cli.server.url>"),
				testutils.NewSecret("cis-provider-secret", nil),
				testutils.NewSecret("sa-provider-secret", map[string][]byte{"credentials": []byte(`{"username": "testUser", "email": "testUser@sap.com", "password": "testPassword"}`)}),
			},
		},
		"failed to resolve provider config reference without tracking": {
			args: args{
				version:         "version",
				providerSource:  "source",
				providerVersion: "someVersion",
				disableTracking: true,
			},
			want: want{
				err:          internal.Ptr(errGetProviderConfig),
				setupCreated: false,
			},
			kubeObjects: []client.Object{
				testutils.NewProviderConfigFull("", "", "", "", ""),
				testutils.NewSecret("cis-provider-secret", nil),
				testutils.NewSecret("sa-provider-secret", nil),
			},
		},
		"error getting service account credentials without tracking": {
			args: args{
				version:         "version",
				providerSource:  "source",
				providerVersion: "someVersion",
				disableTracking: true,
			},
			want: want{
				err:          internal.Ptr(errGetServiceAccountCreds),
				setupCreated: false,
			},
			kubeObjects: []client.Object{
				testutils.NewProviderConfigFull("pc-reference", "cis-provider-secret", "sa-provider-secret", "123", "<https://cli.server.url>"),
				testutils.NewSecret("cis-provider-secret", nil),
				testutils.NewSecret("", nil),
			},
		},
		"error parsing user credentials without tracking": {
			args: args{
				version:         "version",
				providerSource:  "source",
				providerVersion: "someVersion",
				disableTracking: true,
			},
			want: want{
				err:          internal.Ptr(errCouldNotParseUserCredential),
				setupCreated: false,
			},
			kubeObjects: []client.Object{
				testutils.NewProviderConfigFull("pc-reference", "cis-provider-secret", "sa-provider-secret", "123", "<https://cli.server.url>"),
				testutils.NewSecret("cis-provider-secret", nil),
				testutils.NewSecret("sa-provider-secret", map[string][]byte{"credentials": []byte(`{"username": "test"User", "email": "testUser@sap.com", "password": "testPassword"}`)}),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {

			mockClient := testutils.NewFakeKubeClientBuilder().
				AddResources(tc.kubeObjects...).
				Build()

			mg := &MockManaged{
				TypeMeta: metav1.TypeMeta{
					Kind:       "MockManaged",
					APIVersion: "example.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-managed-res",
				},
				Spec: v1.ResourceSpec{
					ProviderConfigReference: &v1.Reference{Name: "pc-reference"},
				},
			}

			tfSetup := TerraformSetupBuilder(tc.args.version, tc.args.providerSource, tc.args.providerVersion, tc.args.disableTracking)
			ctx := context.Background()

			setup, err := tfSetup(ctx, &mockClient, mg)

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
				if err == nil || !strings.Contains(err.Error(), internal.Val(tc.want.err)) {
					t.Errorf("expected error: %v, tfSetup Error: %v", tc.want.err, err)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

		})
	}
}
