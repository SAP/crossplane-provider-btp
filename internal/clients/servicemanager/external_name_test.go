package servicemanager

import (
	"errors"
	"strings"
	"testing"
)

const (
	testInstanceUUID = "6aa64c2f-38c1-49a9-b2e8-cf9fea769b7f"
	testBindingUUID  = "9c2b1f80-3d4e-4a11-8f2c-7b5d6e1a4c33"
)

// ADR(external-name): ServiceManager's key is "<serviceInstanceID>/<serviceBindingID>",
// with a bare "<serviceInstanceID>" as the legitimate phase-1 transient. Fallbacks are
// handed to the recovery path instead of being rejected.
func TestValidateExternalName(t *testing.T) {
	const metadataName = "my-service-manager"

	tests := map[string]struct {
		externalName string
		wantErr      bool
	}{
		"unset is a fallback owned by the recovery path": {
			externalName: "",
		},
		"metadata.name is the pre-suppression default and must still reconcile": {
			externalName: metadataName,
		},
		"bare instance id is the phase-1 transient": {
			externalName: testInstanceUUID,
		},
		"compound key is the steady state": {
			externalName: testInstanceUUID + "/" + testBindingUUID,
		},
		"three segments would silently degrade to instance-only": {
			externalName: testInstanceUUID + "/" + testBindingUUID + "/" + testInstanceUUID,
			wantErr:      true,
		},
		"trailing separator leaves an empty binding segment": {
			externalName: testInstanceUUID + "/",
			wantErr:      true,
		},
		"leading separator leaves an empty instance segment": {
			externalName: "/" + testBindingUUID,
			wantErr:      true,
		},
		"non-uuid instance segment": {
			externalName: "not-a-uuid/" + testBindingUUID,
			wantErr:      true,
		},
		"non-uuid binding segment": {
			externalName: testInstanceUUID + "/not-a-uuid",
			wantErr:      true,
		},
		// uuid.Validate accepts these three spellings; BTP does not. They must be
		// rejected here rather than fail opaquely at the API call.
		"braced uuid spelling is not a BTP guid": {
			externalName: "{" + testInstanceUUID + "}/" + testBindingUUID,
			wantErr:      true,
		},
		"urn-prefixed uuid spelling is not a BTP guid": {
			externalName: "urn:uuid:" + testInstanceUUID,
			wantErr:      true,
		},
		"unhyphenated uuid spelling is not a BTP guid": {
			externalName: strings.ReplaceAll(testInstanceUUID, "-", ""),
			wantErr:      true,
		},
		"bare non-uuid that is not the fallback": {
			externalName: "some-instance-name",
			wantErr:      true,
		},
		"leading space is not allowed": {
			externalName: " " + testInstanceUUID,
			wantErr:      true,
		},
		"trailing space is not allowed": {
			externalName: testInstanceUUID + " ",
			wantErr:      true,
		},
		"inner space around the separator is not allowed": {
			externalName: testInstanceUUID + "/ " + testBindingUUID,
			wantErr:      true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := ValidateExternalName(metadataName, tc.externalName)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ValidateExternalName(%q, %q) = nil, want an error", metadataName, tc.externalName)
				}
				if !errors.Is(err, ErrInvalidExternalName) {
					t.Fatalf("ValidateExternalName(%q, %q) = %v, want errors.Is(_, ErrInvalidExternalName)", metadataName, tc.externalName, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateExternalName(%q, %q) = %v, want nil", metadataName, tc.externalName, err)
			}
		})
	}
}

// splitExternalName is what turns the validated key back into the two IDs the
// child resources are addressed by.
func TestSplitExternalName(t *testing.T) {
	tests := map[string]struct {
		externalName string
		wantInstance string
		wantBinding  string
	}{
		"compound key yields both ids": {
			externalName: testInstanceUUID + "/" + testBindingUUID,
			wantInstance: testInstanceUUID,
			wantBinding:  testBindingUUID,
		},
		"phase-1 transient yields the instance id only": {
			externalName: testInstanceUUID,
			wantInstance: testInstanceUUID,
		},
		// The silent degradation ValidateExternalName exists to prevent: a third
		// segment is dropped, leaving "binding missing" and a duplicate Create.
		"more than two segments degrade to instance-only": {
			externalName: testInstanceUUID + "/" + testBindingUUID + "/" + testInstanceUUID,
			wantInstance: testInstanceUUID,
		},
		"unset yields neither": {},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			gotInstance, gotBinding := splitExternalName(tc.externalName)
			if gotInstance != tc.wantInstance {
				t.Errorf("splitExternalName(%q) instance = %q, want %q", tc.externalName, gotInstance, tc.wantInstance)
			}
			if gotBinding != tc.wantBinding {
				t.Errorf("splitExternalName(%q) binding = %q, want %q", tc.externalName, gotBinding, tc.wantBinding)
			}
		})
	}
}
