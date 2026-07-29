package subaccountservicebroker

import (
	"context"
	"testing"
)

func TestGetBrokerID(t *testing.T) {
	const (
		subID    = "6aa64c2f-38c1-49a9-b2e8-cf9fea769b7f"
		brokerID = "6a55f158-41b5-4e63-aa77-84089fa0ab98"
	)

	cases := map[string]struct {
		externalName string
		want         string
		wantErr      bool
	}{
		"empty external-name returns non-empty placeholder GUID": {
			externalName: "",
			want:         notEmptyGUID,
		},
		"bare GUID passes through unchanged (legacy/migration input)": {
			externalName: brokerID,
			want:         brokerID,
		},
		"compound key returns bare broker id (keeps tfstate id valid)": {
			externalName: subID + "/" + brokerID,
			want:         brokerID,
		},
		"compound key missing broker part is an error": {
			externalName: subID + "/",
			wantErr:      true,
		},
		"compound key missing subaccount part is an error": {
			externalName: "/" + brokerID,
			wantErr:      true,
		},
		"external-name with more than one slash is an error": {
			externalName: subID + "/" + brokerID + "/extra",
			wantErr:      true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := getBrokerID(context.Background(), tc.externalName, nil, nil)
			if (err != nil) != tc.wantErr {
				t.Fatalf("getBrokerID(%q) error = %v, wantErr %v", tc.externalName, err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("getBrokerID(%q) = %q, want %q", tc.externalName, got, tc.want)
			}
		})
	}
}

func TestGetBrokerExternalName(t *testing.T) {
	cases := map[string]struct {
		tfstate map[string]any
		want    string
		wantErr bool
	}{
		"both fields present builds compound key": {
			tfstate: map[string]any{"subaccount_id": "sub-1", "id": "broker-1"},
			want:    "sub-1/broker-1",
		},
		"missing subaccount_id is an error": {
			tfstate: map[string]any{"id": "broker-1"},
			wantErr: true,
		},
		"missing id is an error": {
			tfstate: map[string]any{"subaccount_id": "sub-1"},
			wantErr: true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := getBrokerExternalName(tc.tfstate)
			if (err != nil) != tc.wantErr {
				t.Fatalf("getBrokerExternalName(%v) error = %v, wantErr %v", tc.tfstate, err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("getBrokerExternalName(%v) = %q, want %q", tc.tfstate, got, tc.want)
			}
		})
	}
}

// TestExternalNameRoundTrip locks the invariant the two hooks must share:
// the compound key emitted by getBrokerExternalName must map back through
// getBrokerID to the bare broker id, or tfstate.id breaks on the reconcile
// after every provider restart.
func TestExternalNameRoundTrip(t *testing.T) {
	const (
		subID    = "6aa64c2f-38c1-49a9-b2e8-cf9fea769b7f"
		brokerID = "6a55f158-41b5-4e63-aa77-84089fa0ab98"
	)

	externalName, err := getBrokerExternalName(map[string]any{"subaccount_id": subID, "id": brokerID})
	if err != nil {
		t.Fatalf("getBrokerExternalName() unexpected error: %v", err)
	}
	if want := subID + "/" + brokerID; externalName != want {
		t.Fatalf("getBrokerExternalName() = %q, want %q", externalName, want)
	}

	got, err := getBrokerID(context.Background(), externalName, nil, nil)
	if err != nil {
		t.Fatalf("getBrokerID(%q) unexpected error: %v", externalName, err)
	}
	if got != brokerID {
		t.Fatalf("round-trip broke tfstate id: getBrokerID(getBrokerExternalName(...)) = %q, want %q", got, brokerID)
	}
}
