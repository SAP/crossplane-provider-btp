package subaccount

import (
	"testing"

	"github.com/SAP/xp-clifford/yaml"
	v1 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
)

func TestConvertSubaccountResource(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	displayName := "Test Subaccount"
	saGuid := "12345678-1234-1234-1234-123456789abc"
	region := "eu10"
	subdomain := "test-subdomain"
	admins := []string{"admin@example.com", "admin2@example.com"}
	gaGuid := "global-account-guid"
	dirGuid := "directory-guid"
	description := "Test description"
	usedForProd := "USED_FOR_PRODUCTION"
	betaEnabled := true
	labels := map[string][]string{"env": {"dev"}, "team": {"platform"}}
	displayNameSpecial := "Test_Subaccount With Spaces & Special!@#"
	empty := ""
	wantResourceName := "test-subaccount.eu10"

	tests := []struct {
		name   string
		sa     *btpcli.Subaccount
		admins []string
		want   *yaml.ResourceWithComment
	}{
		{
			name: "all required fields present",
			sa: &btpcli.Subaccount{
				DisplayName: displayName,
				GUID:        saGuid,
				Region:      region,
				Subdomain:   subdomain,
			},
			admins: admins,
			want: yaml.NewResourceWithComment(
				&v1alpha1.Subaccount{
					TypeMeta: metav1.TypeMeta{
						Kind:       v1alpha1.SubaccountKind,
						APIVersion: v1alpha1.CRDGroupVersion.String(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: wantResourceName,
						Annotations: map[string]string{
							"crossplane.io/external-name": saGuid,
						},
					},
					Spec: v1alpha1.SubaccountSpec{
						ClusterManagedResourceSpec: v1.ClusterManagedResourceSpec{
							ManagementPolicies: []v1.ManagementAction{v1.ManagementActionObserve},
						},
						ForProvider: v1alpha1.SubaccountParameters{
							DisplayName:      displayName,
							Region:           region,
							Subdomain:        subdomain,
							SubaccountAdmins: admins,
						},
					},
				}),
		},
		{
			name: "all fields including optional",
			sa: &btpcli.Subaccount{
				DisplayName:       displayName,
				GUID:              saGuid,
				Region:            region,
				Subdomain:         subdomain,
				GlobalAccountGUID: gaGuid,
				ParentGUID:        dirGuid,
				Description:       description,
				UsedForProduction: usedForProd,
				BetaEnabled:       betaEnabled,
				Labels:            labels,
			},
			admins: admins,
			want: yaml.NewResourceWithComment(
				&v1alpha1.Subaccount{
					TypeMeta: metav1.TypeMeta{
						Kind:       v1alpha1.SubaccountKind,
						APIVersion: v1alpha1.CRDGroupVersion.String(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: wantResourceName,
						Annotations: map[string]string{
							"crossplane.io/external-name": saGuid,
						},
					},
					Spec: v1alpha1.SubaccountSpec{
						ClusterManagedResourceSpec: v1.ClusterManagedResourceSpec{
							ManagementPolicies: []v1.ManagementAction{v1.ManagementActionObserve},
						},
						ForProvider: v1alpha1.SubaccountParameters{
							DisplayName:       displayName,
							Region:            region,
							Subdomain:         subdomain,
							SubaccountAdmins:  admins,
							GlobalAccountGuid: gaGuid,
							DirectoryGuid:     dirGuid,
							Description:       description,
							UsedForProduction: usedForProd,
							BetaEnabled:       true,
							Labels:            map[string]v1alpha1.SubaccountLabelValueList{"env": {"dev"}, "team": {"platform"}},
						},
					},
				}),
		},
		{
			name: "missing displayName",
			sa: &btpcli.Subaccount{
				GUID:      saGuid,
				Region:    region,
				Subdomain: subdomain,
			},
			admins: admins,
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1alpha1.Subaccount{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1alpha1.SubaccountKind,
							APIVersion: v1alpha1.CRDGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resources.UndefinedName,
							Annotations: map[string]string{
								"crossplane.io/external-name": saGuid,
							},
						},
						Spec: v1alpha1.SubaccountSpec{
							ClusterManagedResourceSpec: v1.ClusterManagedResourceSpec{
								ManagementPolicies: []v1.ManagementAction{v1.ManagementActionObserve},
							},
							ForProvider: v1alpha1.SubaccountParameters{
								Region:           region,
								Subdomain:        subdomain,
								SubaccountAdmins: admins,
							},
						},
					})
				rwc.AddComment(warnMissingDisplayName)
				return rwc
			}(),
		},
		{
			name: "missing guid",
			sa: &btpcli.Subaccount{
				DisplayName: displayName,
				Region:      region,
				Subdomain:   subdomain,
			},
			admins: admins,
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1alpha1.Subaccount{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1alpha1.SubaccountKind,
							APIVersion: v1alpha1.CRDGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: wantResourceName,
							Annotations: map[string]string{
								"crossplane.io/external-name": "",
							},
						},
						Spec: v1alpha1.SubaccountSpec{
							ClusterManagedResourceSpec: v1.ClusterManagedResourceSpec{
								ManagementPolicies: []v1.ManagementAction{v1.ManagementActionObserve},
							},
							ForProvider: v1alpha1.SubaccountParameters{
								DisplayName:      displayName,
								Region:           region,
								Subdomain:        subdomain,
								SubaccountAdmins: admins,
							},
						},
					})
				rwc.AddComment(warnMissingGuid)
				return rwc
			}(),
		},
		{
			name: "missing region",
			sa: &btpcli.Subaccount{
				DisplayName: displayName,
				GUID:        saGuid,
				Subdomain:   subdomain,
			},
			admins: admins,
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1alpha1.Subaccount{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1alpha1.SubaccountKind,
							APIVersion: v1alpha1.CRDGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resources.UndefinedName,
							Annotations: map[string]string{
								"crossplane.io/external-name": saGuid,
							},
						},
						Spec: v1alpha1.SubaccountSpec{
							ClusterManagedResourceSpec: v1.ClusterManagedResourceSpec{
								ManagementPolicies: []v1.ManagementAction{v1.ManagementActionObserve},
							},
							ForProvider: v1alpha1.SubaccountParameters{
								DisplayName:      displayName,
								Subdomain:        subdomain,
								SubaccountAdmins: admins,
							},
						},
					})
				rwc.AddComment(warnMissingRegion)
				return rwc
			}(),
		},
		{
			name: "missing subdomain",
			sa: &btpcli.Subaccount{
				DisplayName: displayName,
				GUID:        saGuid,
				Region:      region,
			},
			admins: admins,
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1alpha1.Subaccount{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1alpha1.SubaccountKind,
							APIVersion: v1alpha1.CRDGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: wantResourceName,
							Annotations: map[string]string{
								"crossplane.io/external-name": saGuid,
							},
						},
						Spec: v1alpha1.SubaccountSpec{
							ClusterManagedResourceSpec: v1.ClusterManagedResourceSpec{
								ManagementPolicies: []v1.ManagementAction{v1.ManagementActionObserve},
							},
							ForProvider: v1alpha1.SubaccountParameters{
								DisplayName:      displayName,
								Region:           region,
								SubaccountAdmins: admins,
							},
						},
					})
				rwc.AddComment(warnMissingSubdomain)
				return rwc
			}(),
		},
		{
			name: "missing admins",
			sa: &btpcli.Subaccount{
				DisplayName: displayName,
				GUID:        saGuid,
				Region:      region,
				Subdomain:   subdomain,
			},
			admins: nil,
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1alpha1.Subaccount{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1alpha1.SubaccountKind,
							APIVersion: v1alpha1.CRDGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: wantResourceName,
							Annotations: map[string]string{
								"crossplane.io/external-name": saGuid,
							},
						},
						Spec: v1alpha1.SubaccountSpec{
							ClusterManagedResourceSpec: v1.ClusterManagedResourceSpec{
								ManagementPolicies: []v1.ManagementAction{v1.ManagementActionObserve},
							},
							ForProvider: v1alpha1.SubaccountParameters{
								DisplayName:      displayName,
								Region:           region,
								Subdomain:        subdomain,
								SubaccountAdmins: []string{},
							},
						},
					})
				rwc.AddComment(warnMissingAdmins)
				return rwc
			}(),
		},
		{
			name: "parentGUID same as globalAccountGUID",
			sa: &btpcli.Subaccount{
				DisplayName:       displayName,
				GUID:              saGuid,
				Region:            region,
				Subdomain:         subdomain,
				GlobalAccountGUID: gaGuid,
				ParentGUID:        gaGuid,
			},
			admins: admins,
			want: yaml.NewResourceWithComment(
				&v1alpha1.Subaccount{
					TypeMeta: metav1.TypeMeta{
						Kind:       v1alpha1.SubaccountKind,
						APIVersion: v1alpha1.CRDGroupVersion.String(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: wantResourceName,
						Annotations: map[string]string{
							"crossplane.io/external-name": saGuid,
						},
					},
					Spec: v1alpha1.SubaccountSpec{
						ClusterManagedResourceSpec: v1.ClusterManagedResourceSpec{
							ManagementPolicies: []v1.ManagementAction{v1.ManagementActionObserve},
						},
						ForProvider: v1alpha1.SubaccountParameters{
							DisplayName:       displayName,
							Region:            region,
							Subdomain:         subdomain,
							SubaccountAdmins:  admins,
							GlobalAccountGuid: gaGuid,
						},
					},
				}),
		},
		{
			name: "special characters in displayName",
			sa: &btpcli.Subaccount{
				DisplayName: displayNameSpecial,
				GUID:        saGuid,
				Region:      region,
				Subdomain:   subdomain,
			},
			admins: admins,
			want: yaml.NewResourceWithComment(
				&v1alpha1.Subaccount{
					TypeMeta: metav1.TypeMeta{
						Kind:       v1alpha1.SubaccountKind,
						APIVersion: v1alpha1.CRDGroupVersion.String(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-subaccount-with-spaces---special--at-x.eu10",
						Annotations: map[string]string{
							"crossplane.io/external-name": saGuid,
						},
					},
					Spec: v1alpha1.SubaccountSpec{
						ClusterManagedResourceSpec: v1.ClusterManagedResourceSpec{
							ManagementPolicies: []v1.ManagementAction{v1.ManagementActionObserve},
						},
						ForProvider: v1alpha1.SubaccountParameters{
							DisplayName:      displayNameSpecial,
							Region:           region,
							Subdomain:        subdomain,
							SubaccountAdmins: admins,
						},
					},
				}),
		},
		{
			name: "empty string fields",
			sa: &btpcli.Subaccount{
				DisplayName: empty,
				GUID:        empty,
				Region:      empty,
				Subdomain:   empty,
			},
			admins: nil,
			want: func() *yaml.ResourceWithComment {
				rwc := yaml.NewResourceWithComment(
					&v1alpha1.Subaccount{
						TypeMeta: metav1.TypeMeta{
							Kind:       v1alpha1.SubaccountKind,
							APIVersion: v1alpha1.CRDGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: resources.UndefinedName,
							Annotations: map[string]string{
								"crossplane.io/external-name": "",
							},
						},
						Spec: v1alpha1.SubaccountSpec{
							ClusterManagedResourceSpec: v1.ClusterManagedResourceSpec{
								ManagementPolicies: []v1.ManagementAction{v1.ManagementActionObserve},
							},
							ForProvider: v1alpha1.SubaccountParameters{
								SubaccountAdmins: []string{},
							},
						},
					})
				rwc.AddComment(warnMissingDisplayName)
				rwc.AddComment(warnMissingGuid)
				rwc.AddComment(warnMissingRegion)
				rwc.AddComment(warnMissingSubdomain)
				rwc.AddComment(warnMissingAdmins)
				return rwc
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertSubaccountResource(&subaccount{
				Subaccount:          tt.sa,
				ResourceWithComment: yaml.NewResourceWithComment(nil),
				Admins:              tt.admins,
			})
			r.NotNil(result)

			// Verify comments.
			gotComment, gotHasComment := result.Comment()
			wantComment, wantHasComment := tt.want.Comment()
			r.Equal(wantHasComment, gotHasComment, "comment presence mismatch")
			if wantHasComment {
				r.Equal(wantComment, gotComment, "comment mismatch")
			}

			// Verify resource type.
			gotSa, gotOk := result.Resource().(*v1alpha1.Subaccount)
			wantSa, wantOk := tt.want.Resource().(*v1alpha1.Subaccount)
			r.True(gotOk && wantOk, "expected resource type to be *v1alpha1.Subaccount")

			// Verify metadata.
			r.Equal(wantSa.Kind, gotSa.Kind)
			r.Equal(wantSa.APIVersion, gotSa.APIVersion)
			r.Equal(wantSa.Name, gotSa.Name)
			r.Equal(wantSa.Annotations["crossplane.io/external-name"], gotSa.Annotations["crossplane.io/external-name"])

			// Verify ManagementPolicies.
			r.Equal(1, len(gotSa.Spec.ManagementPolicies))
			r.Equal(v1.ManagementActionObserve, gotSa.Spec.ManagementPolicies[0])
			r.Equal(wantSa.Spec.ManagementPolicies, gotSa.Spec.ManagementPolicies)

			// Verify providerConfigRef.
			r.Nil(gotSa.GetProviderConfigReference(), "providerConfigRef must not be set")

			// Verify required fields.
			r.Equal(wantSa.Spec.ForProvider.DisplayName, gotSa.Spec.ForProvider.DisplayName)
			r.Equal(wantSa.Spec.ForProvider.Region, gotSa.Spec.ForProvider.Region)
			r.Equal(wantSa.Spec.ForProvider.Subdomain, gotSa.Spec.ForProvider.Subdomain)
			r.NotNil(gotSa.Spec.ForProvider.SubaccountAdmins, "SubaccountAdmins must not be nil")
			r.Equal(wantSa.Spec.ForProvider.SubaccountAdmins, gotSa.Spec.ForProvider.SubaccountAdmins)

			// Verify optional fields.
			r.Equal(wantSa.Spec.ForProvider.GlobalAccountGuid, gotSa.Spec.ForProvider.GlobalAccountGuid)
			r.Equal(wantSa.Spec.ForProvider.DirectoryGuid, gotSa.Spec.ForProvider.DirectoryGuid)
			r.Equal(wantSa.Spec.ForProvider.Description, gotSa.Spec.ForProvider.Description)
			r.Equal(wantSa.Spec.ForProvider.UsedForProduction, gotSa.Spec.ForProvider.UsedForProduction)
			r.Equal(wantSa.Spec.ForProvider.BetaEnabled, gotSa.Spec.ForProvider.BetaEnabled)
			r.Equal(wantSa.Spec.ForProvider.Labels, gotSa.Spec.ForProvider.Labels)

			// Final overall comparison.
			r.Equal(wantSa, gotSa)
		})
	}
}
