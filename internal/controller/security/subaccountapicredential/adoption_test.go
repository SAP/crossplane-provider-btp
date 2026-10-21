package subaccountapicredential

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAdoptionRejectingConnectorFinalizesRejectedAdoptionLocally(t *testing.T) {
	now := metav1.Now()
	cr := &v1alpha1.SubaccountApiCredential{ObjectMeta: metav1.ObjectMeta{UID: "fresh-uid", DeletionTimestamp: &now}}
	meta.SetExternalName(cr, "subaccount-guid/credential")
	cr.SetConditions(xpv1.Condition{Type: conditionAdoptionRejected, Status: "True", Message: lifecycleEvidenceMessage(cr)})
	cr.SetConditions(xpv1.Deleting()) // Ready may change during finalizer retries.
	connected := false
	connector := &adoptionRejectingConnector{ExternalConnector: managed.ExternalConnectorFn(func(context.Context, resource.Managed) (managed.ExternalClient, error) {
		connected = true
		return nil, nil
	})}
	client, err := connector.Connect(context.Background(), cr)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	if connected {
		t.Fatal("underlying connector was called for failed-adoption cleanup")
	}
	observation, err := client.Observe(context.Background(), cr)
	if err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	if observation.ResourceExists {
		t.Fatal("failed adoption must be treated as locally absent during cleanup")
	}
}

func TestDeletionWithoutCreationEvidenceFinalizesLocally(t *testing.T) {
	now := metav1.Now()
	cr := &v1alpha1.SubaccountApiCredential{ObjectMeta: metav1.ObjectMeta{UID: "legacy-uid", DeletionTimestamp: &now}}
	connected := false
	connector := &adoptionRejectingConnector{ExternalConnector: managed.ExternalConnectorFn(func(context.Context, resource.Managed) (managed.ExternalClient, error) {
		connected = true
		return nil, nil
	})}
	client, err := connector.Connect(context.Background(), cr)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	if connected {
		t.Fatal("unproven acquisition must not reach external Delete path")
	}
	observation, err := client.Observe(context.Background(), cr)
	if err != nil || observation.ResourceExists {
		t.Fatalf("unproven acquisition should be finalized locally: observation=%#v err=%v", observation, err)
	}
}

func TestAdoptionRejectingClientCreateCollision(t *testing.T) {
	cr := &v1alpha1.SubaccountApiCredential{}
	inner := managed.ExternalClientFns{
		CreateFn: func(context.Context, resource.Managed) (managed.ExternalCreation, error) {
			return managed.ExternalCreation{}, errors.New("credential already exists")
		},
	}
	_, err := (&adoptionRejectingClient{ExternalClient: inner}).Create(context.Background(), cr)
	if err == nil || !strings.Contains(err.Error(), errUnsupportedAdoption) {
		t.Fatalf("expected explicit adoption diagnostic, got %v", err)
	}
	if ready := cr.GetCondition(xpv1.TypeReady); ready.Status != corev1.ConditionFalse || !strings.Contains(ready.Message, errUnsupportedAdoption) {
		t.Errorf("expected explicit unavailable condition, got %#v", ready)
	}
	if !adoptionWasRejected(cr) {
		t.Fatal("Create collision must leave durable, identity-bound rejection evidence")
	}
}

func TestCertificateCredentialsKeepExistingLifecycle(t *testing.T) {
	certificate := "certificate-material"
	cr := &v1alpha1.SubaccountApiCredential{Spec: v1alpha1.SubaccountApiCredentialSpec{ForProvider: v1alpha1.SubaccountApiCredentialParameters{CertificatePassed: &certificate}}}
	observed := false
	inner := managed.ExternalClientFns{
		ObserveFn: func(context.Context, resource.Managed) (managed.ExternalObservation, error) {
			observed = true
			return managed.ExternalObservation{ResourceExists: true}, nil
		},
		DisconnectFn: func(context.Context) error { return nil },
	}
	got, err := (&adoptionRejectingClient{ExternalClient: inner}).Observe(context.Background(), cr)
	if err != nil || !observed || !got.ResourceExists {
		t.Fatalf("certificate Observe should retain existing behavior: observation=%#v observed=%t err=%v", got, observed, err)
	}

	now := metav1.Now()
	cr.UID = "certificate-uid"
	cr.DeletionTimestamp = &now
	cr.SetConditions(xpv1.Condition{Type: conditionAdoptionRejected, Status: "True", Message: lifecycleEvidenceMessage(cr)})
	connected := false
	connector := &adoptionRejectingConnector{ExternalConnector: managed.ExternalConnectorFn(func(context.Context, resource.Managed) (managed.ExternalClient, error) {
		connected = true
		return inner, nil
	})}
	if _, err := connector.Connect(context.Background(), cr); err != nil {
		t.Fatal(err)
	}
	if !connected {
		t.Fatal("certificate cleanup must retain its existing connector path")
	}
}

func TestCertificateInInitProviderIsRecognized(t *testing.T) {
	certificate := "certificate-material"
	cr := &v1alpha1.SubaccountApiCredential{Spec: v1alpha1.SubaccountApiCredentialSpec{
		InitProvider: v1alpha1.SubaccountApiCredentialInitParameters{CertificatePassed: &certificate},
	}}
	if !usesCertificate(cr) {
		t.Fatal("initProvider-only certificate should use certificate lifecycle")
	}
	cr.Spec.ForProvider.CertificatePassed = internal.Ptr("effective-certificate")
	if !usesCertificate(cr) {
		t.Fatal("forProvider certificate should take precedence")
	}
}

func TestAdoptionRejectingClientObserve(t *testing.T) {
	publicDetails := map[string][]byte{
		"attribute.client_id": []byte("public-id"),
		"attribute.api_url":   []byte("https://api.example.test"),
	}
	cases := map[string]struct {
		exists                 bool
		createSucceeded        bool
		copiedCreateAnnotation bool
		pendingSuperseded      bool
		failedSuperseded       bool
		previouslyRejected     bool
		deleting               bool
		observeErr             error
		wantAdoption           bool
	}{
		"existing target on fresh MR":                       {exists: true, wantAdoption: true},
		"copied create tracking annotation":                 {exists: true, copiedCreateAnnotation: true, wantAdoption: true},
		"target absent":                                     {},
		"successful provider Create":                        {exists: true, createSucceeded: true},
		"successful Create followed by rejection condition": {exists: true, createSucceeded: true, previouslyRejected: true},
		"tracking annotation is not creation evidence":      {exists: true, pendingSuperseded: true, wantAdoption: true},
		"copied create tracking annotation is not trusted":  {exists: true, failedSuperseded: true, wantAdoption: true},
		"repeated rejected adoption":                        {exists: true, previouslyRejected: true, wantAdoption: true},
		"deleting MR":                                       {exists: true, deleting: true},
		"read failure":                                      {exists: true, observeErr: context.DeadlineExceeded},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cr := &v1alpha1.SubaccountApiCredential{ObjectMeta: metav1.ObjectMeta{Name: "credential"}}
			meta.SetExternalName(cr, "subaccount-guid/credential-name")
			if tc.copiedCreateAnnotation {
				meta.SetExternalCreateSucceeded(cr, time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC))
			}
			if tc.createSucceeded {
				cr.SetConditions(xpv1.Condition{Type: conditionCredentialCreated, Status: "True", Message: lifecycleEvidenceMessage(cr)})
				base := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
				meta.SetExternalCreatePending(cr, base)
				meta.SetExternalCreateSucceeded(cr, base.Add(time.Second))
				if tc.pendingSuperseded {
					meta.SetExternalCreatePending(cr, base.Add(2*time.Second))
				}
				if tc.failedSuperseded {
					meta.SetExternalCreateFailed(cr, base.Add(2*time.Second))
				}
			}
			if tc.previouslyRejected {
				cr.SetConditions(xpv1.Condition{Type: conditionAdoptionRejected, Status: "True", Message: lifecycleEvidenceMessage(cr)})
				cr.SetConditions(xpv1.Unavailable().WithMessage(errUnsupportedAdoption))
			}
			if tc.deleting {
				cr.DeletionTimestamp = internal.Ptr(metav1.Now())
			}

			inner := managed.ExternalClientFns{
				ObserveFn: func(context.Context, resource.Managed) (managed.ExternalObservation, error) {
					return managed.ExternalObservation{ResourceExists: tc.exists, ResourceUpToDate: true, ConnectionDetails: publicDetails}, tc.observeErr
				},
				CreateFn: func(context.Context, resource.Managed) (managed.ExternalCreation, error) {
					return managed.ExternalCreation{}, nil
				},
				UpdateFn: func(context.Context, resource.Managed) (managed.ExternalUpdate, error) {
					return managed.ExternalUpdate{}, nil
				},
				DeleteFn: func(context.Context, resource.Managed) (managed.ExternalDelete, error) {
					return managed.ExternalDelete{}, nil
				},
				DisconnectFn: func(context.Context) error { return nil },
			}
			got, err := (&adoptionRejectingClient{ExternalClient: inner}).Observe(context.Background(), cr)
			if tc.wantAdoption {
				if err == nil || !strings.Contains(err.Error(), errUnsupportedAdoption) {
					t.Fatalf("expected explicit adoption error, got %v", err)
				}
				if !got.ResourceExists || !got.ResourceUpToDate || string(got.ConnectionDetails["attribute.client_id"]) != "public-id" {
					t.Errorf("partial public observation should be retained: %#v", got)
				}
				if ready := cr.GetCondition(xpv1.TypeReady); ready.Status != corev1.ConditionFalse || !strings.Contains(ready.Message, errUnsupportedAdoption) {
					t.Errorf("expected explicit unavailable condition, got %#v", ready)
				}
				return
			}
			if err != tc.observeErr {
				t.Fatalf("expected Observe error %v, got %v", tc.observeErr, err)
			}
		})
	}
}
