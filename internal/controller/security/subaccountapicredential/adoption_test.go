package subaccountapicredential

import (
	"context"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	securityv1alpha1 "github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
)

func TestHasCompleteCredentialConnectionDetails(t *testing.T) {
	complete := managed.ConnectionDetails{
		"attribute.client_id":     []byte("id"),
		"attribute.client_secret": []byte("secret"),
		"attribute.api_url":       []byte("api"),
		"attribute.token_url":     []byte("token"),
		"extra":                   []byte("allowed"),
	}
	for _, key := range requiredCredentialConnectionKeys {
		t.Run("missing_"+key, func(t *testing.T) {
			got := managed.ConnectionDetails{}
			for k, v := range complete {
				if k != key && k != "extra" {
					got[k] = v
				}
			}
			if hasCompleteCredentialConnectionDetails(got) {
				t.Fatalf("expected missing %s to be incomplete", key)
			}
		})
		t.Run("nil_"+key, func(t *testing.T) {
			got := managed.ConnectionDetails{}
			for k, v := range complete {
				got[k] = v
			}
			got[key] = nil
			if hasCompleteCredentialConnectionDetails(got) {
				t.Fatalf("expected nil %s to be incomplete", key)
			}
		})
		t.Run("empty_"+key, func(t *testing.T) {
			got := managed.ConnectionDetails{}
			for k, v := range complete {
				got[k] = v
			}
			got[key] = []byte{}
			if hasCompleteCredentialConnectionDetails(got) {
				t.Fatalf("expected empty %s to be incomplete", key)
			}
		})
	}
	if !hasCompleteCredentialConnectionDetails(complete) {
		t.Fatal("complete connection details with extra keys should be accepted")
	}
}

func TestCredentialAdoptionExternalClientObserve(t *testing.T) {
	complete := managed.ConnectionDetails{
		"attribute.client_id":     []byte("id"),
		"attribute.client_secret": []byte("secret"),
		"attribute.api_url":       []byte("api"),
		"attribute.token_url":     []byte("token"),
	}
	for name, tc := range map[string]struct {
		cr        *securityv1alpha1.SubaccountApiCredential
		details   managed.ConnectionDetails
		wantErr   string
		wantReady corev1.ConditionStatus
	}{
		"complete secret-based state": {
			cr:      &securityv1alpha1.SubaccountApiCredential{},
			details: complete,
		},
		"incomplete secret-based state": {
			cr:        &securityv1alpha1.SubaccountApiCredential{},
			details:   managed.ConnectionDetails{"attribute.client_id": []byte("id")},
			wantErr:   errCredentialAdoptionIncomplete,
			wantReady: corev1.ConditionFalse,
		},
		"observed certificate from forProvider does not require client secret": {
			cr: &securityv1alpha1.SubaccountApiCredential{
				Spec: securityv1alpha1.SubaccountApiCredentialSpec{
					ForProvider: securityv1alpha1.SubaccountApiCredentialParameters{CertificatePassed: strptr("pem")},
				},
				Status: securityv1alpha1.SubaccountApiCredentialStatus{AtProvider: securityv1alpha1.SubaccountApiCredentialObservation{CredentialType: strptr("client certificate")}},
			},
			details: managed.ConnectionDetails{},
		},
		"observed certificate from initProvider does not require client secret": {
			cr: &securityv1alpha1.SubaccountApiCredential{
				Spec: securityv1alpha1.SubaccountApiCredentialSpec{
					InitProvider: securityv1alpha1.SubaccountApiCredentialInitParameters{CertificatePassed: strptr("pem")},
				},
				Status: securityv1alpha1.SubaccountApiCredentialStatus{AtProvider: securityv1alpha1.SubaccountApiCredentialObservation{CredentialType: strptr("client certificate")}},
			},
			details: managed.ConnectionDetails{},
		},
		"certificate input cannot exempt observed secret state": {
			cr: &securityv1alpha1.SubaccountApiCredential{Spec: securityv1alpha1.SubaccountApiCredentialSpec{
				InitProvider: securityv1alpha1.SubaccountApiCredentialInitParameters{CertificatePassed: strptr("pem")},
			}, Status: securityv1alpha1.SubaccountApiCredentialStatus{AtProvider: securityv1alpha1.SubaccountApiCredentialObservation{CredentialType: strptr("secret")}}},
			details:   managed.ConnectionDetails{"attribute.client_id": []byte("id")},
			wantErr:   errCredentialAdoptionIncomplete,
			wantReady: corev1.ConditionFalse,
		},
		"unknown observed type does not grant certificate exemption": {
			cr: &securityv1alpha1.SubaccountApiCredential{Spec: securityv1alpha1.SubaccountApiCredentialSpec{
				ForProvider: securityv1alpha1.SubaccountApiCredentialParameters{CertificatePassed: strptr("pem")},
			}},
			details:   managed.ConnectionDetails{},
			wantErr:   errCredentialAdoptionIncomplete,
			wantReady: corev1.ConditionFalse,
		},
		"deleting secret credential skips active adoption guard": {
			cr:      &securityv1alpha1.SubaccountApiCredential{ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: func() *metav1.Time { now := metav1.Now(); return &now }()}},
			details: managed.ConnectionDetails{"attribute.client_id": []byte("id")},
		},
	} {
		t.Run(name, func(t *testing.T) {
			inner := managed.ExternalClientFns{
				ObserveFn: func(context.Context, resource.Managed) (managed.ExternalObservation, error) {
					return managed.ExternalObservation{ResourceExists: true, ConnectionDetails: tc.details}, nil
				},
			}
			client := &credentialAdoptionExternalClient{ExternalClient: inner}
			_, err := client.Observe(context.Background(), tc.cr)
			if got := errorString(err); got != tc.wantErr {
				t.Fatalf("error mismatch: got %q, want %q", got, tc.wantErr)
			}
			if tc.wantReady != "" && tc.cr.GetCondition("Ready").Status != tc.wantReady {
				t.Errorf("Ready condition mismatch: got %q, want %q", tc.cr.GetCondition("Ready").Status, tc.wantReady)
			}
		})
	}
}

func TestCredentialAdoptionConnectorPinsIdentityForConnect(t *testing.T) {
	cr := &securityv1alpha1.SubaccountApiCredential{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}},
		Spec:       securityv1alpha1.SubaccountApiCredentialSpec{ForProvider: securityv1alpha1.SubaccountApiCredentialParameters{SubaccountID: strptr("sa-drifted")}},
		Status:     securityv1alpha1.SubaccountApiCredentialStatus{AtProvider: securityv1alpha1.SubaccountApiCredentialObservation{SubaccountID: strptr("sa-established"), Name: strptr("credential-established")}},
	}
	meta.SetExternalName(cr, "sa-established/credential-established")
	var connectedSubaccountID, connectedExternalName string
	inner := managed.ExternalConnectorFn(func(_ context.Context, mg resource.Managed) (managed.ExternalClient, error) {
		got, ok := mg.(*credentialFrameworkState)
		if !ok {
			t.Fatalf("Connect received %T, want credential state wrapper", mg)
		}
		connectedSubaccountID = value(got.Spec.ForProvider.SubaccountID)
		connectedExternalName = meta.GetExternalName(got)
		return managed.ExternalClientFns{}, nil
	})
	wrapped := newCredentialAdoptionConnector(inner, nil)
	if _, err := wrapped.Connect(context.Background(), cr); err != nil {
		t.Fatal(err)
	}
	if connectedSubaccountID != "sa-established" || connectedExternalName != "sa-established/credential-established" {
		t.Fatalf("connector saw wrong identity: subaccount=%q external-name=%q", connectedSubaccountID, connectedExternalName)
	}
	if value(cr.Spec.ForProvider.SubaccountID) != "sa-drifted" || meta.GetExternalName(cr) != "sa-established/credential-established" {
		t.Fatal("temporary identity pin was not restored after Connect")
	}
}

func TestCredentialAdoptionConnectorDoesNotReconstructStateDuringDeletion(t *testing.T) {
	now := metav1.Now()
	cr := &securityv1alpha1.SubaccountApiCredential{ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &now}}
	inner := managed.ExternalConnectorFn(func(_ context.Context, mg resource.Managed) (managed.ExternalClient, error) {
		if mg != cr {
			t.Fatalf("deleting connector resource has type %T; expected original managed resource", mg)
		}
		return managed.ExternalClientFns{}, nil
	})
	if _, err := newCredentialAdoptionConnector(inner, nil).Connect(context.Background(), cr); err != nil {
		t.Fatal(err)
	}
}

func TestCredentialAdoptionExternalClientAllowsAbsentResource(t *testing.T) {
	client := &credentialAdoptionExternalClient{ExternalClient: managed.ExternalClientFns{
		ObserveFn: func(context.Context, resource.Managed) (managed.ExternalObservation, error) {
			return managed.ExternalObservation{ResourceExists: false}, nil
		},
	}}
	obs, err := client.Observe(context.Background(), &securityv1alpha1.SubaccountApiCredential{})
	if err != nil {
		t.Fatalf("Observe() rejected an absent resource before Create: %v", err)
	}
	if obs.ResourceExists {
		t.Fatal("expected resource to remain absent so normal policy-driven Create can run")
	}
}

func TestCredentialAdoptionExternalClientCreateRequiresCompleteSecret(t *testing.T) {
	for name, tc := range map[string]struct {
		cr      *securityv1alpha1.SubaccountApiCredential
		details managed.ConnectionDetails
		wantErr bool
	}{
		"complete": {
			cr: &securityv1alpha1.SubaccountApiCredential{},
			details: managed.ConnectionDetails{
				"attribute.client_id": []byte("id"), "attribute.client_secret": []byte("secret"),
				"attribute.api_url": []byte("api"), "attribute.token_url": []byte("token"),
			},
		},
		"incomplete secret credential": {cr: &securityv1alpha1.SubaccountApiCredential{}, wantErr: true},
		"observed certificate": {
			cr: &securityv1alpha1.SubaccountApiCredential{
				Spec:   securityv1alpha1.SubaccountApiCredentialSpec{InitProvider: securityv1alpha1.SubaccountApiCredentialInitParameters{CertificatePassed: strptr("pem")}},
				Status: securityv1alpha1.SubaccountApiCredentialStatus{AtProvider: securityv1alpha1.SubaccountApiCredentialObservation{CredentialType: strptr("client certificate")}},
			},
		},
		"certificate input with observed secret is incomplete": {
			cr: &securityv1alpha1.SubaccountApiCredential{
				Spec:   securityv1alpha1.SubaccountApiCredentialSpec{InitProvider: securityv1alpha1.SubaccountApiCredentialInitParameters{CertificatePassed: strptr("pem")}},
				Status: securityv1alpha1.SubaccountApiCredentialStatus{AtProvider: securityv1alpha1.SubaccountApiCredentialObservation{CredentialType: strptr("secret")}},
			},
			wantErr: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			client := &credentialAdoptionExternalClient{ExternalClient: managed.ExternalClientFns{
				CreateFn: func(context.Context, resource.Managed) (managed.ExternalCreation, error) {
					return managed.ExternalCreation{ConnectionDetails: tc.details}, nil
				},
			}}
			_, err := client.Create(context.Background(), tc.cr)
			if (err != nil) != tc.wantErr {
				t.Fatalf("Create() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestPinEstablishedIdentity(t *testing.T) {
	for name, tc := range map[string]struct {
		externalName string
		specID       string
		statusID     string
		statusName   string
		wantName     string
		wantID       string
		wantErr      string
	}{
		"spec drift is pinned to observed identity": {
			externalName: "sa-one/credential-one", specID: "sa-two", statusID: "sa-one", statusName: "credential-one",
			wantName: "sa-one/credential-one", wantID: "sa-one",
		},
		"annotation retarget is rejected": {
			externalName: "sa-two/credential-two", specID: "sa-two", statusID: "sa-one", statusName: "credential-one",
			wantName: "sa-two/credential-two", wantID: "sa-two", wantErr: errCredentialIdentityConflict,
		},
		"legacy annotation normalizes to observed identity": {
			externalName: "credential-one", specID: "sa-two", statusID: "sa-one", statusName: "credential-one",
			wantName: "sa-one/credential-one", wantID: "sa-one",
		},
	} {
		t.Run(name, func(t *testing.T) {
			cr := &securityv1alpha1.SubaccountApiCredential{
				ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}},
				Spec:       securityv1alpha1.SubaccountApiCredentialSpec{ForProvider: securityv1alpha1.SubaccountApiCredentialParameters{SubaccountID: strptr(tc.specID)}},
				Status:     securityv1alpha1.SubaccountApiCredentialStatus{AtProvider: securityv1alpha1.SubaccountApiCredentialObservation{SubaccountID: strptr(tc.statusID), Name: strptr(tc.statusName)}},
			}
			meta.SetExternalName(cr, tc.externalName)
			err := pinEstablishedIdentity(cr)
			if got := errorString(err); got != tc.wantErr {
				t.Fatalf("error mismatch: got %q, want %q", got, tc.wantErr)
			}
			if got := meta.GetExternalName(cr); got != tc.wantName {
				t.Errorf("external name mismatch: got %q, want %q", got, tc.wantName)
			}
			if got := *cr.Spec.ForProvider.SubaccountID; got != tc.wantID {
				t.Errorf("subaccount ID mismatch: got %q, want %q", got, tc.wantID)
			}
		})
	}
}

func TestCredentialFrameworkStateReadIdentity(t *testing.T) {
	for _, tc := range []struct {
		name       string
		specID     string
		observedID string
		wantID     string
	}{
		{name: "new MR uses spec identity", specID: "subaccount-new", wantID: "subaccount-new"},
		{name: "observed identity wins over drifted spec", specID: "subaccount-drifted", observedID: "subaccount-established", wantID: "subaccount-established"},
		{name: "partial observed identity is retained", observedID: "subaccount-established", wantID: "subaccount-established"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			if err := corev1.AddToScheme(scheme); err != nil {
				t.Fatal(err)
			}
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "connection", Namespace: "test"},
				Data: map[string][]byte{
					"attribute.client_secret": []byte("synthetic-secret"),
					// Extra keys cannot replace the resource's Read identity.
					"attribute.subaccount_id": []byte("subaccount-from-secret"),
					"attribute.name":          []byte("name-from-secret"),
				},
			}
			cr := &securityv1alpha1.SubaccountApiCredential{
				Spec: securityv1alpha1.SubaccountApiCredentialSpec{
					ClusterManagedResourceSpec: xpv2.ClusterManagedResourceSpec{
						WriteConnectionSecretToReference: &xpv2.SecretReference{Name: secret.Name, Namespace: secret.Namespace},
					},
					ForProvider: securityv1alpha1.SubaccountApiCredentialParameters{SubaccountID: strptr(tc.specID), Name: strptr("configured-name")},
				},
				Status: securityv1alpha1.SubaccountApiCredentialStatus{
					AtProvider: securityv1alpha1.SubaccountApiCredentialObservation{SubaccountID: strptr(tc.observedID)},
				},
			}
			wrapped := &credentialFrameworkState{
				SubaccountApiCredential: cr,
				ctx:                     context.Background(),
				kube:                    kubefake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build(),
			}
			observation, err := wrapped.GetObservation()
			if err != nil {
				t.Fatal(err)
			}
			if observation["subaccount_id"] != tc.wantID {
				t.Errorf("Read subaccount = %q, want %q", observation["subaccount_id"], tc.wantID)
			}
			if observation["name"] != "configured-name" {
				t.Error("configured name was not retained in reconstructed Read state")
			}
			if observation["client_secret"] != "synthetic-secret" {
				t.Error("connection secret was not restored to the in-memory state")
			}
			if value(cr.Status.AtProvider.SubaccountID) != tc.observedID || value(cr.Spec.ForProvider.SubaccountID) != tc.specID {
				t.Error("state reconstruction changed the MR's spec or status")
			}
		})
	}
}

func strptr(s string) *string { return &s }

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
