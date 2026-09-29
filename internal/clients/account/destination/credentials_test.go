package destination

import (
	"context"
	"encoding/json"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func secretWith(name, ns string, data map[string][]byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Data:       data,
	}
}

func TestLoadFromSecret_FlatKeys(t *testing.T) {
	kube := fake.NewClientBuilder().WithObjects(secretWith("dest-secret", "default", map[string][]byte{
		"clientid":     []byte("cid"),
		"clientsecret": []byte("csec"),
		"tokenurl":     []byte("https://token"),
		"uri":          []byte("https://dest"),
	})).Build()
	ref := xpv1.SecretKeySelector{
		SecretReference: xpv1.SecretReference{Name: "dest-secret", Namespace: "default"},
	}

	raw, err := LoadFromSecret(context.Background(), kube, ref)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got map[string]string
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if got["clientid"] != "cid" {
		t.Errorf("clientid = %q, want %q", got["clientid"], "cid")
	}
	if got["tokenurl"] != "https://token" {
		t.Errorf("tokenurl = %q, want %q", got["tokenurl"], "https://token")
	}
}

func TestLoadFromSecret_SingleJSONKey(t *testing.T) {
	payload := `{"clientid":"cid","clientsecret":"csec","tokenurl":"https://token","uri":"https://dest"}`
	kube := fake.NewClientBuilder().WithObjects(secretWith("dest-secret", "default", map[string][]byte{
		"credentials": []byte(payload),
	})).Build()
	ref := xpv1.SecretKeySelector{
		SecretReference: xpv1.SecretReference{Name: "dest-secret", Namespace: "default"},
		Key:             "credentials",
	}

	raw, err := LoadFromSecret(context.Background(), kube, ref)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(raw) != payload {
		t.Errorf("raw = %q, want %q", raw, payload)
	}
}

func TestLoadFromSecret_MissingSecret(t *testing.T) {
	kube := fake.NewClientBuilder().Build()
	ref := xpv1.SecretKeySelector{
		SecretReference: xpv1.SecretReference{Name: "missing", Namespace: "default"},
	}

	_, err := LoadFromSecret(context.Background(), kube, ref)
	if err == nil {
		t.Fatal("expected error for missing secret, got nil")
	}
}

func TestLoadFromSecret_MissingKey_SingleJSONFormat(t *testing.T) {
	kube := fake.NewClientBuilder().WithObjects(secretWith("dest-secret", "default", map[string][]byte{
		"other-key": []byte("value"),
	})).Build()
	ref := xpv1.SecretKeySelector{
		SecretReference: xpv1.SecretReference{Name: "dest-secret", Namespace: "default"},
		Key:             "credentials",
	}

	_, err := LoadFromSecret(context.Background(), kube, ref)
	if err == nil {
		t.Fatal("expected error when named key is absent, got nil")
	}
}

func TestLoadFromSecret_MissingUri_FlatKeys(t *testing.T) {
	kube := fake.NewClientBuilder().WithObjects(secretWith("dest-secret", "default", map[string][]byte{
		"clientid":     []byte("cid"),
		"clientsecret": []byte("csec"),
		"tokenurl":     []byte("https://token"),
		// uri intentionally absent
	})).Build()
	ref := xpv1.SecretKeySelector{
		SecretReference: xpv1.SecretReference{Name: "dest-secret", Namespace: "default"},
	}

	_, err := LoadFromSecret(context.Background(), kube, ref)
	if err == nil {
		t.Fatal("expected error when uri key is missing, got nil")
	}
}

func TestLoadFromSecret_TokenUrlAlias(t *testing.T) {
	kube := fake.NewClientBuilder().WithObjects(secretWith("dest-secret", "default", map[string][]byte{
		"clientid":     []byte("cid"),
		"clientsecret": []byte("csec"),
		"token_url":    []byte("https://token"),
		"uri":          []byte("https://dest"),
	})).Build()
	ref := xpv1.SecretKeySelector{
		SecretReference: xpv1.SecretReference{Name: "dest-secret", Namespace: "default"},
	}

	raw, err := LoadFromSecret(context.Background(), kube, ref)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got map[string]string
	_ = json.Unmarshal(raw, &got)
	if got["tokenurl"] == "" {
		t.Error("expected tokenurl to be set from token_url alias, got empty")
	}
}

func TestLoadFromSecret_UrlAlias(t *testing.T) {
	kube := fake.NewClientBuilder().WithObjects(secretWith("dest-secret", "default", map[string][]byte{
		"clientid":     []byte("cid"),
		"clientsecret": []byte("csec"),
		"tokenurl":     []byte("https://token"),
		"url":          []byte("https://dest"),
	})).Build()
	ref := xpv1.SecretKeySelector{
		SecretReference: xpv1.SecretReference{Name: "dest-secret", Namespace: "default"},
	}

	raw, err := LoadFromSecret(context.Background(), kube, ref)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got map[string]string
	_ = json.Unmarshal(raw, &got)
	if got["uri"] == "" {
		t.Error("expected uri to be set from url alias, got empty")
	}
}
