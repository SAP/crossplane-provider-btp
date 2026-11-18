package rolecollection

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/SAP/xp-clifford/yaml"
	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/sap/crossplane-provider-btp/apis/security/v1alpha1"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources/subaccount"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestConvertRoleCollectionResource(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	// Keep all subaccounts so the parent reference can be resolved.
	viper.Set("subaccount", []string{".*"})

	for _, tt := range []struct {
		name           string
		rcName         string
		parentGUID     string
		description    string
		roleReferences []v1alpha1.RoleReference
		externalName   string
		resourceName   string
		wantComments   []string
	}{
		{
			name:         "complete role collection carries empty api credential reference and todo",
			rcName:       "my-collection",
			parentGUID:   "sa-one",
			description:  "a description",
			externalName: "my-collection",
			resourceName: "my-collection.sa-one",
			roleReferences: []v1alpha1.RoleReference{
				{RoleTemplateAppId: "app-id", RoleTemplateName: "template", Name: "role"},
			},
			wantComments: []string{"TODO: provide xsuaa credentials"},
		},
		{
			name:         "empty description left nil",
			rcName:       "no-desc",
			parentGUID:   "sa-one",
			externalName: "no-desc",
			resourceName: "no-desc.sa-one",
			wantComments: []string{"TODO: provide xsuaa credentials"},
		},
		{
			name:         "missing name yields undefined names and warnings",
			parentGUID:   "sa-one",
			externalName: resources.UndefinedExternalName,
			resourceName: resources.UndefinedName,
			wantComments: []string{
				resources.WarnUndefinedResourceName,
				resources.WarnUndefinedExternalName,
				"TODO: provide xsuaa credentials",
			},
		},
		{
			name:         "missing parent guid still carries empty reference and todo",
			rcName:       "orphan",
			externalName: "orphan",
			resourceName: resources.UndefinedName,
			wantComments: []string{
				resources.WarnUndefinedResourceName,
				"TODO: provide xsuaa credentials",
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			subaccount.Reset()
			t.Cleanup(subaccount.Reset)

			rc := &rolecollection{
				roleCollection: &btpcli.RoleCollection{
					ParentGUID:     tt.parentGUID,
					Name:           tt.rcName,
					Description:    tt.description,
					RoleReferences: tt.roleReferences,
				},
				ResourceWithComment: yaml.NewResourceWithComment(nil),
			}

			result := convertRoleCollectionResource(t.Context(), rc)
			cr := result.Resource().(*v1alpha1.RoleCollection)

			require.Equal(t, v1alpha1.RoleCollectionKind, cr.Kind)
			require.Equal(t, "security.btp.sap.crossplane.io/v1alpha1", cr.APIVersion)
			require.Equal(t, tt.externalName, cr.Annotations["crossplane.io/external-name"])
			require.Equal(t, tt.resourceName, cr.Name)
			require.Equal(t, tt.rcName, cr.Spec.ForProvider.Name)
			if tt.description == "" {
				require.Nil(t, cr.Spec.ForProvider.Description)
			} else {
				require.Equal(t, tt.description, *cr.Spec.ForProvider.Description)
			}
			require.Equal(t, tt.roleReferences, cr.Spec.ForProvider.RoleReferences)
			require.Equal(t, xpv1.ManagementPolicies{xpv1.ManagementActionObserve}, cr.Spec.ManagementPolicies)
			require.Equal(t, &xpv1.Reference{Name: ""}, cr.Spec.SubaccountApiCredentialRef)

			comment, hasComment := result.Comment()
			if len(tt.wantComments) == 0 {
				require.False(t, hasComment)
			} else {
				require.True(t, hasComment)
				for _, want := range tt.wantComments {
					require.Contains(t, comment, want)
				}
			}
		})
	}
}

// fakeBTPCLI writes a stub btp CLI answering the two commands the convert and
// selection paths rely on: the authorized subaccount list (used to resolve the
// parent subaccount's exported k8s name) and the role-collections list.
func fakeBTPCLI(t *testing.T) string {
	t.Helper()
	cliPath := filepath.Join(t.TempDir(), "btp")
	require.NoError(t, os.WriteFile(cliPath, []byte(`#!/bin/sh
case "$*" in
  *"accounts/subaccount"*)
    echo '{"value":[{"guid":"sa-one","displayName":"sub-one","region":"sa-one"},{"guid":"sa-two","displayName":"sub-two","region":"sa-two"}]}'
    ;;
  *"security/role-collections --subaccount sa-one"*)
    echo '[{"name":"alpha","description":"first"},{"name":"beta"}]'
    ;;
  *"security/role-collections --subaccount sa-two"*)
    echo '[{"name":"alpha","description":"first in two"},{"name":"gamma"}]'
    ;;
  *) echo "unexpected command: $*" >&2; exit 1 ;;
esac
`), 0o755))
	return cliPath
}
