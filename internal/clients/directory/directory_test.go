package directory

import (
	"context"
	"errors"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/internal"
	accountclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-accounts-service-api-go/pkg"
	"github.com/sap/crossplane-provider-btp/internal/testutils"
)

func TestNeedsCreation(t *testing.T) {
	type args struct {
		cr         *v1alpha1.Directory
		mockClient MockDirClient
	}
	type want struct {
		o   bool
		cr  resource.Managed
		err error
	}
	tests := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"NoGuid": {
			reason: "Without Guid set as external-name we can't look it up, so we have to create it",
			args: args{
				cr: testutils.NewDirectory("unittest-client"),
			},
			want: want{
				o:  true,
				cr: testutils.NewDirectory("unittest-client"),
			},
		},
		"APIError": {
			reason: "In case of failing lookup we expect to require a creation",
			args: args{
				mockClient: MockDirClient{GetErr: errors.New("internalServerError")},
				cr: testutils.NewDirectory("unittest-client",
					testutils.WithExternalName("aaaaaaaa-bbbb-cccc-eeee-ffffffffffff")),
			},
			want: want{
				o:   true,
				err: errors.New("internalServerError"),
				cr: testutils.NewDirectory("unittest-client",
					testutils.WithExternalName("aaaaaaaa-bbbb-cccc-eeee-ffffffffffff")),
			},
		},
		"NotExistingAnymore": {
			reason: "In case of failing lookup we expect to require a creation",
			args: args{
				mockClient: MockDirClient{GetErr: errors.New("notFoundError"), ResultStatusCode: 404},
				cr: testutils.NewDirectory("unittest-client",
					testutils.WithExternalName("aaaaaaaa-bbbb-cccc-eeee-ffffffffffff")),
			},
			want: want{
				o:   true,
				err: nil,
				cr: testutils.NewDirectory("unittest-client",
					testutils.WithExternalName("aaaaaaaa-bbbb-cccc-eeee-ffffffffffff")),
			},
		},
		"Available": {
			reason: "If its available no need for creation",
			args: args{
				mockClient: MockDirClient{
					GetResult: &accountclient.DirectoryResponseObject{
						Guid: "123",
					}},
				cr: testutils.NewDirectory("unittest-client",
					testutils.WithExternalName("aaaaaaaa-bbbb-cccc-eeee-ffffffffffff")),
			},
			want: want{
				o: false,
				cr: testutils.NewDirectory("unittest-client",
					testutils.WithExternalName("aaaaaaaa-bbbb-cccc-eeee-ffffffffffff")),
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			btpClient := btp.Client{AccountsServiceClient: &accountclient.APIClient{DirectoryOperationsAPI: tc.args.mockClient}}

			client := NewDirectoryClient(&btpClient, tc.args.cr)
			got, err := client.NeedsCreation(context.Background())

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.NeedsCreation(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\n%s\ne.NeedsCreation(...): -want, +got:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.cr, tc.args.cr); diff != "" {
				t.Errorf("\n%s\ne.NeedsCreation(...): -want cr, +got cr:\n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestNeedsUpdate(t *testing.T) {
	type args struct {
		cr         *v1alpha1.Directory
		mockClient MockDirClient
		cachedAPI  *accountclient.DirectoryResponseObject
	}
	type want struct {
		o   bool
		cr  resource.Managed
		err error
	}
	tests := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"NoCacheNoGUID": {
			args: args{
				cr: testutils.NewDirectory("unittest-client",
					testutils.WithData(v1alpha1.DirectoryParameters{Description: "cr-desc"})),
			},
			want: want{
				err: errors.New(errMisUse),
				cr: testutils.NewDirectory("unittest-client",
					testutils.WithData(v1alpha1.DirectoryParameters{Description: "cr-desc"})),
			},
		},
		"NoCacheAPIFailure": {
			args: args{
				cr: testutils.NewDirectory("unittest-client",
					testutils.WithData(v1alpha1.DirectoryParameters{Description: "cr-desc"}),
					testutils.WithExternalName("aaaaaaaa-bbbb-cccc-eeee-ffffffffffff")),
				mockClient: MockDirClient{GetErr: errors.New("apiError")},
			},
			want: want{
				err: errors.New("apiError"),
				cr: testutils.NewDirectory("unittest-client",
					testutils.WithData(v1alpha1.DirectoryParameters{Description: "cr-desc"}),
					testutils.WithExternalName("aaaaaaaa-bbbb-cccc-eeee-ffffffffffff")),
			},
		},
		"NeedsUpdateCache": {
			args: args{
				cr: testutils.NewDirectory("unittest-client",
					testutils.WithData(v1alpha1.DirectoryParameters{Description: "cr-desc"})),
				cachedAPI: &accountclient.DirectoryResponseObject{
					Description: "api-desc"},
			},
			want: want{
				o: true,
				cr: testutils.NewDirectory("unittest-client",
					testutils.WithData(v1alpha1.DirectoryParameters{Description: "cr-desc"})),
			},
		},
		"NeedsUpdateApiRequested": {
			reason: "If there are no changes we expect to not require an update",
			args: args{
				cr: testutils.NewDirectory("unittest-client",
					testutils.WithData(v1alpha1.DirectoryParameters{DisplayName: internal.Ptr("someName")}),
					testutils.WithExternalName("aaaaaaaa-bbbb-cccc-eeee-ffffffffffff")),
				mockClient: MockDirClient{
					GetResult: &accountclient.DirectoryResponseObject{
						DisplayName: "anotherName"}},
			},
			want: want{
				o: true,
				cr: testutils.NewDirectory("unittest-client",
					testutils.WithData(v1alpha1.DirectoryParameters{DisplayName: internal.Ptr("someName")}),
					testutils.WithExternalName("aaaaaaaa-bbbb-cccc-eeee-ffffffffffff")),
			},
		},
		"NeedsUpdateDirectoryFeatures": {
			reason: "changed directoryFeatures need to be recognized as well",
			args: args{
				cachedAPI: &accountclient.DirectoryResponseObject{
					DisplayName:       "someName",
					DirectoryFeatures: []string{"DEFAULT", "ENTITLEMENTS"},
				},
				cr: testutils.NewDirectory("unittest-client",
					testutils.WithData(v1alpha1.DirectoryParameters{DisplayName: internal.Ptr("someName"), DirectoryFeatures: []string{"DEFAULT"}}),
					testutils.WithExternalName("aaaaaaaa-bbbb-cccc-eeee-ffffffffffff")),
			},
			want: want{
				o: true,
				cr: testutils.NewDirectory("unittest-client",
					testutils.WithData(v1alpha1.DirectoryParameters{DisplayName: internal.Ptr("someName"), DirectoryFeatures: []string{"DEFAULT"}}),
					testutils.WithExternalName("aaaaaaaa-bbbb-cccc-eeee-ffffffffffff")),
			},
		},
		"UpToDateApiRequested": {
			reason: "If there are no changes we expect to not require an update",
			args: args{
				cr: testutils.NewDirectory("unittest-client",
					testutils.WithData(v1alpha1.DirectoryParameters{
						Description: "desc",
						DisplayName: internal.Ptr("someName"),
						Labels:      map[string][]string{"custom_label": {"custom_value"}},
					}),
					testutils.WithExternalName("aaaaaaaa-bbbb-cccc-eeee-ffffffffffff")),
				mockClient: MockDirClient{
					GetResult: &accountclient.DirectoryResponseObject{
						Description: "desc",
						DisplayName: "someName",
						Labels:      &map[string][]string{"custom_label": {"custom_value"}},
						DirectoryFeatures: []string{"DEFAULT"},
					}},
			},
			want: want{
				o: false,
				cr: testutils.NewDirectory("unittest-client",
					testutils.WithData(v1alpha1.DirectoryParameters{
						Description: "desc",
						DisplayName: internal.Ptr("someName"),
						Labels:      map[string][]string{"custom_label": {"custom_value"}},
					}),
					testutils.WithExternalName("aaaaaaaa-bbbb-cccc-eeee-ffffffffffff")),
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			btpClient := btp.Client{AccountsServiceClient: &accountclient.APIClient{DirectoryOperationsAPI: tc.args.mockClient}}

			client := NewDirectoryClient(&btpClient, tc.args.cr)
			client.cachedApi = tc.args.cachedAPI

			got, err := client.NeedsUpdate(context.Background())

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.NeedsUpdate(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\n%s\ne.NeedsUpdate(...): -want, +got:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.cr, tc.args.cr); diff != "" {
				t.Errorf("\n%s\ne.NeedsUpdate(...): -want cr, +got cr:\n%s\n", tc.reason, diff)
			}

		})
	}
}

func TestIsAvailable(t *testing.T) {
	type args struct {
		cr         *v1alpha1.Directory
		mockClient MockDirClient
		cachedAPI  *accountclient.DirectoryResponseObject
	}
	type want struct {
		o  bool
		cr resource.Managed
	}
	tests := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"NotAvailableOnNotOK": {
			args: args{
				cr: testutils.NewDirectory("unittest-client",
					testutils.WithStatus(v1alpha1.DirectoryObservation{EntityState: internal.Ptr("CREATING")})),
			},
			want: want{
				o: false,
				cr: testutils.NewDirectory("unittest-client",
					testutils.WithStatus(v1alpha1.DirectoryObservation{EntityState: internal.Ptr("CREATING")})),
			},
		},
		"AvailableOnOk": {
			args: args{
				cr: testutils.NewDirectory("unittest-client",
					testutils.WithStatus(v1alpha1.DirectoryObservation{EntityState: internal.Ptr(v1alpha1.DirectoryEntityStateOk)})),
			},
			want: want{
				o: true,
				cr: testutils.NewDirectory("unittest-client",
					testutils.WithStatus(v1alpha1.DirectoryObservation{EntityState: internal.Ptr(v1alpha1.DirectoryEntityStateOk)})),
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			btpClient := btp.Client{AccountsServiceClient: &accountclient.APIClient{DirectoryOperationsAPI: tc.args.mockClient}}

			client := NewDirectoryClient(&btpClient, tc.args.cr)
			client.cachedApi = tc.args.cachedAPI

			got := client.IsAvailable()

			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\n%s\ne.NeedsUpdate(...): -want, +got:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.cr, tc.args.cr); diff != "" {
				t.Errorf("\n%s\ne.NeedsUpdate(...): -want cr, +got cr:\n%s\n", tc.reason, diff)
			}

		})
	}
}

func TestSyncStatus(t *testing.T) {
	type args struct {
		mockClient MockDirClient
		cr         *v1alpha1.Directory
		cachedApi  *accountclient.DirectoryResponseObject
	}
	type want struct {
		cr  resource.Managed
		err error
	}
	tests := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"SetGUIDError": {
			reason: "Can't set GUID in status if there is an API error",
			args: args{
				mockClient: MockDirClient{GetErr: errors.New("apiError")},
				cr: testutils.NewDirectory("unittest-client", testutils.WithData(v1alpha1.DirectoryParameters{
					Description:       "Some Directory",
					DirectoryAdmins:   []string{"1@sap.com"},
					DirectoryFeatures: []string{"DEFAULT"},
					DisplayName:       internal.Ptr("created-from-unittest"),
					Labels:            map[string][]string{"custom_label": {"custom_value"}},
				}), testutils.WithExternalName("aaaaaaaa-bbbb-cccc-eeee-ffffffffffff")),
				cachedApi: nil,
			},
			want: want{
				cr: testutils.NewDirectory("unittest-client", testutils.WithData(v1alpha1.DirectoryParameters{
					Description:       "Some Directory",
					DirectoryAdmins:   []string{"1@sap.com"},
					DirectoryFeatures: []string{"DEFAULT"},
					DisplayName:       internal.Ptr("created-from-unittest"),
					Labels:            map[string][]string{"custom_label": {"custom_value"}},
				}), testutils.WithExternalName("aaaaaaaa-bbbb-cccc-eeee-ffffffffffff")),
				err: errors.New("apiError"),
			},
		},
		"SetGUIDSucess": {
			reason: "Expect to set GUID in status",
			args: args{
				cr: testutils.NewDirectory("unittest-client", testutils.WithData(v1alpha1.DirectoryParameters{
					Description:       "Some Directory",
					DirectoryAdmins:   []string{"1@sap.com"},
					DirectoryFeatures: []string{"DEFAULT"},
					DisplayName:       internal.Ptr("created-from-unittest"),
					Labels:            map[string][]string{"custom_label": {"custom_value"}},
				})),
				cachedApi: &accountclient.DirectoryResponseObject{
					Guid:              "123",
					EntityState:       internal.Ptr("OK"),
					StateMessage:      internal.Ptr("Its great"),
					Subdomain:         internal.Ptr("some-subdomain"),
					DirectoryFeatures: []string{"DEFAULT", "ENTITLEMENTS"},
				},
			},
			want: want{
				cr: testutils.NewDirectory("unittest-client", testutils.WithData(v1alpha1.DirectoryParameters{
					Description:       "Some Directory",
					DirectoryAdmins:   []string{"1@sap.com"},
					DirectoryFeatures: []string{"DEFAULT"},
					DisplayName:       internal.Ptr("created-from-unittest"),
					Labels:            map[string][]string{"custom_label": {"custom_value"}},
				}), testutils.WithStatus(v1alpha1.DirectoryObservation{
					Guid:              internal.Ptr("123"),
					EntityState:       internal.Ptr("OK"),
					StateMessage:      internal.Ptr("Its great"),
					Subdomain:         internal.Ptr("some-subdomain"),
					DirectoryFeatures: []string{"DEFAULT", "ENTITLEMENTS"},
				})),
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			btpClient := &btp.Client{AccountsServiceClient: &accountclient.APIClient{DirectoryOperationsAPI: tc.args.mockClient}}
			client := NewDirectoryClient(btpClient, tc.args.cr)
			client.cachedApi = tc.args.cachedApi
			err := client.SyncStatus(context.Background())

			// make sure changes have been applied to passed instance
			if diff := cmp.Diff(tc.want.cr, tc.args.cr); diff != "" {
				t.Errorf("\n%s\ne.SyncStatus(...): -want cr, +got cr:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.SyncStatus(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestCreateDirectory(t *testing.T) {
	type args struct {
		cr         *v1alpha1.Directory
		mockClient MockDirClient
	}
	type want struct {
		cr  resource.Managed
		err error
	}
	tests := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"APIFailure": {
			reason: "",
			args: args{
				mockClient: MockDirClient{CreateErr: errors.New("InternalServerError")},
				cr: testutils.NewDirectory("unittest-client", testutils.WithData(v1alpha1.DirectoryParameters{
					Description:       "Some Directory",
					DirectoryAdmins:   []string{"1@sap.com"},
					DirectoryFeatures: []string{"DEFAULT"},
					DisplayName:       internal.Ptr("created-from-unittest"),
					Labels:            map[string][]string{"custom_label": {"custom_value"}},
				})),
			},
			want: want{
				err: errors.New("InternalServerError"),
				cr: testutils.NewDirectory("unittest-client", testutils.WithData(v1alpha1.DirectoryParameters{
					Description:       "Some Directory",
					DirectoryAdmins:   []string{"1@sap.com"},
					DirectoryFeatures: []string{"DEFAULT"},
					DisplayName:       internal.Ptr("created-from-unittest"),
					Labels:            map[string][]string{"custom_label": {"custom_value"}},
				})),
			},
		},
		"Success": {
			reason: "With successful API call we expect to succeed the operatior",
			args: args{
				mockClient: MockDirClient{
					CreateResult: &accountclient.DirectoryResponseObject{
						Guid: "123",
					},
				},
				cr: testutils.NewDirectory("unittest-client", testutils.WithData(v1alpha1.DirectoryParameters{
					Description:       "Some Directory",
					DirectoryAdmins:   []string{"1@sap.com"},
					DirectoryFeatures: []string{"DEFAULT"},
					DisplayName:       internal.Ptr("created-from-unittest"),
					Labels:            map[string][]string{"custom_label": {"custom_value"}},
				})),
			},
			want: want{
				cr: testutils.NewDirectory("unittest-client", testutils.WithData(v1alpha1.DirectoryParameters{
					Description:       "Some Directory",
					DirectoryAdmins:   []string{"1@sap.com"},
					DirectoryFeatures: []string{"DEFAULT"},
					DisplayName:       internal.Ptr("created-from-unittest"),
					Labels:            map[string][]string{"custom_label": {"custom_value"}},
				}),
					testutils.WithExternalName("123")),
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			btpClient := btp.Client{AccountsServiceClient: &accountclient.APIClient{DirectoryOperationsAPI: tc.args.mockClient}}

			client := NewDirectoryClient(&btpClient, tc.args.cr)
			got, err := client.CreateDirectory(context.Background())

			if diff := cmp.Diff(tc.want.cr, got); diff != "" {
				t.Errorf("\n%s\ne.CreateDirectory(...): -want, +got:\n%s\n", tc.reason, diff)
			}
			// make sure changes have been applied to passed instance
			if diff := cmp.Diff(tc.want.cr, tc.args.cr); diff != "" {
				t.Errorf("\n%s\ne.CreateDirectory(...): -want cr, +got cr:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.CreateDirectory(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestUpdateDirectory(t *testing.T) {
	type args struct {
		cr         *v1alpha1.Directory
		mockClient MockDirClient
	}
	type want struct {
		cr  resource.Managed
		err error
	}
	tests := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"CorruptedUsage": {
			reason: "We can't update without an ID in the status, we want to prevent unforseen side effects here",
			args: args{
				cr: testutils.NewDirectory("unittest-client", testutils.WithData(v1alpha1.DirectoryParameters{
					Description:       "Some Directory",
					DirectoryAdmins:   []string{"1@sap.com"},
					DirectoryFeatures: []string{"DEFAULT"},
					DisplayName:       internal.Ptr("created-from-unittest"),
					Labels:            map[string][]string{"custom_label": {"custom_value"}},
				})),
			},
			want: want{
				err: errors.New(errMisUse),
				cr: testutils.NewDirectory("unittest-client", testutils.WithData(v1alpha1.DirectoryParameters{
					Description:       "Some Directory",
					DirectoryAdmins:   []string{"1@sap.com"},
					DirectoryFeatures: []string{"DEFAULT"},
					DisplayName:       internal.Ptr("created-from-unittest"),
					Labels:            map[string][]string{"custom_label": {"custom_value"}},
				})),
			},
		},
		"APIFailureOnBasicUpdate": {
			reason: "We should only update basic settings on fail due to API failure",
			args: args{
				mockClient: MockDirClient{UpdateErr: errors.New("internalServerError")},
				cr: testutils.NewDirectory("unittest-client", testutils.WithData(v1alpha1.DirectoryParameters{
					Description:       "Some Directory",
					DirectoryAdmins:   []string{"1@sap.com"},
					DirectoryFeatures: []string{"DEFAULT"},
					DisplayName:       internal.Ptr("created-from-unittest"),
					Labels:            map[string][]string{"custom_label": {"custom_value"}},
				}), testutils.WithExternalName("aaaaaaaa-bbbb-cccc-eeee-ffffffffffff")),
			},
			want: want{
				err: errors.New("internalServerError"),
				cr: testutils.NewDirectory("unittest-client", testutils.WithData(v1alpha1.DirectoryParameters{
					Description:       "Some Directory",
					DirectoryAdmins:   []string{"1@sap.com"},
					DirectoryFeatures: []string{"DEFAULT"},
					DisplayName:       internal.Ptr("created-from-unittest"),
					Labels:            map[string][]string{"custom_label": {"custom_value"}},
				}), testutils.WithExternalName("aaaaaaaa-bbbb-cccc-eeee-ffffffffffff")),
			},
		},
		"APIFailureDirectorySettings": {
			reason: "We expect to update basics successfully and fail on directory Settings",
			args: args{
				mockClient: MockDirClient{UpdateErr: nil, UpdateSettingsErr: errors.New("updateSettingsInternalServerError")},
				cr: testutils.NewDirectory("unittest-client", testutils.WithData(v1alpha1.DirectoryParameters{
					Description:       "Some Directory",
					DirectoryAdmins:   []string{"1@sap.com"},
					DirectoryFeatures: []string{"DEFAULT", "ENTITLEMENTS"},
					DisplayName:       internal.Ptr("created-from-unittest"),
					Labels:            map[string][]string{"custom_label": {"custom_value"}},
				}), testutils.WithExternalName("aaaaaaaa-bbbb-cccc-eeee-ffffffffffff")),
			},
			want: want{
				err: errors.New("updateSettingsInternalServerError"),
				cr: testutils.NewDirectory("unittest-client", testutils.WithData(v1alpha1.DirectoryParameters{
					Description:       "Some Directory",
					DirectoryAdmins:   []string{"1@sap.com"},
					DirectoryFeatures: []string{"DEFAULT", "ENTITLEMENTS"},
					DisplayName:       internal.Ptr("created-from-unittest"),
					Labels:            map[string][]string{"custom_label": {"custom_value"}},
				}), testutils.WithExternalName("aaaaaaaa-bbbb-cccc-eeee-ffffffffffff")),
			},
		},
		"SuccessUpdate": {
			reason: "With successful API call we expect to succeed the operations",
			args: args{
				mockClient: MockDirClient{UpdateErr: nil, UpdateSettingsErr: nil},
				cr: testutils.NewDirectory("unittest-client", testutils.WithData(v1alpha1.DirectoryParameters{
					Description:       "Some Directory",
					DirectoryAdmins:   []string{"1@sap.com"},
					DirectoryFeatures: []string{"DEFAULT", "ENTITLEMENTS"},
					DisplayName:       internal.Ptr("created-from-unittest"),
					Labels:            map[string][]string{"custom_label": {"custom_value"}},
				}), testutils.WithExternalName("aaaaaaaa-bbbb-cccc-eeee-ffffffffffff")),
			},
			want: want{
				cr: testutils.NewDirectory("unittest-client", testutils.WithData(v1alpha1.DirectoryParameters{
					Description:       "Some Directory",
					DirectoryAdmins:   []string{"1@sap.com"},
					DirectoryFeatures: []string{"DEFAULT", "ENTITLEMENTS"},
					DisplayName:       internal.Ptr("created-from-unittest"),
					Labels:            map[string][]string{"custom_label": {"custom_value"}},
				}), testutils.WithExternalName("aaaaaaaa-bbbb-cccc-eeee-ffffffffffff")),
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			btpClient := btp.Client{AccountsServiceClient: &accountclient.APIClient{DirectoryOperationsAPI: tc.args.mockClient}}

			client := NewDirectoryClient(&btpClient, tc.args.cr)
			got, err := client.UpdateDirectory(context.Background())

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.UpdateDirectory(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.cr, got); diff != "" {
				t.Errorf("\n%s\ne.UpdateDirectory(...): -want cr, +got cr:\n%s\n", tc.reason, diff)
			}
			// make sure changes have been applied to passed instance
			if diff := cmp.Diff(tc.want.cr, tc.args.cr); diff != "" {
				t.Errorf("\n%s\ne.UpdateDirectory(...): -want cr, +got cr:\n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestDeleteDirectory(t *testing.T) {
	type args struct {
		cr         *v1alpha1.Directory
		mockClient MockDirClient
	}
	type want struct {
		err error
	}
	tests := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"CorruptedUsage": {
			reason: "We can't delete without an ID in the status, we want to prevent unforseen side effects here",
			args: args{
				cr: testutils.NewDirectory("unittest-client", testutils.WithData(v1alpha1.DirectoryParameters{
					Description:       "Some Directory",
					DirectoryAdmins:   []string{"1@sap.com"},
					DirectoryFeatures: []string{"DEFAULT"},
					DisplayName:       internal.Ptr("created-from-unittest"),
					Labels:            map[string][]string{"custom_label": {"custom_value"}},
				})),
			},
			want: want{
				err: errors.New(errMisUse),
			},
		},
		"APIFailure": {
			reason: "",
			args: args{
				mockClient: MockDirClient{DeleteErr: errors.New("InternalServerError")},
				cr: testutils.NewDirectory("unittest-client", testutils.WithData(v1alpha1.DirectoryParameters{
					Description:       "Some Directory",
					DirectoryAdmins:   []string{"1@sap.com"},
					DirectoryFeatures: []string{"DEFAULT"},
					DisplayName:       internal.Ptr("created-from-unittest"),
					Labels:            map[string][]string{"custom_label": {"custom_value"}},
				}), testutils.WithExternalName("aaaaaaaa-bbbb-cccc-eeee-ffffffffffff")),
			},
			want: want{
				err: errors.New("InternalServerError"),
			},
		},
		"Success": {
			reason: "With successful API call we expect to succeed the operation",
			args: args{
				mockClient: MockDirClient{
					CreateResult: &accountclient.DirectoryResponseObject{
						Guid: "123",
					},
				},
				cr: testutils.NewDirectory("unittest-client", testutils.WithData(v1alpha1.DirectoryParameters{
					Description:       "Some Directory",
					DirectoryAdmins:   []string{"1@sap.com"},
					DirectoryFeatures: []string{"DEFAULT"},
					DisplayName:       internal.Ptr("created-from-unittest"),
					Labels:            map[string][]string{"custom_label": {"custom_value"}},
				}), testutils.WithExternalName("aaaaaaaa-bbbb-cccc-eeee-ffffffffffff")),
			},
			want: want{
				err: nil,
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			btpClient := btp.Client{AccountsServiceClient: &accountclient.APIClient{DirectoryOperationsAPI: tc.args.mockClient}}

			client := NewDirectoryClient(&btpClient, tc.args.cr)
			err := client.DeleteDirectory(context.Background())

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.DeleteDirectory(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
		})
	}
}
