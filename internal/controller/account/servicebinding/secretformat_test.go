package servicebinding

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
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
			result, err := enrichConnectionDetails(tc.inputCreds, tc.instanceName, tc.instanceGUID, tc.offeringName, tc.planName)

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

func TestEnrichConnectionDetails_ReservedKeyOverwrite(t *testing.T) {
	result, err := enrichConnectionDetails(
		map[string][]byte{
			"type": []byte("old-value"),
		},
		"my-instance",
		"guid-123",
		"xsuaa",
		"application",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(result["type"]) != "xsuaa" {
		t.Errorf("expected type to be overwritten to 'xsuaa', got %q", string(result["type"]))
	}
}
