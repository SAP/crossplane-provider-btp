package subaccount

import (
	"context"
	"net/http"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
	accountclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-accounts-service-api-go/pkg"
	"github.com/sap/crossplane-provider-btp/internal/testutils"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
	trackingtest "github.com/sap/crossplane-provider-btp/internal/tracking/test"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	client "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sap/crossplane-provider-btp/btp"
)

func TestObserve(t *testing.T) {
	type args struct {
		cr            resource.Managed
		mockAPIClient *MockSubaccountClient
		mockKube      test.MockClient
	}
	type want struct {
		err       error
		o         managed.ExternalObservation
		crChanges func(cr *v1alpha1.Subaccount)
	}
	tests := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"NilResource": {
			reason: "Expect error if used with another resource type",
			args: args{
				cr:            nil,
				mockAPIClient: &MockSubaccountClient{},
			},
			want: want{
				err: errors.New(errNotSubaccount),
			},
		},
		"NeedsCreation": {
			reason: "Empty status indicates not found",
			args: args{
				cr: NewSubaccount("unittest-sa"),
				mockAPIClient: &MockSubaccountClient{
					returnSubaccounts: &accountclient.ResponseCollection{Value: []accountclient.SubaccountResponseObject{}},
				},
			},
			want: want{
				o: managed.ExternalObservation{ResourceExists: false},
			},
		},
		"FindSubaccountError": {
			reason: "Get Subaccount error should reset remote state",
			args: args{
				cr: NewSubaccount("unittest-sa"),
				mockAPIClient: &MockSubaccountClient{
					returnErr: errors.New("Error getting subaccount"),
				},
			},
			want: want{
				o:   managed.ExternalObservation{ResourceExists: false},
				err: errors.New("Error getting subaccount"),
			},
		},
		"DontUpdateEmptyDescription": {
			reason: "Empty description should NOT require Update",
			args: args{
				cr: NewSubaccount("unittest-sa", WithData(v1alpha1.SubaccountParameters{
					Subdomain:         "sub1",
					Region:            "eu12",
					DisplayName:       "unittest-sa",
					UsedForProduction: "",
					BetaEnabled:       false,
				}), WithProviderConfig(xpv1.Reference{
					Name: "unittest-pc",
				})),
				mockAPIClient: &MockSubaccountClient{
					returnSubaccounts: &accountclient.ResponseCollection{
						Value: []accountclient.SubaccountResponseObject{
							{
								Guid:              "123",
								Description:       "",
								Subdomain:         "sub1",
								Region:            "eu12",
								State:             "OK",
								Labels:            &map[string][]string{},
								StateMessage:      internal.Ptr("OK"),
								DisplayName:       "unittest-sa",
								UsedForProduction: "",
								BetaEnabled:       false,
							},
						},
					},
				},
				mockKube: testutils.NewFakeKubeClientBuilder().
					AddResources(testutils.NewProviderConfig("unittest-pc", "", "")).
					Build(),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  true,
					ConnectionDetails: managed.ConnectionDetails{},
				},
				crChanges: func(cr *v1alpha1.Subaccount) {
					cr.Status.AtProvider.SubaccountGuid = internal.Ptr("123")
					cr.Status.AtProvider.Status = internal.Ptr("OK")
					cr.Status.AtProvider.Region = internal.Ptr("eu12")
					cr.Status.AtProvider.Subdomain = internal.Ptr("sub1")
					cr.Status.AtProvider.Labels = &map[string][]string{}
					cr.Status.AtProvider.Description = internal.Ptr("")
					cr.Status.AtProvider.StatusMessage = internal.Ptr("OK")
					cr.Status.AtProvider.DisplayName = internal.Ptr("unittest-sa")
					cr.Status.AtProvider.UsedForProduction = internal.Ptr("")
					cr.Status.AtProvider.BetaEnabled = internal.Ptr(false)
					cr.Status.AtProvider.ParentGuid = internal.Ptr("")
					cr.Status.AtProvider.GlobalAccountGUID = internal.Ptr("")
				},
			},
		},
		"NeedsUpdateDescription": {
			reason: "Changed description should require Update",
			args: args{
				cr: NewSubaccount("unittest-sa", WithData(v1alpha1.SubaccountParameters{
					Description:       "someDesc",
					Subdomain:         "sub1",
					Region:            "eu12",
					DisplayName:       "unittest-sa",
					UsedForProduction: "",
					BetaEnabled:       false,
				}), WithProviderConfig(xpv1.Reference{
					Name: "unittest-pc",
				})),
				mockAPIClient: &MockSubaccountClient{
					returnSubaccounts: &accountclient.ResponseCollection{
						Value: []accountclient.SubaccountResponseObject{
							{
								Guid:              "123",
								Description:       "anotherDesc",
								Subdomain:         "sub1",
								Region:            "eu12",
								State:             "OK",
								Labels:            &map[string][]string{},
								StateMessage:      internal.Ptr("OK"),
								DisplayName:       "unittest-sa",
								UsedForProduction: "",
								BetaEnabled:       false,
							},
						},
					},
				},
				mockKube: testutils.NewFakeKubeClientBuilder().
					AddResources(testutils.NewProviderConfig("unittest-pc", "", "")).
					Build(),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  false,
					ConnectionDetails: managed.ConnectionDetails{},
				},
				crChanges: func(cr *v1alpha1.Subaccount) {
					cr.Status.AtProvider.SubaccountGuid = internal.Ptr("123")
					cr.Status.AtProvider.Status = internal.Ptr("OK")
					cr.Status.AtProvider.Region = internal.Ptr("eu12")
					cr.Status.AtProvider.Subdomain = internal.Ptr("sub1")
					cr.Status.AtProvider.Labels = &map[string][]string{}
					cr.Status.AtProvider.Description = internal.Ptr("anotherDesc")
					cr.Status.AtProvider.StatusMessage = internal.Ptr("OK")
					cr.Status.AtProvider.DisplayName = internal.Ptr("unittest-sa")
					cr.Status.AtProvider.UsedForProduction = internal.Ptr("")
					cr.Status.AtProvider.BetaEnabled = internal.Ptr(false)
					cr.Status.AtProvider.ParentGuid = internal.Ptr("")
					cr.Status.AtProvider.GlobalAccountGUID = internal.Ptr("")
				},
			},
		},
		"NeedsUpdateDisplayName": {
			reason: "Changed display name should require Update",
			args: args{
				cr: NewSubaccount("unittest-sa", WithData(v1alpha1.SubaccountParameters{
					Description:       "someDesc",
					Subdomain:         "sub1",
					Region:            "eu12",
					DisplayName:       "unittest-sa",
					UsedForProduction: "",
					BetaEnabled:       false,
				}), WithProviderConfig(xpv1.Reference{
					Name: "unittest-pc",
				})),
				mockAPIClient: &MockSubaccountClient{
					returnSubaccounts: &accountclient.ResponseCollection{
						Value: []accountclient.SubaccountResponseObject{
							{
								Guid:              "123",
								Description:       "someDesc",
								Subdomain:         "sub1",
								Region:            "eu12",
								State:             "OK",
								Labels:            &map[string][]string{},
								StateMessage:      internal.Ptr("OK"),
								DisplayName:       "changed-unittest-sa",
								UsedForProduction: "",
								BetaEnabled:       false,
							},
						},
					},
				},
				mockKube: testutils.NewFakeKubeClientBuilder().
					AddResources(testutils.NewProviderConfig("unittest-pc", "", "")).
					Build(),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  false,
					ConnectionDetails: managed.ConnectionDetails{},
				},
				crChanges: func(cr *v1alpha1.Subaccount) {
					cr.Status.AtProvider.SubaccountGuid = internal.Ptr("123")
					cr.Status.AtProvider.Status = internal.Ptr("OK")
					cr.Status.AtProvider.Region = internal.Ptr("eu12")
					cr.Status.AtProvider.Subdomain = internal.Ptr("sub1")
					cr.Status.AtProvider.Labels = &map[string][]string{}
					cr.Status.AtProvider.Description = internal.Ptr("someDesc")
					cr.Status.AtProvider.StatusMessage = internal.Ptr("OK")
					cr.Status.AtProvider.DisplayName = internal.Ptr("changed-unittest-sa")
					cr.Status.AtProvider.UsedForProduction = internal.Ptr("")
					cr.Status.AtProvider.BetaEnabled = internal.Ptr(false)
					cr.Status.AtProvider.ParentGuid = internal.Ptr("")
					cr.Status.AtProvider.GlobalAccountGUID = internal.Ptr("")
				},
			},
		},
		"NeedsUpdateBetaEnabled": {
			reason: "Changed beta enabled toggle should require Update",
			args: args{
				cr: NewSubaccount("unittest-sa", WithData(v1alpha1.SubaccountParameters{
					Description:       "someDesc",
					Subdomain:         "sub1",
					Region:            "eu12",
					DisplayName:       "unittest-sa",
					UsedForProduction: "",
					BetaEnabled:       false,
				}), WithProviderConfig(xpv1.Reference{
					Name: "unittest-pc",
				})),
				mockAPIClient: &MockSubaccountClient{
					returnSubaccounts: &accountclient.ResponseCollection{
						Value: []accountclient.SubaccountResponseObject{
							{
								Guid:              "123",
								Description:       "someDesc",
								Subdomain:         "sub1",
								Region:            "eu12",
								State:             "OK",
								Labels:            &map[string][]string{},
								StateMessage:      internal.Ptr("OK"),
								DisplayName:       "unittest-sa",
								UsedForProduction: "",
								BetaEnabled:       true,
							},
						},
					},
				},
				mockKube: testutils.NewFakeKubeClientBuilder().
					AddResources(testutils.NewProviderConfig("unittest-pc", "", "")).
					Build(),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  false,
					ConnectionDetails: managed.ConnectionDetails{},
				},
				crChanges: func(cr *v1alpha1.Subaccount) {
					cr.Status.AtProvider.SubaccountGuid = internal.Ptr("123")
					cr.Status.AtProvider.Status = internal.Ptr("OK")
					cr.Status.AtProvider.Region = internal.Ptr("eu12")
					cr.Status.AtProvider.Subdomain = internal.Ptr("sub1")
					cr.Status.AtProvider.Labels = &map[string][]string{}
					cr.Status.AtProvider.Description = internal.Ptr("someDesc")
					cr.Status.AtProvider.StatusMessage = internal.Ptr("OK")
					cr.Status.AtProvider.DisplayName = internal.Ptr("unittest-sa")
					cr.Status.AtProvider.UsedForProduction = internal.Ptr("")
					cr.Status.AtProvider.BetaEnabled = internal.Ptr(true)
					cr.Status.AtProvider.ParentGuid = internal.Ptr("")
					cr.Status.AtProvider.GlobalAccountGUID = internal.Ptr("")
				},
			},
		},
		"NeedsUpdateUsedForProduction": {
			reason: "Changed UsedForProduction toggle should require Update",
			args: args{
				cr: NewSubaccount("unittest-sa", WithData(v1alpha1.SubaccountParameters{
					Description:       "someDesc",
					Subdomain:         "sub1",
					Region:            "eu12",
					DisplayName:       "unittest-sa",
					UsedForProduction: "NOT_USED_FOR_PRODUCTION",
					BetaEnabled:       true,
				}), WithProviderConfig(xpv1.Reference{
					Name: "unittest-pc",
				})),
				mockAPIClient: &MockSubaccountClient{
					returnSubaccounts: &accountclient.ResponseCollection{
						Value: []accountclient.SubaccountResponseObject{
							{
								Guid:              "123",
								Description:       "someDesc",
								Subdomain:         "sub1",
								Region:            "eu12",
								State:             "OK",
								Labels:            &map[string][]string{},
								StateMessage:      internal.Ptr("OK"),
								DisplayName:       "unittest-sa",
								UsedForProduction: "USED_FOR_PRODUCTION",
								BetaEnabled:       true,
							},
						},
					},
				},
				mockKube: testutils.NewFakeKubeClientBuilder().
					AddResources(testutils.NewProviderConfig("unittest-pc", "", "")).
					Build(),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  false,
					ConnectionDetails: managed.ConnectionDetails{},
				},
				crChanges: func(cr *v1alpha1.Subaccount) {
					cr.Status.AtProvider.SubaccountGuid = internal.Ptr("123")
					cr.Status.AtProvider.Status = internal.Ptr("OK")
					cr.Status.AtProvider.Region = internal.Ptr("eu12")
					cr.Status.AtProvider.Subdomain = internal.Ptr("sub1")
					cr.Status.AtProvider.Labels = &map[string][]string{}
					cr.Status.AtProvider.Description = internal.Ptr("someDesc")
					cr.Status.AtProvider.StatusMessage = internal.Ptr("OK")
					cr.Status.AtProvider.DisplayName = internal.Ptr("unittest-sa")
					cr.Status.AtProvider.UsedForProduction = internal.Ptr("USED_FOR_PRODUCTION")
					cr.Status.AtProvider.BetaEnabled = internal.Ptr(true)
					cr.Status.AtProvider.ParentGuid = internal.Ptr("")
					cr.Status.AtProvider.GlobalAccountGUID = internal.Ptr("")
				},
			},
		},
		"NeedsUpdateBetweenDirectories": {
			reason: "Changed Directory GUID should require Update",
			args: args{
				cr: NewSubaccount("unittest-sa", WithData(v1alpha1.SubaccountParameters{
					Description:       "someDesc",
					Subdomain:         "sub1",
					Region:            "eu12",
					DisplayName:       "unittest-sa",
					UsedForProduction: "",
					BetaEnabled:       false,
					DirectoryGuid:     "234",
				}), WithProviderConfig(xpv1.Reference{
					Name: "unittest-pc",
				})),
				mockAPIClient: &MockSubaccountClient{returnSubaccounts: &accountclient.ResponseCollection{
					Value: []accountclient.SubaccountResponseObject{
						{
							Guid:              "123",
							Description:       "someDesc",
							Subdomain:         "sub1",
							Region:            "eu12",
							State:             "OK",
							DisplayName:       "unittest-sa",
							Labels:            &map[string][]string{},
							StateMessage:      internal.Ptr("OK"),
							UsedForProduction: "",
							BetaEnabled:       false,
							ParentGUID:        "345",
						},
					},
				}},
				mockKube: testutils.NewFakeKubeClientBuilder().
					AddResources(testutils.NewProviderConfig("unittest-pc", "", "")).
					Build(),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  false,
					ConnectionDetails: managed.ConnectionDetails{},
				},
				crChanges: func(cr *v1alpha1.Subaccount) {
					cr.Status.AtProvider.SubaccountGuid = internal.Ptr("123")
					cr.Status.AtProvider.Status = internal.Ptr("OK")
					cr.Status.AtProvider.Region = internal.Ptr("eu12")
					cr.Status.AtProvider.Subdomain = internal.Ptr("sub1")
					cr.Status.AtProvider.Labels = &map[string][]string{}
					cr.Status.AtProvider.Description = internal.Ptr("someDesc")
					cr.Status.AtProvider.StatusMessage = internal.Ptr("OK")
					cr.Status.AtProvider.DisplayName = internal.Ptr("unittest-sa")
					cr.Status.AtProvider.UsedForProduction = internal.Ptr("")
					cr.Status.AtProvider.BetaEnabled = internal.Ptr(false)
					cr.Status.AtProvider.ParentGuid = internal.Ptr("345")
					cr.Status.AtProvider.GlobalAccountGUID = internal.Ptr("")
				},
			},
		},
		"NeedsUpdateFromGlobalToDirectory": {
			reason: "Changed Directory GUID from global account needs update",
			args: args{
				cr: NewSubaccount("unittest-sa", WithData(v1alpha1.SubaccountParameters{
					Description:       "someDesc",
					Subdomain:         "sub1",
					Region:            "eu12",
					DisplayName:       "unittest-sa",
					UsedForProduction: "",
					BetaEnabled:       false,
					DirectoryGuid:     "234",
					DirectoryRef:      &xpv1.Reference{Name: "dir-1"},
				}), WithProviderConfig(xpv1.Reference{
					Name: "unittest-pc",
				})),
				mockAPIClient: &MockSubaccountClient{
					returnSubaccounts: &accountclient.ResponseCollection{
						Value: []accountclient.SubaccountResponseObject{
							{
								Guid:              "123",
								Description:       "someDesc",
								Subdomain:         "sub1",
								Region:            "eu12",
								State:             "OK",
								DisplayName:       "unittest-sa",
								Labels:            &map[string][]string{},
								StateMessage:      internal.Ptr("OK"),
								UsedForProduction: "",
								BetaEnabled:       false,
								ParentGUID:        "global-123",
								GlobalAccountGUID: "global-123",
							},
						},
					},
				},
				mockKube: testutils.NewFakeKubeClientBuilder().
					AddResources(testutils.NewProviderConfig("unittest-pc", "", "")).
					Build(),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  false,
					ConnectionDetails: managed.ConnectionDetails{},
				},
				crChanges: func(cr *v1alpha1.Subaccount) {
					cr.Status.AtProvider.SubaccountGuid = internal.Ptr("123")
					cr.Status.AtProvider.Status = internal.Ptr("OK")
					cr.Status.AtProvider.Region = internal.Ptr("eu12")
					cr.Status.AtProvider.Subdomain = internal.Ptr("sub1")
					cr.Status.AtProvider.Labels = &map[string][]string{}
					cr.Status.AtProvider.Description = internal.Ptr("someDesc")
					cr.Status.AtProvider.StatusMessage = internal.Ptr("OK")
					cr.Status.AtProvider.DisplayName = internal.Ptr("unittest-sa")
					cr.Status.AtProvider.UsedForProduction = internal.Ptr("")
					cr.Status.AtProvider.BetaEnabled = internal.Ptr(false)
					cr.Status.AtProvider.ParentGuid = internal.Ptr("global-123")
					cr.Status.AtProvider.GlobalAccountGUID = internal.Ptr("global-123")
				},
			},
		},
		"NeedsUpdateFromDirectoryToGlobal": {
			reason: "Changed Directory GUID directory to global",
			args: args{
				cr: NewSubaccount("unittest-sa", WithData(v1alpha1.SubaccountParameters{
					Description:       "someDesc",
					Subdomain:         "sub1",
					Region:            "eu12",
					DisplayName:       "unittest-sa",
					UsedForProduction: "",
					BetaEnabled:       false,
				}), WithProviderConfig(xpv1.Reference{
					Name: "unittest-pc",
				})),
				mockAPIClient: &MockSubaccountClient{
					returnSubaccounts: &accountclient.ResponseCollection{
						Value: []accountclient.SubaccountResponseObject{
							{
								Guid:              "123",
								Description:       "someDesc",
								Subdomain:         "sub1",
								Region:            "eu12",
								State:             "OK",
								DisplayName:       "unittest-sa",
								Labels:            &map[string][]string{},
								StateMessage:      internal.Ptr("OK"),
								UsedForProduction: "",
								BetaEnabled:       false,
								ParentGUID:        "456",
								GlobalAccountGUID: "global-123",
							},
						},
					},
				},
				mockKube: testutils.NewFakeKubeClientBuilder().
					AddResources(testutils.NewProviderConfig("unittest-pc", "", "")).
					Build(),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  false,
					ConnectionDetails: managed.ConnectionDetails{},
				},
				crChanges: func(cr *v1alpha1.Subaccount) {
					cr.Status.AtProvider.SubaccountGuid = internal.Ptr("123")
					cr.Status.AtProvider.Status = internal.Ptr("OK")
					cr.Status.AtProvider.Region = internal.Ptr("eu12")
					cr.Status.AtProvider.Subdomain = internal.Ptr("sub1")
					cr.Status.AtProvider.Labels = &map[string][]string{}
					cr.Status.AtProvider.Description = internal.Ptr("someDesc")
					cr.Status.AtProvider.StatusMessage = internal.Ptr("OK")
					cr.Status.AtProvider.DisplayName = internal.Ptr("unittest-sa")
					cr.Status.AtProvider.UsedForProduction = internal.Ptr("")
					cr.Status.AtProvider.BetaEnabled = internal.Ptr(false)
					cr.Status.AtProvider.ParentGuid = internal.Ptr("456")
					cr.Status.AtProvider.GlobalAccountGUID = internal.Ptr("global-123")
				},
			},
		},
		"UpToDateNoDirectory": {
			args: args{
				cr: NewSubaccount("unittest-sa", WithData(v1alpha1.SubaccountParameters{
					Description:       "someDesc",
					Subdomain:         "sub1",
					Region:            "eu12",
					DisplayName:       "unittest-sa",
					UsedForProduction: "",
					BetaEnabled:       false,
				}), WithProviderConfig(xpv1.Reference{
					Name: "unittest-pc",
				})),
				mockAPIClient: &MockSubaccountClient{
					returnSubaccounts: &accountclient.ResponseCollection{
						Value: []accountclient.SubaccountResponseObject{
							{
								Guid:              "123",
								Description:       "someDesc",
								Subdomain:         "sub1",
								Region:            "eu12",
								State:             "OK",
								DisplayName:       "unittest-sa",
								Labels:            &map[string][]string{},
								StateMessage:      internal.Ptr("OK"),
								UsedForProduction: "",
								BetaEnabled:       false,
								ParentGUID:        "global-123",
								GlobalAccountGUID: "global-123",
							},
						},
					},
				},

				mockKube: testutils.NewFakeKubeClientBuilder().
					AddResources(testutils.NewProviderConfig("unittest-pc", "", "")).
					Build(),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  true,
					ConnectionDetails: managed.ConnectionDetails{},
				},
				crChanges: func(cr *v1alpha1.Subaccount) {
					cr.Status.AtProvider.SubaccountGuid = internal.Ptr("123")
					cr.Status.AtProvider.Status = internal.Ptr("OK")
					cr.Status.AtProvider.Region = internal.Ptr("eu12")
					cr.Status.AtProvider.Subdomain = internal.Ptr("sub1")
					cr.Status.AtProvider.Labels = &map[string][]string{}
					cr.Status.AtProvider.Description = internal.Ptr("someDesc")
					cr.Status.AtProvider.StatusMessage = internal.Ptr("OK")
					cr.Status.AtProvider.DisplayName = internal.Ptr("unittest-sa")
					cr.Status.AtProvider.UsedForProduction = internal.Ptr("")
					cr.Status.AtProvider.BetaEnabled = internal.Ptr(false)
					cr.Status.AtProvider.ParentGuid = internal.Ptr("global-123")
					cr.Status.AtProvider.GlobalAccountGUID = internal.Ptr("global-123")

					cr.Status.SetConditions(xpv1.Available())
				},
			},
		},
		"UpToDateWithinDirectory": {
			args: args{
				cr: NewSubaccount("unittest-sa", WithData(v1alpha1.SubaccountParameters{
					Description:       "someDesc",
					Subdomain:         "sub1",
					Region:            "eu12",
					DisplayName:       "unittest-sa",
					UsedForProduction: "",
					BetaEnabled:       false,
					DirectoryGuid:     "234",
					DirectoryRef:      &xpv1.Reference{Name: "dir-1"},
				}), WithProviderConfig(xpv1.Reference{
					Name: "unittest-pc",
				})),
				mockAPIClient: &MockSubaccountClient{returnSubaccounts: &accountclient.ResponseCollection{
					Value: []accountclient.SubaccountResponseObject{
						{
							Guid:              "123",
							Description:       "someDesc",
							Subdomain:         "sub1",
							Region:            "eu12",
							State:             "OK",
							DisplayName:       "unittest-sa",
							Labels:            &map[string][]string{},
							StateMessage:      internal.Ptr("OK"),
							UsedForProduction: "",
							BetaEnabled:       false,
							ParentGUID:        "234",
						},
					},
				}},
				mockKube: testutils.NewFakeKubeClientBuilder().
					AddResources(testutils.NewProviderConfig("unittest-pc", "", "")).
					Build(),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  true,
					ConnectionDetails: managed.ConnectionDetails{},
				},
				crChanges: func(cr *v1alpha1.Subaccount) {
					cr.Status.AtProvider.SubaccountGuid = internal.Ptr("123")
					cr.Status.AtProvider.Status = internal.Ptr("OK")
					cr.Status.AtProvider.Region = internal.Ptr("eu12")
					cr.Status.AtProvider.Subdomain = internal.Ptr("sub1")
					cr.Status.AtProvider.Labels = &map[string][]string{}
					cr.Status.AtProvider.Description = internal.Ptr("someDesc")
					cr.Status.AtProvider.StatusMessage = internal.Ptr("OK")
					cr.Status.AtProvider.DisplayName = internal.Ptr("unittest-sa")
					cr.Status.AtProvider.UsedForProduction = internal.Ptr("")
					cr.Status.AtProvider.BetaEnabled = internal.Ptr(false)
					cr.Status.AtProvider.ParentGuid = internal.Ptr("234")
					cr.Status.AtProvider.GlobalAccountGUID = internal.Ptr("")

					cr.Status.SetConditions(xpv1.Available())
				},
			},
		},
		"UpToDateWithDirectoryGUID": {
			reason: "Directly referencing a directory via GUID should also work (without name ref)",
			args: args{
				cr: NewSubaccount("unittest-sa", WithData(v1alpha1.SubaccountParameters{
					Description:       "someDesc",
					Subdomain:         "sub1",
					Region:            "eu12",
					DisplayName:       "unittest-sa",
					UsedForProduction: "",
					BetaEnabled:       false,
					DirectoryGuid:     "234",
				}), WithProviderConfig(xpv1.Reference{
					Name: "unittest-pc",
				})),
				mockAPIClient: &MockSubaccountClient{returnSubaccounts: &accountclient.ResponseCollection{
					Value: []accountclient.SubaccountResponseObject{
						{
							Guid:              "123",
							Description:       "someDesc",
							Subdomain:         "sub1",
							Region:            "eu12",
							State:             "OK",
							DisplayName:       "unittest-sa",
							Labels:            &map[string][]string{},
							StateMessage:      internal.Ptr("OK"),
							UsedForProduction: "",
							BetaEnabled:       false,
							ParentGUID:        "234",
							GlobalAccountGUID: "123",
						},
					},
				}},
				mockKube: testutils.NewFakeKubeClientBuilder().
					AddResources(testutils.NewProviderConfig("unittest-pc", "", "")).
					Build(),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  true,
					ConnectionDetails: managed.ConnectionDetails{},
				},
				crChanges: func(cr *v1alpha1.Subaccount) {
					cr.Status.AtProvider.SubaccountGuid = internal.Ptr("123")
					cr.Status.AtProvider.Status = internal.Ptr("OK")
					cr.Status.AtProvider.Region = internal.Ptr("eu12")
					cr.Status.AtProvider.Subdomain = internal.Ptr("sub1")
					cr.Status.AtProvider.Labels = &map[string][]string{}
					cr.Status.AtProvider.Description = internal.Ptr("someDesc")
					cr.Status.AtProvider.StatusMessage = internal.Ptr("OK")
					cr.Status.AtProvider.DisplayName = internal.Ptr("unittest-sa")
					cr.Status.AtProvider.UsedForProduction = internal.Ptr("")
					cr.Status.AtProvider.BetaEnabled = internal.Ptr(false)
					cr.Status.AtProvider.ParentGuid = internal.Ptr("234")
					cr.Status.AtProvider.GlobalAccountGUID = internal.Ptr("123")

					cr.Status.SetConditions(xpv1.Available())
				},
			},
		},
		"UpToDateDespiteDifferentLabelNilTypes": {
			reason: "Labels pointer type mismatch should not lead to unexpected comparison results",
			args: args{
				cr: NewSubaccount("unittest-sa", WithData(v1alpha1.SubaccountParameters{
					Description:       "someDesc",
					Subdomain:         "sub1",
					Region:            "eu12",
					DisplayName:       "unittest-sa",
					UsedForProduction: "",
					BetaEnabled:       false,
					DirectoryGuid:     "234",
					Labels:            nil,
				}), WithProviderConfig(xpv1.Reference{
					Name: "unittest-pc",
				})),
				mockAPIClient: &MockSubaccountClient{returnSubaccounts: &accountclient.ResponseCollection{
					Value: []accountclient.SubaccountResponseObject{
						{
							Guid:              "123",
							Description:       "someDesc",
							Subdomain:         "sub1",
							Region:            "eu12",
							State:             "OK",
							DisplayName:       "unittest-sa",
							Labels:            nil,
							StateMessage:      internal.Ptr("OK"),
							UsedForProduction: "",
							BetaEnabled:       false,
							ParentGUID:        "234",
						},
					},
				}},
				mockKube: testutils.NewFakeKubeClientBuilder().
					AddResources(testutils.NewProviderConfig("unittest-pc", "", "")).
					Build(),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  true,
					ConnectionDetails: managed.ConnectionDetails{},
				},
				crChanges: func(cr *v1alpha1.Subaccount) {
					cr.Status.AtProvider.SubaccountGuid = internal.Ptr("123")
					cr.Status.AtProvider.Status = internal.Ptr("OK")
					cr.Status.AtProvider.Region = internal.Ptr("eu12")
					cr.Status.AtProvider.Subdomain = internal.Ptr("sub1")
					cr.Status.AtProvider.Labels = nil
					cr.Status.AtProvider.Description = internal.Ptr("someDesc")
					cr.Status.AtProvider.StatusMessage = internal.Ptr("OK")
					cr.Status.AtProvider.DisplayName = internal.Ptr("unittest-sa")
					cr.Status.AtProvider.UsedForProduction = internal.Ptr("")
					cr.Status.AtProvider.BetaEnabled = internal.Ptr(false)
					cr.Status.AtProvider.ParentGuid = internal.Ptr("234")
					cr.Status.AtProvider.GlobalAccountGUID = internal.Ptr("")

					cr.Status.SetConditions(xpv1.Available())
				},
			},
		},
		"NeedsUpdateLabel": {
			reason: "Adding label to an existing subaacount should require Update",
			args: args{
				cr: NewSubaccount("unittest-sa", WithData(v1alpha1.SubaccountParameters{
					Description:       "someDesc",
					Subdomain:         "sub1",
					Region:            "eu12",
					DisplayName:       "unittest-sa",
					Labels:            map[string][]string{"somekey": {"somevalue"}},
					UsedForProduction: "",
					BetaEnabled:       false,
				}), WithProviderConfig(xpv1.Reference{
					Name: "unittest-pc",
				})),
				mockAPIClient: &MockSubaccountClient{
					returnSubaccounts: &accountclient.ResponseCollection{
						Value: []accountclient.SubaccountResponseObject{
							{
								Guid:              "123",
								Description:       "someDesc",
								Subdomain:         "sub1",
								Region:            "eu12",
								State:             "OK",
								Labels:            nil,
								StateMessage:      internal.Ptr("OK"),
								DisplayName:       "unittest-sa",
								UsedForProduction: "",
								BetaEnabled:       false,
							},
						},
					},
				},
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  false,
					ConnectionDetails: managed.ConnectionDetails{},
				},
				crChanges: func(cr *v1alpha1.Subaccount) {
					cr.Status.AtProvider.SubaccountGuid = internal.Ptr("123")
					cr.Status.AtProvider.Status = internal.Ptr("OK")
					cr.Status.AtProvider.Region = internal.Ptr("eu12")
					cr.Status.AtProvider.Subdomain = internal.Ptr("sub1")
					cr.Status.AtProvider.Labels = nil
					cr.Status.AtProvider.Description = internal.Ptr("someDesc")
					cr.Status.AtProvider.StatusMessage = internal.Ptr("OK")
					cr.Status.AtProvider.DisplayName = internal.Ptr("unittest-sa")
					cr.Status.AtProvider.UsedForProduction = internal.Ptr("")
					cr.Status.AtProvider.BetaEnabled = internal.Ptr(false)
					cr.Status.AtProvider.ParentGuid = internal.Ptr("")
					cr.Status.AtProvider.GlobalAccountGUID = internal.Ptr("")
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := external{
				Client:  &tc.args.mockKube,
				tracker: trackingtest.NoOpReferenceResolverTracker{},
				btp: btp.Client{
					AccountsServiceClient: &accountclient.APIClient{
						SubaccountOperationsAPI: tc.args.mockAPIClient,
					},
				},
			}

			got, err := ctrl.Observe(context.Background(), tc.args.cr)
			if contained := testutils.ContainsError(err, tc.want.err); !contained {
				t.Errorf("\ne.Create(...): error \"%v\" not part of \"%v\"", err, tc.want.err)
			}
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\n%s\ne.Observe(...): -want, +got:\n%s\n", tc.reason, diff)
			}

			if tc.args.cr != nil {
				crCopy := tc.args.cr.DeepCopyObject()
				if tc.want.crChanges != nil {
					tc.want.crChanges(crCopy.(*v1alpha1.Subaccount))
				}
				if diff := cmp.Diff(crCopy, tc.args.cr); diff != "" {
					t.Errorf("\n%s\ne.Observe(...): -want cr, +got cr:\n%s\n", tc.reason, diff)
				}
			}
		})
	}
}

func TestCreate(t *testing.T) {
	type args struct {
		cr         resource.Managed
		mockClient *MockSubaccountClient
	}
	type want struct {
		err error
		o   managed.ExternalCreation
		cr  resource.Managed
	}
	tests := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"NilResource": {
			reason: "Expect error if used with another resource type",
			args: args{
				cr: nil,
			},
			want: want{
				err: errors.New(errNotSubaccount),
			},
		},
		"RunningCreation": {
			reason: "Return Gracefully if creation is already triggered",
			args: args{
				cr: NewSubaccount("unittest-sa", WithStatus(v1alpha1.SubaccountObservation{Status: internal.Ptr("STARTED")})),
			},
			want: want{
				cr: NewSubaccount("unittest-sa", WithStatus(v1alpha1.SubaccountObservation{Status: internal.Ptr("STARTED")})),
				o:  managed.ExternalCreation{},
			},
		},
		"APIErrorBadRequest": {
			reason: "API Error should be prevent creation",
			args: args{
				cr: NewSubaccount("unittest-sa"),
				mockClient: &MockSubaccountClient{
					returnSubaccount: &accountclient.SubaccountResponseObject{},
					returnErr:        errors.New("badRequestError"),
				},
			},
			want: want{
				cr:  NewSubaccount("unittest-sa"),
				o:   managed.ExternalCreation{},
				err: errors.New("badRequestError"),
			},
		},
		"CreateSuccess": {
			reason: "We should cache status in case everything worked out",
			args: args{
				cr: NewSubaccount("unittest-sa"),
				mockClient: &MockSubaccountClient{
					returnSubaccount: &accountclient.SubaccountResponseObject{
						Guid:         "123",
						StateMessage: internal.Ptr("Success"),
					},
				},
			},
			want: want{
				cr: NewSubaccount("unittest-sa", WithStatus(v1alpha1.SubaccountObservation{
					SubaccountGuid: internal.Ptr("123"),
					Status:         internal.Ptr("Success"),
					ParentGuid:     internal.Ptr(""),
				}),
					WithConditions(xpv1.Creating())),
				o: managed.ExternalCreation{ConnectionDetails: managed.ConnectionDetails{}},
			},
		},
		"MapDirectoryGuid": {
			reason: "DirectoryID needs to be passed as payload to API and saved in Status",
			args: args{
				cr: NewSubaccount("unittest-sa", WithData(v1alpha1.SubaccountParameters{DirectoryGuid: "234"})),
				mockClient: &MockSubaccountClient{
					returnSubaccount: &accountclient.SubaccountResponseObject{
						Guid:         "123",
						StateMessage: internal.Ptr("Success"),
						ParentGUID:   "234",
					},
				},
			},
			want: want{
				cr: NewSubaccount("unittest-sa",
					WithStatus(v1alpha1.SubaccountObservation{
						SubaccountGuid: internal.Ptr("123"),
						Status:         internal.Ptr("Success"),
						ParentGuid:     internal.Ptr("234"),
					}),
					WithConditions(xpv1.Creating()),
					WithData(v1alpha1.SubaccountParameters{DirectoryGuid: "234"}),
				),
				o: managed.ExternalCreation{ConnectionDetails: managed.ConnectionDetails{}},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := external{
				btp: btp.Client{
					AccountsServiceClient: &accountclient.APIClient{
						SubaccountOperationsAPI: tc.args.mockClient,
					},
				},
			}
			got, err := ctrl.Create(context.Background(), tc.args.cr)
			if contained := testutils.ContainsError(err, tc.want.err); !contained {
				t.Errorf("\ne.Create(...): error \"%v\" not part of \"%v\"", err, tc.want.err)
			}
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\n%s\ne.Create(...): -want, +got:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.cr, tc.args.cr); diff != "" {
				t.Errorf("\n%s\ne.Create(...): -want cr, +got cr:\n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestConnect(t *testing.T) {
	type args struct {
		cr              resource.Managed
		kubeObjects     []client.Object
		serviceFnErr    error
		serviceFnReturn *btp.Client
	}
	type newServiceArgs struct {
		cisCreds []byte
		saCreds  []byte
	}
	type want struct {
		err            error
		newServiceArgs newServiceArgs
	}

	tests := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"NilResource": {
			reason: "Expect error if used with another resource type",
			args: args{
				cr:          nil,
				kubeObjects: []client.Object{},
			},
			want: want{
				err: errors.New(errNotSubaccount),
			},
		},
		"NoProviderConfig": {
			reason: "Expect error if no provider config is set",
			args: args{
				cr:          NewSubaccount("unittest-sa", WithProviderConfig(xpv1.Reference{Name: "unittest-pc"})),
				kubeObjects: []client.Object{},
			},
			want: want{
				err: errors.New("cannot get ProviderConfig"),
			},
		},
		"NoCISCredentials": {
			reason: "Expect error if no CIS credentials are set",
			args: args{
				cr: NewSubaccount("unittest-sa", WithProviderConfig(xpv1.Reference{Name: "unittest-pc"})),
				kubeObjects: []client.Object{
					testutils.NewProviderConfig("unittest-pc", "cis-provider-secret", "sa-provider-secret"),
				},
			},
			want: want{
				err: errors.New("cannot get CIS credentials"),
			},
		},
		"NoSACredentials": {
			reason: "Expect error if no Service Account credentials are set",
			args: args{
				cr: NewSubaccount("unittest-sa", WithProviderConfig(xpv1.Reference{Name: "unittest-pc"})),
				kubeObjects: []client.Object{
					testutils.NewProviderConfig("unittest-pc", "cis-provider-secret", "sa-provider-secret"),
					testutils.NewSecret("cis-provider-secret", nil),
				},
			},
			want: want{
				err: errors.New("cannot get Service Account credentials"),
			},
		},
		"EmptyCISSecret": {
			reason: "Expect error if CIS secret is empty",
			args: args{
				cr: NewSubaccount("unittest-sa", WithProviderConfig(xpv1.Reference{Name: "unittest-pc"})),
				kubeObjects: []client.Object{
					testutils.NewProviderConfig("unittest-pc", "cis-provider-secret", "sa-provider-secret"),
					testutils.NewSecret("cis-provider-secret", nil),
					testutils.NewSecret("sa-provider-secret", nil),
				},
			},
			want: want{
				err: errors.New("CF Secret is empty or nil, please check config & secrets referenced in provider config"),
			},
		},
		"NewServiceFnError": {
			reason: "Expect error if newServiceFn returns an error",
			args: args{
				cr: NewSubaccount("unittest-sa", WithProviderConfig(xpv1.Reference{Name: "unittest-pc"})),
				kubeObjects: []client.Object{
					testutils.NewProviderConfig("unittest-pc", "cis-provider-secret", "sa-provider-secret"),
					testutils.NewSecret("cis-provider-secret", map[string][]byte{"data": []byte("someCISCreds")}),
					testutils.NewSecret("sa-provider-secret", map[string][]byte{"credentials": []byte("someSACreds")}),
				},
				serviceFnReturn: &btp.Client{},
				serviceFnErr:    errors.New("serviceFnError"),
			},
			want: want{
				newServiceArgs: newServiceArgs{
					cisCreds: []byte("someCISCreds"),
					saCreds:  []byte("someSACreds"),
				},
				err: errors.New("serviceFnError"),
			},
		},
		"ConnectSuccess": {
			reason: "Expect no error if everything is set up correctly",
			args: args{
				cr: NewSubaccount("unittest-sa", WithProviderConfig(xpv1.Reference{Name: "unittest-pc"})),
				kubeObjects: []client.Object{
					testutils.NewProviderConfig("unittest-pc", "cis-provider-secret", "sa-provider-secret"),
					testutils.NewSecret("cis-provider-secret", map[string][]byte{"data": []byte("someCISCreds")}),
					testutils.NewSecret("sa-provider-secret", map[string][]byte{"credentials": []byte("someSACreds")}),
				},
				serviceFnReturn: &btp.Client{},
			},
			want: want{
				newServiceArgs: newServiceArgs{
					cisCreds: []byte("someCISCreds"),
					saCreds:  []byte("someSACreds"),
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			kube := testutils.NewFakeKubeClientBuilder().
				AddResources(tc.args.kubeObjects...).
				Build()
			ctrl := connector{
				kube:            &kube,
				usage:           trackingtest.NoOpReferenceResolverTracker{},
				resourcetracker: trackingtest.NoOpReferenceResolverTracker{},
				newServiceFn: func(cisSecretData []byte, serviceAccountSecretData []byte) (*btp.Client, error) {
					if tc.want.newServiceArgs.cisCreds != nil && string(tc.want.newServiceArgs.cisCreds) != string(cisSecretData) {
						t.Errorf("\n%s\ne.Connect(...): Passed CIS Creds to newServiceFN do not match; Passed: %v, Expected: %v", tc.reason, cisSecretData, tc.want.newServiceArgs.cisCreds)
					}
					if tc.want.newServiceArgs.saCreds != nil && string(tc.want.newServiceArgs.saCreds) != string(serviceAccountSecretData) {
						t.Errorf("\n%s\ne.Connect(...): Passed SA Creds to newServiceFN do not match; Passed: %v, Expected: %v", tc.reason, cisSecretData, tc.want.newServiceArgs.saCreds)
					}
					return tc.args.serviceFnReturn, tc.args.serviceFnErr
				},
			}
			client, err := ctrl.Connect(context.Background(), tc.args.cr)

			if contained := testutils.ContainsError(err, tc.want.err); !contained {
				t.Errorf("\n%s\ne.Connect(...): error \"%v\" not part of \"%v\"", tc.reason, err, tc.want.err)
			}
			if tc.want.err == nil {
				if client == nil {
					t.Errorf("\n%s\ne.Connect(...): Expected connector to be != nil", tc.reason)
				}
			}
		})
	}
}

func TestDelete(t *testing.T) {
	type args struct {
		cr         resource.Managed
		mockClient *MockSubaccountClient
		tracker    tracking.ReferenceResolverTracker
	}
	type want struct {
		err error
	}
	tests := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"NilResource": {
			reason: "Expect error if used with another resource type",
			args: args{
				cr:         nil,
				mockClient: &MockSubaccountClient{},
				tracker:    trackingtest.NoOpReferenceResolverTracker{},
			},
			want: want{
				err: errors.New(errNotSubaccount),
			},
		},
		"DeleteSuccess": {
			reason: "Deletion should be successful",
			args: args{
				cr: NewSubaccount("unittest-sa", WithStatus(v1alpha1.SubaccountObservation{SubaccountGuid: internal.Ptr("123")})),
				mockClient: &MockSubaccountClient{
					returnSubaccount: &accountclient.SubaccountResponseObject{Guid: "123"},
					mockDeleteSubaccountExecute: func(r accountclient.ApiDeleteSubaccountRequest) (*accountclient.SubaccountResponseObject, *http.Response, error) {
						return &accountclient.SubaccountResponseObject{Guid: "123", State: "Deleting"}, &http.Response{StatusCode: 200}, nil
					},
				},
				tracker: trackingtest.NoOpReferenceResolverTracker{},
			},
			want: want{
				// this needs a fix from implementation side, shoul not return error after deletion success. issue: https://github.com/SAP/crossplane-provider-btp/issues/155
				err: errors.New("Deletion Pending: Current status: Deleting"),
			},
		},
		"DeleteAPI404": {
			reason: "Deletion should be successful if subaccount not found",
			args: args{
				cr: NewSubaccount("unittest-sa", WithStatus(v1alpha1.SubaccountObservation{SubaccountGuid: internal.Ptr("123")})),
				mockClient: &MockSubaccountClient{
					returnSubaccount: &accountclient.SubaccountResponseObject{Guid: "123"},
					mockDeleteSubaccountExecute: func(r accountclient.ApiDeleteSubaccountRequest) (*accountclient.SubaccountResponseObject, *http.Response, error) {
						return &accountclient.SubaccountResponseObject{Guid: "123"}, &http.Response{StatusCode: 404}, nil
					},
				},
				tracker: trackingtest.NoOpReferenceResolverTracker{},
			},
			want: want{
				err: nil,
			},
		},
		"DeleteAPIError": {
			reason: "Deletion should fail if API returns error",
			args: args{
				cr: NewSubaccount("unittest-sa", WithStatus(v1alpha1.SubaccountObservation{SubaccountGuid: internal.Ptr("123")})),
				mockClient: &MockSubaccountClient{
					returnSubaccount: &accountclient.SubaccountResponseObject{Guid: "123"},
					mockDeleteSubaccountExecute: func(r accountclient.ApiDeleteSubaccountRequest) (*accountclient.SubaccountResponseObject, *http.Response, error) {
						return &accountclient.SubaccountResponseObject{Guid: "123"}, &http.Response{StatusCode: 500}, errors.New("apiError")
					},
				},
				tracker: trackingtest.NoOpReferenceResolverTracker{},
			},
			want: want{
				err: errors.New("deletion of subaccount failed: apiError"),
			},
		},
		"SubaccountAlreadyInDeletingState": {
			reason: "Deletion should be successful if subaccount already in deleting state",
			args: args{
				cr: NewSubaccount("unittest-sa",
					WithStatus(v1alpha1.SubaccountObservation{SubaccountGuid: internal.Ptr("123")}),
					WithStatus(v1alpha1.SubaccountObservation{Status: internal.Ptr("DELETING")})),
				mockClient: &MockSubaccountClient{
					returnSubaccount: &accountclient.SubaccountResponseObject{Guid: "123"},
				},
				tracker: trackingtest.NoOpReferenceResolverTracker{},
			},
			want: want{
				err: nil,
			},
		},
		"TrackerBlocked": {
			reason: "Deletion should be blocked if tracker is blocked",
			args: args{
				cr: NewSubaccount("unittest-sa",
					WithStatus(v1alpha1.SubaccountObservation{SubaccountGuid: internal.Ptr("123")}),
					WithStatus(v1alpha1.SubaccountObservation{Status: internal.Ptr("DELETING")})),
				mockClient: &MockSubaccountClient{
					returnSubaccount: &accountclient.SubaccountResponseObject{Guid: "123"},
				},
				tracker: trackingtest.NoOpReferenceResolverTracker{
					IsResourceBlocked: true,
				},
			},
			want: want{
				err: errors.New("Resource cannot be deleted, still has usages"),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := external{
				btp: btp.Client{
					AccountsServiceClient: &accountclient.APIClient{
						SubaccountOperationsAPI: tc.args.mockClient,
					},
				},
				tracker: tc.args.tracker,
			}
			err := ctrl.Delete(context.Background(), tc.args.cr)
			if contained := testutils.ContainsError(err, tc.want.err); !contained {
				t.Errorf("\n%s\ne.Delete(...): error \"%v\" not part of \"%v\"", tc.reason, err, tc.want.err)
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	type args struct {
		cr           resource.Managed
		mockClient   *MockSubaccountClient
		mockAccessor AccountsApiAccessor
	}
	type want struct {
		err error
		o   managed.ExternalUpdate
		cr  resource.Managed
		// guid for which the move operation is called in Api
		moveTargetParam string
	}
	tests := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"NilResource": {
			reason: "Expect error if used with another resource type",
			args: args{
				cr: nil,
			},
			want: want{
				err: errors.New(errNotSubaccount),
			},
		},
		"SkipOnCreating": {
			reason: "Return Gracefully if creation is already triggered",
			args: args{
				cr: NewSubaccount("unittest-sa", WithStatus(v1alpha1.SubaccountObservation{Status: internal.Ptr("CREATING")})),
			},
			want: want{
				cr: NewSubaccount("unittest-sa", WithStatus(v1alpha1.SubaccountObservation{Status: internal.Ptr("CREATING")})),
				o:  managed.ExternalUpdate{},
			},
		},
		"BasicUpdateError": {
			reason: "Error while UpdateDescription in API",
			args: args{
				cr: NewSubaccount("unittest-sa",
					WithData(v1alpha1.SubaccountParameters{
						DirectoryGuid: "234",
						DirectoryRef:  &xpv1.Reference{Name: "dir-1"},
					}),
					WithStatus(v1alpha1.SubaccountObservation{
						SubaccountGuid: internal.Ptr("123"),
						ParentGuid:     internal.Ptr("234"),
					}),
				),
				mockAccessor: &MockAccountsApiAccessor{
					returnErr: errors.New("apiError"),
				},
			},
			want: want{
				cr: NewSubaccount("unittest-sa",
					WithData(v1alpha1.SubaccountParameters{
						DirectoryGuid: "234",
						DirectoryRef:  &xpv1.Reference{Name: "dir-1"},
					}),
					WithStatus(v1alpha1.SubaccountObservation{
						SubaccountGuid: internal.Ptr("123"),
						ParentGuid:     internal.Ptr("234"),
					})),
				o:   managed.ExternalUpdate{},
				err: errors.New("apiError"),
			},
		},
		"BasicUpdateSuccess": {
			reason: "UpdateDescription in API",
			args: args{
				cr: NewSubaccount("unittest-sa",
					WithData(v1alpha1.SubaccountParameters{
						DirectoryGuid: "234",
						DirectoryRef:  &xpv1.Reference{Name: "dir-1"},
					}),
					WithStatus(v1alpha1.SubaccountObservation{
						SubaccountGuid: internal.Ptr("123"),
						ParentGuid:     internal.Ptr("234"),
					}),
				),
				mockClient:   &MockSubaccountClient{returnSubaccount: &accountclient.SubaccountResponseObject{}},
				mockAccessor: &MockAccountsApiAccessor{},
			},
			want: want{
				cr: NewSubaccount("unittest-sa",
					WithData(v1alpha1.SubaccountParameters{
						DirectoryGuid: "234",
						DirectoryRef:  &xpv1.Reference{Name: "dir-1"},
					}),
					WithStatus(v1alpha1.SubaccountObservation{
						SubaccountGuid: internal.Ptr("123"),
						ParentGuid:     internal.Ptr("234"),
					})),
				o: managed.ExternalUpdate{ConnectionDetails: managed.ConnectionDetails{}},
			},
		},
		"BasicUpdateSuccessWithLabels": {
			reason: "UpdateDescription in API",
			args: args{
				cr: NewSubaccount("unittest-sa",
					WithData(v1alpha1.SubaccountParameters{
						DirectoryGuid: "234",
						DirectoryRef:  &xpv1.Reference{Name: "dir-1"},
						Labels:        map[string][]string{"somekey": {"somevalue"}},
					}),
					WithStatus(v1alpha1.SubaccountObservation{
						SubaccountGuid: internal.Ptr("123"),
						ParentGuid:     internal.Ptr("234"),
					}),
				),
				mockClient:   &MockSubaccountClient{returnSubaccount: &accountclient.SubaccountResponseObject{}},
				mockAccessor: &MockAccountsApiAccessor{},
			},
			want: want{
				cr: NewSubaccount("unittest-sa",
					WithData(v1alpha1.SubaccountParameters{
						DirectoryGuid: "234",
						DirectoryRef:  &xpv1.Reference{Name: "dir-1"},
						Labels:        map[string][]string{"somekey": {"somevalue"}},
					}),
					WithStatus(v1alpha1.SubaccountObservation{
						SubaccountGuid: internal.Ptr("123"),
						ParentGuid:     internal.Ptr("234"),
					})),
				o: managed.ExternalUpdate{ConnectionDetails: managed.ConnectionDetails{}},
			},
		},
		"MoveAccountError": {
			reason: "Error attempting to move subaccount",
			args: args{
				cr: NewSubaccount("unittest-sa",
					WithData(v1alpha1.SubaccountParameters{
						DirectoryGuid: "345",
					}),
					WithStatus(v1alpha1.SubaccountObservation{
						SubaccountGuid: internal.Ptr("123"),
						ParentGuid:     internal.Ptr("234"),
					}),
				),
				mockAccessor: &MockAccountsApiAccessor{returnErr: errors.New("apiError")},
			},
			want: want{
				cr: NewSubaccount("unittest-sa",
					WithData(v1alpha1.SubaccountParameters{
						DirectoryGuid: "345",
					}),
					WithStatus(v1alpha1.SubaccountObservation{
						SubaccountGuid: internal.Ptr("123"),
						ParentGuid:     internal.Ptr("234"),
					})),
				o:   managed.ExternalUpdate{},
				err: errors.New("apiError"),
			},
		},
		"MoveAccountDirectorySuccess": {
			reason: "Successfully move subaccount over API",
			args: args{
				cr: NewSubaccount("unittest-sa",
					WithData(v1alpha1.SubaccountParameters{
						DirectoryGuid: "dir-123",
					}),
					WithStatus(v1alpha1.SubaccountObservation{
						SubaccountGuid:    internal.Ptr("123"),
						GlobalAccountGUID: internal.Ptr("global-123"),
						ParentGuid:        internal.Ptr("global-123"),
					}),
				),
				mockAccessor: &MockAccountsApiAccessor{},
			},
			want: want{
				cr: NewSubaccount("unittest-sa",
					WithData(v1alpha1.SubaccountParameters{
						DirectoryGuid: "dir-123",
					}),
					WithStatus(v1alpha1.SubaccountObservation{
						SubaccountGuid:    internal.Ptr("123"),
						GlobalAccountGUID: internal.Ptr("global-123"),
						ParentGuid:        internal.Ptr("global-123"),
					})),
				o:               managed.ExternalUpdate{ConnectionDetails: managed.ConnectionDetails{}},
				moveTargetParam: "dir-123",
			},
		},
		"MoveAccountGlobalSuccess": {
			reason: "Successfully move subaccount over API",
			args: args{
				cr: NewSubaccount("unittest-sa",
					WithData(v1alpha1.SubaccountParameters{
						DirectoryGuid: "",
					}),
					WithStatus(v1alpha1.SubaccountObservation{
						SubaccountGuid:    internal.Ptr("123"),
						GlobalAccountGUID: internal.Ptr("global-123"),
						ParentGuid:        internal.Ptr("dir-123"),
					}),
				),
				mockAccessor: &MockAccountsApiAccessor{},
			},
			want: want{
				cr: NewSubaccount("unittest-sa",
					WithData(v1alpha1.SubaccountParameters{
						DirectoryGuid: "",
					}),
					WithStatus(v1alpha1.SubaccountObservation{
						SubaccountGuid:    internal.Ptr("123"),
						GlobalAccountGUID: internal.Ptr("global-123"),
						ParentGuid:        internal.Ptr("dir-123"),
					})),
				o:               managed.ExternalUpdate{ConnectionDetails: managed.ConnectionDetails{}},
				moveTargetParam: "global-123",
			},
		},
		"LabelUpdateSuccess": {
			reason: "Removing label from subaccount should succeed in API",
			args: args{
				cr: NewSubaccount("unittest-sa",
					WithData(v1alpha1.SubaccountParameters{
						Labels: nil,
					}),
					WithStatus(v1alpha1.SubaccountObservation{
						Labels: &map[string][]string{"somekey": {"somevalue"}},
					}),
				),
				mockClient:   &MockSubaccountClient{returnSubaccount: &accountclient.SubaccountResponseObject{}},
				mockAccessor: &MockAccountsApiAccessor{},
			},
			want: want{
				cr: NewSubaccount("unittest-sa",
					WithData(v1alpha1.SubaccountParameters{
						Labels: nil,
					}),
					WithStatus(v1alpha1.SubaccountObservation{
						Labels: &map[string][]string{"somekey": {"somevalue"}},
					})),
				o: managed.ExternalUpdate{ConnectionDetails: managed.ConnectionDetails{}},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := external{
				btp: btp.Client{
					AccountsServiceClient: &accountclient.APIClient{
						SubaccountOperationsAPI: tc.args.mockClient,
					},
				},
				accountsAccessor: tc.args.mockAccessor,
			}
			got, err := ctrl.Update(context.Background(), tc.args.cr)
			if contained := testutils.ContainsError(err, tc.want.err); !contained {
				t.Errorf("\ne.Create(...): error \"%v\" not part of \"%v\"", err, tc.want.err)
			}
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\n%s\ne.Update(...): -want, +got:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.cr, tc.args.cr); diff != "" {
				t.Errorf("\n%s\ne.Update(...): -want cr, +got cr:\n%s\n", tc.reason, diff)
			}
		})
	}
}

func NewSubaccount(name string, m ...SubaccountModifier) *v1alpha1.Subaccount {
	cr := &v1alpha1.Subaccount{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}
	for _, f := range m {
		f(cr)
	}
	return cr
}

type SubaccountModifier func(dirEnvironment *v1alpha1.Subaccount)

func WithStatus(status v1alpha1.SubaccountObservation) SubaccountModifier {
	return func(r *v1alpha1.Subaccount) {
		r.Status.AtProvider = status
	}
}

func WithData(data v1alpha1.SubaccountParameters) SubaccountModifier {
	return func(r *v1alpha1.Subaccount) {
		r.Spec.ForProvider = data
	}
}

func WithProviderConfig(pc xpv1.Reference) SubaccountModifier {
	return func(r *v1alpha1.Subaccount) {
		r.Spec.ProviderConfigReference = &pc
	}
}

func WithConditions(c ...xpv1.Condition) SubaccountModifier {
	return func(r *v1alpha1.Subaccount) { r.Status.ConditionedStatus.Conditions = c }
}
