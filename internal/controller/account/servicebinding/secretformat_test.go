package servicebinding

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
)

func TestDetectFormat(t *testing.T) {
	cases := map[string]struct {
		input []byte
		want  string
	}{
		"PlainString": {
			input: []byte("hello"),
			want:  "text",
		},
		"EmptyValue": {
			input: []byte(""),
			want:  "text",
		},
		"URLString": {
			input: []byte("https://example.com/api"),
			want:  "text",
		},
		"JSONObject": {
			input: []byte(`{"clientid":"abc","clientsecret":"xyz"}`),
			want:  "json",
		},
		"JSONArray": {
			input: []byte(`["tag1","tag2"]`),
			want:  "json",
		},
		"EmptyJSONObject": {
			input: []byte(`{}`),
			want:  "json",
		},
		"EmptyJSONArray": {
			input: []byte(`[]`),
			want:  "json",
		},
		"InvalidJSON": {
			input: []byte("{broken"),
			want:  "text",
		},
		"NumberAsString": {
			input: []byte("42"),
			want:  "text",
		},
		"BooleanAsString": {
			input: []byte("true"),
			want:  "text",
		},
		"NestedJSON": {
			input: []byte(`{"uaa":{"url":"https://auth.example.com","clientid":"x"}}`),
			want:  "json",
		},
		"WhitespaceJSON": {
			input: []byte(`  { "key": "value" }  `),
			want:  "json",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := detectFormat(tc.input)
			if got != tc.want {
				t.Errorf("detectFormat(%q) = %q, want %q", string(tc.input), got, tc.want)
			}
		})
	}
}

func TestEnrichConnectionDetails(t *testing.T) {
	cases := map[string]struct {
		inputCreds   map[string][]byte
		instanceName string
		instanceGUID string
		offeringName string
		planName     string
		wantKeys     []string
		wantErr      bool
	}{
		"EmptyCredentials": {
			inputCreds:   map[string][]byte{},
			instanceName: "my-instance",
			instanceGUID: "guid-123",
			offeringName: "xsuaa",
			planName:     "application",
			wantKeys:     []string{"type", "label", "plan", "tags", "instance_name", "instance_guid", ".metadata"},
		},
		"NilCredentials": {
			inputCreds:   nil,
			instanceName: "my-instance",
			instanceGUID: "guid-123",
			offeringName: "xsuaa",
			planName:     "application",
			wantKeys:     []string{"type", "label", "plan", "tags", "instance_name", "instance_guid", ".metadata"},
		},
		"WithStringCredentials": {
			inputCreds: map[string][]byte{
				"url":       []byte("https://api.example.com"),
				"client_id": []byte("admin"),
			},
			instanceName: "my-instance",
			instanceGUID: "guid-123",
			offeringName: "destination",
			planName:     "lite",
			wantKeys:     []string{"type", "label", "plan", "tags", "instance_name", "instance_guid", ".metadata", "url", "client_id"},
		},
		"WithJSONCredentials": {
			inputCreds: map[string][]byte{
				"uaa": []byte(`{"clientid":"x","clientsecret":"y"}`),
			},
			instanceName: "my-instance",
			instanceGUID: "guid-123",
			offeringName: "xsuaa",
			planName:     "application",
			wantKeys:     []string{"type", "label", "plan", "tags", "instance_name", "instance_guid", ".metadata", "uaa"},
		},
		"ReservedKeyOverwrite": {
			inputCreds: map[string][]byte{
				"type": []byte("old-type-from-credentials"),
				"url":  []byte("https://api.example.com"),
			},
			instanceName: "my-instance",
			instanceGUID: "guid-123",
			offeringName: "xsuaa",
			planName:     "application",
			wantKeys:     []string{"type", "label", "plan", "tags", "instance_name", "instance_guid", ".metadata", "url"},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			result, err := enrichConnectionDetails(tc.inputCreds, tc.instanceName, tc.instanceGUID, tc.offeringName, tc.planName, nil)

			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			for _, key := range tc.wantKeys {
				if _, ok := result[key]; !ok {
					t.Errorf("expected key %q in result, but not found", key)
				}
			}
		})
	}
}

func TestEnrichConnectionDetails_MetadataValues(t *testing.T) {
	result, err := enrichConnectionDetails(
		map[string][]byte{
			"url":       []byte("https://api.example.com"),
			"client_id": []byte("admin"),
		},
		"my-instance",
		"guid-123",
		"destination",
		"lite",
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if diff := cmp.Diff(string(result["type"]), "destination"); diff != "" {
		t.Errorf("type mismatch (-got +want):\n%s", diff)
	}
	if diff := cmp.Diff(string(result["label"]), "destination"); diff != "" {
		t.Errorf("label mismatch (-got +want):\n%s", diff)
	}
	if diff := cmp.Diff(string(result["plan"]), "lite"); diff != "" {
		t.Errorf("plan mismatch (-got +want):\n%s", diff)
	}
	if diff := cmp.Diff(string(result["instance_name"]), "my-instance"); diff != "" {
		t.Errorf("instance_name mismatch (-got +want):\n%s", diff)
	}
	if diff := cmp.Diff(string(result["instance_guid"]), "guid-123"); diff != "" {
		t.Errorf("instance_guid mismatch (-got +want):\n%s", diff)
	}
	if diff := cmp.Diff(string(result["tags"]), "[]"); diff != "" {
		t.Errorf("tags mismatch (-got +want):\n%s", diff)
	}
}

func TestEnrichConnectionDetails_MetadataDescriptor(t *testing.T) {
	result, err := enrichConnectionDetails(
		map[string][]byte{
			"url": []byte("https://api.example.com"),
			"uaa": []byte(`{"clientid":"x"}`),
		},
		"my-instance",
		"guid-123",
		"xsuaa",
		"application",
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var md secretMetadata
	if err := json.Unmarshal(result[".metadata"], &md); err != nil {
		t.Fatalf("failed to unmarshal .metadata: %v", err)
	}

	expectedMetaProps := []secretMetadataProperty{
		{Name: "instance_name", Format: "text"},
		{Name: "instance_guid", Format: "text"},
		{Name: "plan", Format: "text"},
		{Name: "label", Format: "text"},
		{Name: "type", Format: "text"},
		{Name: "tags", Format: "json"},
	}
	if diff := cmp.Diff(md.MetaDataProperties, expectedMetaProps); diff != "" {
		t.Errorf("metaDataProperties mismatch (-got +want):\n%s", diff)
	}

	expectedCredProps := []secretMetadataProperty{
		{Name: "uaa", Format: "json"},
		{Name: "url", Format: "text"},
	}
	if diff := cmp.Diff(md.CredentialProperties, expectedCredProps); diff != "" {
		t.Errorf("credentialProperties mismatch (-got +want):\n%s", diff)
	}
}

func TestEnrichConnectionDetails_WithSecretKey(t *testing.T) {
	secretKey := "credentials"
	result, err := enrichConnectionDetails(
		map[string][]byte{
			"credentials": []byte(`{"clientid":"x","url":"https://api.example.com"}`),
		},
		"my-instance",
		"guid-123",
		"destination",
		"lite",
		&secretKey,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Metadata keys should be present
	if string(result["type"]) != "destination" {
		t.Errorf("expected type 'destination', got %q", string(result["type"]))
	}
	if string(result["plan"]) != "lite" {
		t.Errorf("expected plan 'lite', got %q", string(result["plan"]))
	}

	// .metadata should have container: true for the secretKey
	var md secretMetadata
	if err := json.Unmarshal(result[".metadata"], &md); err != nil {
		t.Fatalf("failed to unmarshal .metadata: %v", err)
	}

	if len(md.CredentialProperties) != 1 {
		t.Fatalf("expected 1 credential property, got %d", len(md.CredentialProperties))
	}
	if md.CredentialProperties[0].Name != "credentials" {
		t.Errorf("expected credential name 'credentials', got %q", md.CredentialProperties[0].Name)
	}
	if md.CredentialProperties[0].Format != "json" {
		t.Errorf("expected format 'json', got %q", md.CredentialProperties[0].Format)
	}
	if !md.CredentialProperties[0].Container {
		t.Error("expected container: true")
	}
}

func TestEnrichConnectionDetails_ReservedKeyOverwrite(t *testing.T) {
	result, err := enrichConnectionDetails(
		map[string][]byte{
			"type": []byte("old-value"),
		},
		"my-instance",
		"guid-123",
		"xsuaa",
		"application",
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(result["type"]) != "xsuaa" {
		t.Errorf("expected type to be overwritten to 'xsuaa', got %q", string(result["type"]))
	}
}

func TestBundleCredentials(t *testing.T) {
	cases := map[string]struct {
		secretKey string
		details   map[string][]byte
		wantKey   string
		wantErr   bool
	}{
		"SingleJSONObject": {
			secretKey: "credentials",
			details: map[string][]byte{
				"attribute.credentials": []byte(`{"clientid":"x","clientsecret":"y","url":"https://api.example.com"}`),
			},
			wantKey: "credentials",
		},
		"MultipleKeys": {
			secretKey: "credentials",
			details: map[string][]byte{
				"url":      []byte("https://api.example.com"),
				"clientid": []byte("admin"),
			},
			wantKey: "credentials",
		},
		"EmptyDetails": {
			secretKey: "credentials",
			details:   map[string][]byte{},
			wantKey:   "credentials",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			result, err := bundleCredentials(tc.secretKey, tc.details)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if _, ok := result[tc.wantKey]; !ok {
				t.Errorf("expected key %q in result", tc.wantKey)
			}
			if len(result) != 1 {
				t.Errorf("expected exactly 1 key, got %d", len(result))
			}
			// Value should be valid JSON
			if !json.Valid(result[tc.wantKey]) {
				t.Errorf("value is not valid JSON: %s", string(result[tc.wantKey]))
			}
		})
	}
}

func TestBundleCredentials_SingleJSONPreserved(t *testing.T) {
	input := `{"clientid":"x","clientsecret":"y","uaa":{"url":"https://auth"}}`
	result, err := bundleCredentials("credentials", map[string][]byte{
		"attribute.credentials": []byte(input),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result["credentials"]) != input {
		t.Errorf("expected original JSON preserved, got %s", string(result["credentials"]))
	}
}

func TestAssembleCredentialJSON(t *testing.T) {
	cases := map[string]struct {
		details map[string][]byte
		check   func(t *testing.T, result []byte)
	}{
		"Empty": {
			details: map[string][]byte{},
			check: func(t *testing.T, result []byte) {
				if string(result) != "{}" {
					t.Errorf("expected '{}', got %q", string(result))
				}
			},
		},
		"SingleJSONPassthrough": {
			details: map[string][]byte{
				"creds": []byte(`{"a":"b"}`),
			},
			check: func(t *testing.T, result []byte) {
				if string(result) != `{"a":"b"}` {
					t.Errorf("expected passthrough, got %q", string(result))
				}
			},
		},
		"MultipleKeysAssembled": {
			details: map[string][]byte{
				"url":  []byte("https://api.example.com"),
				"user": []byte("admin"),
			},
			check: func(t *testing.T, result []byte) {
				var obj map[string]interface{}
				if err := json.Unmarshal(result, &obj); err != nil {
					t.Fatalf("invalid JSON: %v", err)
				}
				if obj["url"] != "https://api.example.com" {
					t.Errorf("url mismatch: %v", obj["url"])
				}
				if obj["user"] != "admin" {
					t.Errorf("user mismatch: %v", obj["user"])
				}
			},
		},
		"JSONValuePreservedAsObject": {
			details: map[string][]byte{
				"url": []byte("https://api.example.com"),
				"uaa": []byte(`{"clientid":"x"}`),
			},
			check: func(t *testing.T, result []byte) {
				var obj map[string]json.RawMessage
				if err := json.Unmarshal(result, &obj); err != nil {
					t.Fatalf("invalid JSON: %v", err)
				}
				var uaa map[string]interface{}
				if err := json.Unmarshal(obj["uaa"], &uaa); err != nil {
					t.Fatalf("uaa not a JSON object: %v", err)
				}
				if uaa["clientid"] != "x" {
					t.Errorf("uaa.clientid mismatch: %v", uaa["clientid"])
				}
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			result, err := assembleCredentialJSON(tc.details)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tc.check(t, result)
		})
	}
}

func TestProcessConnectionDetails(t *testing.T) {
	secretKey := "credentials"

	cases := map[string]struct {
		cr      *v1alpha1.ServiceBinding
		details map[string][]byte
		check   func(t *testing.T, result map[string][]byte)
	}{
		"NoSecretKey_Flattens": {
			cr: &v1alpha1.ServiceBinding{},
			details: map[string][]byte{
				"attribute.credentials": []byte(`{"clientid":"x","url":"https://api"}`),
			},
			check: func(t *testing.T, result map[string][]byte) {
				if _, ok := result["clientid"]; !ok {
					t.Error("expected flattened key 'clientid'")
				}
				if _, ok := result["url"]; !ok {
					t.Error("expected flattened key 'url'")
				}
			},
		},
		"WithSecretKey_Bundles": {
			cr: &v1alpha1.ServiceBinding{
				Spec: v1alpha1.ServiceBindingSpec{
					SecretKey: &secretKey,
				},
			},
			details: map[string][]byte{
				"attribute.credentials": []byte(`{"clientid":"x","url":"https://api"}`),
			},
			check: func(t *testing.T, result map[string][]byte) {
				if len(result) != 1 {
					t.Errorf("expected 1 key, got %d: %v", len(result), result)
				}
				if _, ok := result["credentials"]; !ok {
					t.Error("expected key 'credentials'")
				}
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			result, err := processConnectionDetails(tc.cr, tc.details)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tc.check(t, result)
		})
	}
}
