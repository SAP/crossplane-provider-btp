package subaccountdestinationgeneratedcertificate

import (
	"context"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal/clients/account/destination"
	destclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-destination-service-api-go/pkg"
)

type mockGenClient struct {
	keys        []destclient.PublicKey
	keysErr     error
	keysCalls   []string
	generated   *destclient.Attributes
	generateErr error
	deleted     []string
}

func (m *mockGenClient) PublicKeys(_ context.Context, name string) ([]destclient.PublicKey, error) {
	m.keysCalls = append(m.keysCalls, name)
	return m.keys, m.keysErr
}
func (m *mockGenClient) Generate(_ context.Context, a destclient.Attributes) error {
	m.generated = &a
	return m.generateErr
}
func (m *mockGenClient) Delete(_ context.Context, name string) error {
	m.deleted = append(m.deleted, name)
	return nil
}

var fixedNow = time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)

func newCR(extName string, p v1alpha1.SubaccountDestinationGeneratedCertificateParameters) *v1alpha1.SubaccountDestinationGeneratedCertificate {
	cr := &v1alpha1.SubaccountDestinationGeneratedCertificate{
		ObjectMeta: metav1.ObjectMeta{Name: "gen", Annotations: map[string]string{}},
		Spec:       v1alpha1.SubaccountDestinationGeneratedCertificateSpec{ForProvider: p},
	}
	if extName != "" {
		meta.SetExternalName(cr, extName)
	}
	return cr
}

func nowFn() time.Time { return fixedNow }

func TestObserve_EmptyExternalName(t *testing.T) {
	m := &mockGenClient{}
	e := &external{client: m, now: nowFn}
	obs, err := e.Observe(context.Background(), newCR("", v1alpha1.SubaccountDestinationGeneratedCertificateParameters{Name: "c.pem"}))
	if err != nil || obs.ResourceExists {
		t.Fatalf("obs = %+v, err = %v", obs, err)
	}
	if len(m.keysCalls) != 0 {
		t.Errorf("PublicKeys called for %v, want no call", m.keysCalls)
	}
}

func TestObserve_InvalidExternalName(t *testing.T) {
	e := &external{client: &mockGenClient{}, now: nowFn}
	if _, err := e.Observe(context.Background(), newCR("nodash", v1alpha1.SubaccountDestinationGeneratedCertificateParameters{Name: "c.pem"})); err == nil {
		t.Fatal("want error for invalid external name")
	}
}

func TestObserve_NotFound(t *testing.T) {
	e := &external{client: &mockGenClient{keysErr: destination.NewNotFoundError()}, now: nowFn}
	obs, err := e.Observe(context.Background(), newCR("sub/c.pem", v1alpha1.SubaccountDestinationGeneratedCertificateParameters{Name: "c.pem"}))
	if err != nil || obs.ResourceExists {
		t.Fatalf("obs = %+v, err = %v", obs, err)
	}
}

// The external name, not spec.forProvider.name, identifies the certificate:
// an adopted certificate can have a different name.
func TestObserve_Available(t *testing.T) {
	pemText, _ := selfSigned(t, issueSubject("cn"), big.NewInt(1), fixedNow.Add(30*24*time.Hour))
	cr := newCR("sub/adopted.pem", v1alpha1.SubaccountDestinationGeneratedCertificateParameters{Name: "c.pem"})
	m := &mockGenClient{keys: []destclient.PublicKey{{Content: pemText}}}
	e := &external{client: m, now: nowFn}
	obs, err := e.Observe(context.Background(), cr)
	if err != nil || !obs.ResourceExists || !obs.ResourceUpToDate {
		t.Fatalf("obs = %+v, err = %v", obs, err)
	}
	if len(m.keysCalls) != 1 || m.keysCalls[0] != "adopted.pem" {
		t.Errorf("PublicKeys called for %v, want [adopted.pem]", m.keysCalls)
	}
	if cr.Status.AtProvider.Name == nil || *cr.Status.AtProvider.Name != "adopted.pem" {
		t.Errorf("atProvider.name = %v", cr.Status.AtProvider.Name)
	}
	if cr.Status.AtProvider.Certificate == nil || cr.Status.AtProvider.Certificate.CommonName != "cn" {
		t.Errorf("atProvider.certificate = %+v", cr.Status.AtProvider.Certificate)
	}
	if c := cr.GetCondition(xpv1.TypeReady); c.Status != corev1.ConditionTrue {
		t.Errorf("ready = %+v", c)
	}
}

func TestObserve_Expired(t *testing.T) {
	pemText, _ := selfSigned(t, issueSubject("cn"), big.NewInt(1), fixedNow.Add(-time.Hour))
	cr := newCR("sub/c.pem", v1alpha1.SubaccountDestinationGeneratedCertificateParameters{Name: "c.pem"})
	e := &external{client: &mockGenClient{keys: []destclient.PublicKey{{Content: pemText}}}, now: func() time.Time { return fixedNow }}
	obs, err := e.Observe(context.Background(), cr)
	if err != nil || !obs.ResourceExists || !obs.ResourceUpToDate {
		t.Fatalf("obs = %+v, err = %v", obs, err)
	}
	c := cr.GetCondition(xpv1.TypeReady)
	if c.Status != corev1.ConditionFalse || c.Reason != xpv1.ReasonUnavailable || !strings.Contains(c.Message, "expired at") {
		t.Errorf("ready = %+v", c)
	}
}

func TestObserve_UnparseableDoesNotBlock(t *testing.T) {
	cr := newCR("sub/c.pem", v1alpha1.SubaccountDestinationGeneratedCertificateParameters{Name: "c.pem"})
	e := &external{client: &mockGenClient{keys: nil}, now: nowFn}
	obs, err := e.Observe(context.Background(), cr)
	if err != nil || !obs.ResourceExists || !obs.ResourceUpToDate {
		t.Fatalf("obs = %+v, err = %v", obs, err)
	}
	if cr.Status.AtProvider.Certificate != nil {
		t.Errorf("certificate = %+v, want nil", cr.Status.AtProvider.Certificate)
	}
	c := cr.GetCondition(xpv1.TypeReady)
	if c.Status != corev1.ConditionFalse || c.Reason != xpv1.ReasonUnavailable {
		t.Errorf("ready = %+v", c)
	}
}

func TestCreate_RequiresSubaccountID(t *testing.T) {
	m := &mockGenClient{}
	e := &external{client: m, now: nowFn}
	cr := newCR("", v1alpha1.SubaccountDestinationGeneratedCertificateParameters{Name: "c.pem"})
	if _, err := e.Create(context.Background(), cr); err == nil {
		t.Fatal("want error without subaccountId")
	}
	if m.generated != nil {
		t.Errorf("Generate called: %+v", m.generated)
	}
}

func TestCreate_MinimalPEM(t *testing.T) {
	m := &mockGenClient{}
	e := &external{client: m, now: nowFn}
	cr := newCR("", v1alpha1.SubaccountDestinationGeneratedCertificateParameters{Name: "c.pem", SubaccountID: new("sub")})
	if _, err := e.Create(context.Background(), cr); err != nil {
		t.Fatalf("Create: %v", err)
	}
	g := m.generated
	if g == nil {
		t.Fatal("Generate not called")
	}
	if g.Name != "c.pem" || g.Type != nil {
		t.Errorf("name/type = %q/%v", g.Name, g.Type)
	}
	a := g.Attributes
	if a.CN != nil || a.Validity != nil || a.Password != nil {
		t.Errorf("optional attributes set: %+v", a)
	}
	if a.AutomaticRenew == nil || *a.AutomaticRenew {
		t.Errorf("AutomaticRenew = %v, want explicit false", a.AutomaticRenew)
	}
	if got := meta.GetExternalName(cr); got != "sub/c.pem" {
		t.Errorf("external name = %q", got)
	}
}

func TestCreate_FullP12(t *testing.T) {
	kube := kubefake.NewClientBuilder().WithObjects(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "pw", Namespace: "crossplane-system"},
		Data:       map[string][]byte{"password": []byte("s3cret")},
	}).Build()
	m := &mockGenClient{}
	e := &external{kube: kube, client: m, now: nowFn}
	cr := newCR("", v1alpha1.SubaccountDestinationGeneratedCertificateParameters{
		Name:           "c.p12",
		SubaccountID:   new("sub"),
		CommonName:     new("my-cn"),
		Validity:       &v1alpha1.GeneratedCertificateValidity{TimeUnit: "MONTHS", Value: 3},
		AutomaticRenew: new(true),
		PasswordSecretRef: &xpv1.SecretKeySelector{
			SecretReference: xpv1.SecretReference{Name: "pw", Namespace: "crossplane-system"},
			Key:             "password",
		},
	})
	if _, err := e.Create(context.Background(), cr); err != nil {
		t.Fatalf("Create: %v", err)
	}
	g := m.generated
	if g == nil {
		t.Fatal("Generate not called")
	}
	if g.Type == nil || *g.Type != "P12" {
		t.Errorf("Type = %v", g.Type)
	}
	a := g.Attributes
	if a.CN == nil || *a.CN != "my-cn" {
		t.Errorf("CN = %v", a.CN)
	}
	if a.Validity == nil || a.Validity.TimeUnit == nil || *a.Validity.TimeUnit != "MONTHS" || a.Validity.Value == nil || *a.Validity.Value != 3 {
		t.Errorf("Validity = %+v", a.Validity)
	}
	if a.Password == nil || *a.Password != "s3cret" {
		t.Error("Password not set from secret")
	}
	if a.AutomaticRenew == nil || !*a.AutomaticRenew {
		t.Errorf("AutomaticRenew = %v, want true", a.AutomaticRenew)
	}
	if got := meta.GetExternalName(cr); got != "sub/c.p12" {
		t.Errorf("external name = %q", got)
	}
}

func TestCreate_PresetExternalNameIsUsed(t *testing.T) {
	m := &mockGenClient{}
	e := &external{client: m, now: nowFn}
	cr := newCR("sub/other.pem", v1alpha1.SubaccountDestinationGeneratedCertificateParameters{Name: "c.pem", SubaccountID: new("sub")})
	if _, err := e.Create(context.Background(), cr); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if m.generated == nil || m.generated.Name != "other.pem" {
		t.Fatalf("generated = %+v", m.generated)
	}
	if got := meta.GetExternalName(cr); got != "sub/other.pem" {
		t.Errorf("external name = %q", got)
	}
}

func TestCreate_PresetExternalNameBadSuffix(t *testing.T) {
	m := &mockGenClient{}
	e := &external{client: m, now: nowFn}
	cr := newCR("sub/other.jks", v1alpha1.SubaccountDestinationGeneratedCertificateParameters{Name: "c.pem", SubaccountID: new("sub")})
	if _, err := e.Create(context.Background(), cr); err == nil {
		t.Fatal("want error for unsupported suffix")
	}
	if m.generated != nil {
		t.Errorf("Generate called: %+v", m.generated)
	}
}

func TestCreate_Conflict(t *testing.T) {
	e := &external{client: &mockGenClient{generateErr: destination.NewConflictError()}, now: nowFn}
	cr := newCR("", v1alpha1.SubaccountDestinationGeneratedCertificateParameters{Name: "c.pem", SubaccountID: new("sub")})
	_, err := e.Create(context.Background(), cr)
	if err == nil || err.Error() != errAlreadyExists {
		t.Fatalf("err = %v, want %q", err, errAlreadyExists)
	}
}

func TestCreate_MissingPasswordKey(t *testing.T) {
	kube := kubefake.NewClientBuilder().WithObjects(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "pw", Namespace: "crossplane-system"},
		Data:       map[string][]byte{"other": []byte("s3cret")},
	}).Build()
	m := &mockGenClient{}
	e := &external{kube: kube, client: m, now: nowFn}
	cr := newCR("", v1alpha1.SubaccountDestinationGeneratedCertificateParameters{
		Name:         "c.pem",
		SubaccountID: new("sub"),
		PasswordSecretRef: &xpv1.SecretKeySelector{
			SecretReference: xpv1.SecretReference{Name: "pw", Namespace: "crossplane-system"},
			Key:             "password",
		},
	})
	_, err := e.Create(context.Background(), cr)
	if err == nil {
		t.Fatal("want error for missing password key")
	}
	if !strings.Contains(err.Error(), "crossplane-system/pw") {
		t.Errorf("error doesn't name the secret: %v", err)
	}
	if strings.Contains(err.Error(), "s3cret") {
		t.Error("error leaks secret value")
	}
	if m.generated != nil {
		t.Errorf("Generate called: %+v", m.generated)
	}
}

// An empty value would generate an unprotected key that can never be fixed:
// passwordSecretRef is immutable and read only on Create.
func TestCreate_EmptyPassword(t *testing.T) {
	kube := kubefake.NewClientBuilder().WithObjects(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "pw", Namespace: "crossplane-system"},
		Data:       map[string][]byte{"password": {}},
	}).Build()
	m := &mockGenClient{}
	e := &external{kube: kube, client: m, now: nowFn}
	cr := newCR("", v1alpha1.SubaccountDestinationGeneratedCertificateParameters{
		Name:         "c.p12",
		SubaccountID: new("sub"),
		PasswordSecretRef: &xpv1.SecretKeySelector{
			SecretReference: xpv1.SecretReference{Name: "pw", Namespace: "crossplane-system"},
			Key:             "password",
		},
	})
	_, err := e.Create(context.Background(), cr)
	if err == nil || !strings.Contains(err.Error(), "crossplane-system/pw") {
		t.Fatalf("err = %v, want error naming the secret", err)
	}
	if m.generated != nil {
		t.Errorf("Generate called: %+v", m.generated)
	}
}

func TestUpdate_NoClientCall(t *testing.T) {
	m := &mockGenClient{}
	e := &external{client: m, now: nowFn}
	if _, err := e.Update(context.Background(), newCR("sub/c.pem", v1alpha1.SubaccountDestinationGeneratedCertificateParameters{Name: "c.pem"})); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if m.generated != nil || len(m.deleted) != 0 || len(m.keysCalls) != 0 {
		t.Errorf("client called: %+v", m)
	}
}

func TestDelete(t *testing.T) {
	m := &mockGenClient{}
	e := &external{client: m, now: nowFn}
	if _, err := e.Delete(context.Background(), newCR("sub/adopted.pem", v1alpha1.SubaccountDestinationGeneratedCertificateParameters{Name: "c.pem"})); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(m.deleted) != 1 || m.deleted[0] != "adopted.pem" {
		t.Errorf("deleted = %v, want [adopted.pem]", m.deleted)
	}
}

func TestDelete_EmptyExternalName(t *testing.T) {
	m := &mockGenClient{}
	e := &external{client: m, now: nowFn}
	if _, err := e.Delete(context.Background(), newCR("", v1alpha1.SubaccountDestinationGeneratedCertificateParameters{Name: "c.pem"})); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(m.deleted) != 0 {
		t.Errorf("deleted = %v", m.deleted)
	}
}
