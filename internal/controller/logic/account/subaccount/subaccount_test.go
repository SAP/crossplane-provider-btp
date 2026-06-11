package subaccount

import (
	"context"
	"net/http"
	"reflect"
	"testing"
	"unsafe"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource/fake"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	base "github.com/sap/crossplane-provider-btp/apis/base/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/internal"
	accountclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-accounts-service-api-go/pkg"
	"github.com/sap/crossplane-provider-btp/internal/testutils"
)

const SAMPLE_GUID = "12340000-0000-0000-0000-000000000000"

// --- Mock SubaccountOperationsAPI ---

type MockSubaccountClient struct {
	returnSubaccounts           *accountclient.ResponseCollection
	returnSubaccount            *accountclient.SubaccountResponseObject
	mockDeleteSubaccountExecute func(r accountclient.ApiDeleteSubaccountRequest) (*accountclient.SubaccountResponseObject, *http.Response, error)
	returnErr                   error
	getSubaccountErr            error
	httpStatusCode              int
}

var _ accountclient.SubaccountOperationsAPI = &MockSubaccountClient{}

func (m *MockSubaccountClient) GetSubaccounts(_ context.Context) accountclient.ApiGetSubaccountsRequest {
	return accountclient.ApiGetSubaccountsRequest{ApiService: m}
}
func (m *MockSubaccountClient) GetSubaccountsExecute(_ accountclient.ApiGetSubaccountsRequest) (*accountclient.ResponseCollection, *http.Response, error) {
	return m.returnSubaccounts, nil, m.returnErr
}
func (m *MockSubaccountClient) CreateSubaccount(_ context.Context) accountclient.ApiCreateSubaccountRequest {
	return accountclient.ApiCreateSubaccountRequest{ApiService: m}
}
func (m *MockSubaccountClient) CreateSubaccountExecute(_ accountclient.ApiCreateSubaccountRequest) (*accountclient.SubaccountResponseObject, *http.Response, error) {
	return m.returnSubaccount, &http.Response{StatusCode: m.httpStatusCode}, m.returnErr
}
func (m *MockSubaccountClient) GetSubaccount(_ context.Context, _ string) accountclient.ApiGetSubaccountRequest {
	return accountclient.ApiGetSubaccountRequest{ApiService: m}
}
func (m *MockSubaccountClient) GetSubaccountExecute(_ accountclient.ApiGetSubaccountRequest) (*accountclient.SubaccountResponseObject, *http.Response, error) {
	if m.getSubaccountErr != nil {
		return nil, nil, m.getSubaccountErr
	}
	return m.returnSubaccount, nil, m.returnErr
}
func (m *MockSubaccountClient) UpdateSubaccount(_ context.Context, _ string) accountclient.ApiUpdateSubaccountRequest {
	return accountclient.ApiUpdateSubaccountRequest{ApiService: m}
}
func (m *MockSubaccountClient) UpdateSubaccountExecute(_ accountclient.ApiUpdateSubaccountRequest) (*accountclient.SubaccountResponseObject, *http.Response, error) {
	return m.returnSubaccount, nil, m.returnErr
}
func (m *MockSubaccountClient) MoveSubaccount(_ context.Context, _ string) accountclient.ApiMoveSubaccountRequest {
	return accountclient.ApiMoveSubaccountRequest{ApiService: m}
}
func (m *MockSubaccountClient) MoveSubaccountExecute(_ accountclient.ApiMoveSubaccountRequest) (*accountclient.SubaccountResponseObject, *http.Response, error) {
	return m.returnSubaccount, nil, m.returnErr
}
func (m *MockSubaccountClient) DeleteSubaccount(_ context.Context, _ string) accountclient.ApiDeleteSubaccountRequest {
	return accountclient.ApiDeleteSubaccountRequest{ApiService: m}
}
func (m *MockSubaccountClient) DeleteSubaccountExecute(r accountclient.ApiDeleteSubaccountRequest) (*accountclient.SubaccountResponseObject, *http.Response, error) {
	if m.mockDeleteSubaccountExecute != nil {
		return m.mockDeleteSubaccountExecute(r)
	}
	return m.returnSubaccount, &http.Response{StatusCode: m.httpStatusCode}, m.returnErr
}

// Unused interface methods
func (m *MockSubaccountClient) CloneNeoSubaccount(_ context.Context, _ string) accountclient.ApiCloneNeoSubaccountRequest {
	panic("not implemented")
}
func (m *MockSubaccountClient) CloneNeoSubaccountExecute(_ accountclient.ApiCloneNeoSubaccountRequest) (*accountclient.SubaccountResponseObject, *http.Response, error) {
	panic("not implemented")
}
func (m *MockSubaccountClient) CreateOrUpdateSubaccountSettings(_ context.Context, _ string) accountclient.ApiCreateOrUpdateSubaccountSettingsRequest {
	panic("not implemented")
}
func (m *MockSubaccountClient) CreateOrUpdateSubaccountSettingsExecute(_ accountclient.ApiCreateOrUpdateSubaccountSettingsRequest) (*accountclient.DataResponseObject, *http.Response, error) {
	panic("not implemented")
}
func (m *MockSubaccountClient) CreateServiceManagementBinding(_ context.Context, _ string) accountclient.ApiCreateServiceManagementBindingRequest {
	panic("not implemented")
}
func (m *MockSubaccountClient) CreateServiceManagementBindingExecute(_ accountclient.ApiCreateServiceManagementBindingRequest) (*accountclient.ServiceManagerBindingResponseObject, *http.Response, error) {
	panic("not implemented")
}
func (m *MockSubaccountClient) CreateServiceManagerBindingV2(_ context.Context, _ string) accountclient.ApiCreateServiceManagerBindingV2Request {
	panic("not implemented")
}
func (m *MockSubaccountClient) CreateServiceManagerBindingV2Execute(_ accountclient.ApiCreateServiceManagerBindingV2Request) (*accountclient.ServiceManagerBindingExtendedResponseObject, *http.Response, error) {
	panic("not implemented")
}
func (m *MockSubaccountClient) CreateSubaccountLabels(_ context.Context, _ string) accountclient.ApiCreateSubaccountLabelsRequest {
	panic("not implemented")
}
func (m *MockSubaccountClient) CreateSubaccountLabelsExecute(_ accountclient.ApiCreateSubaccountLabelsRequest) (*accountclient.LabelsResponseObject, *http.Response, error) {
	panic("not implemented")
}
func (m *MockSubaccountClient) DeleteServiceManagementBindingOfSubaccount(_ context.Context, _ string) accountclient.ApiDeleteServiceManagementBindingOfSubaccountRequest {
	panic("not implemented")
}
func (m *MockSubaccountClient) DeleteServiceManagementBindingOfSubaccountExecute(_ accountclient.ApiDeleteServiceManagementBindingOfSubaccountRequest) (*http.Response, error) {
	panic("not implemented")
}
func (m *MockSubaccountClient) DeleteServiceManagerBindingV2(_ context.Context, _ string, _ string) accountclient.ApiDeleteServiceManagerBindingV2Request {
	panic("not implemented")
}
func (m *MockSubaccountClient) DeleteServiceManagerBindingV2Execute(_ accountclient.ApiDeleteServiceManagerBindingV2Request) (*http.Response, error) {
	panic("not implemented")
}
func (m *MockSubaccountClient) DeleteSubaccountLabels(_ context.Context, _ string) accountclient.ApiDeleteSubaccountLabelsRequest {
	panic("not implemented")
}
func (m *MockSubaccountClient) DeleteSubaccountLabelsExecute(_ accountclient.ApiDeleteSubaccountLabelsRequest) (*accountclient.LabelsResponseObject, *http.Response, error) {
	panic("not implemented")
}
func (m *MockSubaccountClient) DeleteSubaccountSettings(_ context.Context, _ string) accountclient.ApiDeleteSubaccountSettingsRequest {
	panic("not implemented")
}
func (m *MockSubaccountClient) DeleteSubaccountSettingsExecute(_ accountclient.ApiDeleteSubaccountSettingsRequest) (*accountclient.DataResponseObject, *http.Response, error) {
	panic("not implemented")
}
func (m *MockSubaccountClient) GetAllServiceManagerBindingsV2(_ context.Context, _ string) accountclient.ApiGetAllServiceManagerBindingsV2Request {
	panic("not implemented")
}
func (m *MockSubaccountClient) GetAllServiceManagerBindingsV2Execute(_ accountclient.ApiGetAllServiceManagerBindingsV2Request) (*accountclient.ServiceManagerBindingsResponseList, *http.Response, error) {
	panic("not implemented")
}
func (m *MockSubaccountClient) GetServiceManagementBinding(_ context.Context, _ string) accountclient.ApiGetServiceManagementBindingRequest {
	panic("not implemented")
}
func (m *MockSubaccountClient) GetServiceManagementBindingExecute(_ accountclient.ApiGetServiceManagementBindingRequest) (*accountclient.ServiceManagerBindingResponseObject, *http.Response, error) {
	panic("not implemented")
}
func (m *MockSubaccountClient) GetServiceManagerBindingV2(_ context.Context, _ string, _ string) accountclient.ApiGetServiceManagerBindingV2Request {
	panic("not implemented")
}
func (m *MockSubaccountClient) GetServiceManagerBindingV2Execute(_ accountclient.ApiGetServiceManagerBindingV2Request) (*accountclient.ServiceManagerBindingExtendedResponseObject, *http.Response, error) {
	panic("not implemented")
}
func (m *MockSubaccountClient) GetSubaccountCustomProperties(_ context.Context, _ string) accountclient.ApiGetSubaccountCustomPropertiesRequest {
	panic("not implemented")
}
func (m *MockSubaccountClient) GetSubaccountCustomPropertiesExecute(_ accountclient.ApiGetSubaccountCustomPropertiesRequest) (*accountclient.ResponseCollection, *http.Response, error) {
	panic("not implemented")
}
func (m *MockSubaccountClient) GetSubaccountLabels(_ context.Context, _ string) accountclient.ApiGetSubaccountLabelsRequest {
	panic("not implemented")
}
func (m *MockSubaccountClient) GetSubaccountLabelsExecute(_ accountclient.ApiGetSubaccountLabelsRequest) (*accountclient.LabelsResponseObject, *http.Response, error) {
	panic("not implemented")
}
func (m *MockSubaccountClient) GetSubaccountSettings(_ context.Context, _ string) accountclient.ApiGetSubaccountSettingsRequest {
	panic("not implemented")
}
func (m *MockSubaccountClient) GetSubaccountSettingsExecute(_ accountclient.ApiGetSubaccountSettingsRequest) (*accountclient.DataResponseObject, *http.Response, error) {
	panic("not implemented")
}
func (m *MockSubaccountClient) MoveSubaccounts(_ context.Context) accountclient.ApiMoveSubaccountsRequest {
	panic("not implemented")
}
func (m *MockSubaccountClient) MoveSubaccountsExecute(_ accountclient.ApiMoveSubaccountsRequest) (*accountclient.ResponseCollection, *http.Response, error) {
	panic("not implemented")
}

// --- CR Builders ---

type saModifier func(*base.BaseSubaccount)

func newSubaccount(name string, mods ...saModifier) *base.BaseSubaccount {
	cr := &base.BaseSubaccount{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}
	for _, m := range mods {
		m(cr)
	}
	return cr
}

func withExternalName(name string) saModifier {
	return func(cr *base.BaseSubaccount) {
		meta.SetExternalName(cr, name)
	}
}

func withStatus(obs base.BaseSubaccountObservation) saModifier {
	return func(cr *base.BaseSubaccount) {
		cr.Status.AtProvider = obs
	}
}

func withSpec(params base.BaseSubaccountParameters) saModifier {
	return func(cr *base.BaseSubaccount) {
		cr.Spec.ForProvider = params
	}
}

func newTestClient(mock *MockSubaccountClient) Client {
	return Client{
		btp: btp.Client{
			AccountsServiceClient: &accountclient.APIClient{
				SubaccountOperationsAPI: mock,
			},
		},
	}
}

// --- Tests ---

func TestObserve(t *testing.T) {
	type args struct {
		client Client
		cr     *base.BaseSubaccount
	}
	type want struct {
		o   managed.ExternalObservation
		err error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"EmptyExternalNameNeedsCreation": {
			reason: "Empty external name indicates not found",
			args: args{
				client: newTestClient(&MockSubaccountClient{}),
				cr:     newSubaccount("unittest-sa"),
			},
			want: want{
				o: managed.ExternalObservation{ResourceExists: false},
			},
		},
		"InvalidGUIDFormatError": {
			reason: "Invalid GUID format in external-name should return error",
			args: args{
				client: newTestClient(&MockSubaccountClient{}),
				cr:     newSubaccount("unittest-sa", withExternalName("invalid-guid-format")),
			},
			want: want{
				o:   managed.ExternalObservation{},
				err: errors.New("external-name 'invalid-guid-format' is not a valid GUID"),
			},
		},
		"FindSubaccountError": {
			reason: "Get Subaccount error (non-404) should propagate",
			args: args{
				client: newTestClient(&MockSubaccountClient{
					getSubaccountErr: errors.New("Error getting subaccount"),
				}),
				cr: newSubaccount("unittest-sa", withExternalName(SAMPLE_GUID)),
			},
			want: want{
				o:   managed.ExternalObservation{},
				err: errors.New("Error getting subaccount"),
			},
		},
		"404ReturnsResourceExistsFalse": {
			reason: "404 response should reset state and indicate needs creation",
			args: args{
				client: newTestClient(&MockSubaccountClient{
					getSubaccountErr: errors.New("404 not found"),
				}),
				cr: newSubaccount("unittest-sa", withExternalName(SAMPLE_GUID)),
			},
			want: want{
				o: managed.ExternalObservation{ResourceExists: false},
			},
		},
		"DontUpdateEmptyDescription": {
			reason: "Empty description should NOT require Update",
			args: args{
				client: newTestClient(&MockSubaccountClient{
					returnSubaccount: &accountclient.SubaccountResponseObject{
						Guid:              SAMPLE_GUID,
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
				}),
				cr: newSubaccount("unittest-sa",
					withExternalName(SAMPLE_GUID),
					withSpec(base.BaseSubaccountParameters{
						Subdomain:         "sub1",
						Region:            "eu12",
						DisplayName:       "unittest-sa",
						UsedForProduction: "",
						BetaEnabled:       false,
					}),
				),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: true,
				},
			},
		},
		"NeedsUpdateDescription": {
			reason: "Changed description should require Update",
			args: args{
				client: newTestClient(&MockSubaccountClient{
					returnSubaccount: &accountclient.SubaccountResponseObject{
						Guid:              SAMPLE_GUID,
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
				}),
				cr: newSubaccount("unittest-sa",
					withExternalName(SAMPLE_GUID),
					withSpec(base.BaseSubaccountParameters{
						Description:       "someDesc",
						Subdomain:         "sub1",
						Region:            "eu12",
						DisplayName:       "unittest-sa",
						UsedForProduction: "",
						BetaEnabled:       false,
					}),
				),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: false,
				},
			},
		},
		"NeedsUpdateDisplayName": {
			reason: "Changed display name should require Update",
			args: args{
				client: newTestClient(&MockSubaccountClient{
					returnSubaccount: &accountclient.SubaccountResponseObject{
						Guid:              SAMPLE_GUID,
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
				}),
				cr: newSubaccount("unittest-sa",
					withExternalName(SAMPLE_GUID),
					withSpec(base.BaseSubaccountParameters{
						Description:       "someDesc",
						Subdomain:         "sub1",
						Region:            "eu12",
						DisplayName:       "unittest-sa",
						UsedForProduction: "",
						BetaEnabled:       false,
					}),
				),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: false,
				},
			},
		},
		"NeedsUpdateBetaEnabled": {
			reason: "Changed beta enabled toggle should require Update",
			args: args{
				client: newTestClient(&MockSubaccountClient{
					returnSubaccount: &accountclient.SubaccountResponseObject{
						Guid:              SAMPLE_GUID,
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
				}),
				cr: newSubaccount("unittest-sa",
					withExternalName(SAMPLE_GUID),
					withSpec(base.BaseSubaccountParameters{
						Description:       "someDesc",
						Subdomain:         "sub1",
						Region:            "eu12",
						DisplayName:       "unittest-sa",
						UsedForProduction: "",
						BetaEnabled:       false,
					}),
				),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: false,
				},
			},
		},
		"NeedsUpdateUsedForProduction": {
			reason: "Changed UsedForProduction toggle should require Update",
			args: args{
				client: newTestClient(&MockSubaccountClient{
					returnSubaccount: &accountclient.SubaccountResponseObject{
						Guid:              SAMPLE_GUID,
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
				}),
				cr: newSubaccount("unittest-sa",
					withExternalName(SAMPLE_GUID),
					withSpec(base.BaseSubaccountParameters{
						Description:       "someDesc",
						Subdomain:         "sub1",
						Region:            "eu12",
						DisplayName:       "unittest-sa",
						UsedForProduction: "NOT_USED_FOR_PRODUCTION",
						BetaEnabled:       true,
					}),
				),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: false,
				},
			},
		},
		"NeedsUpdateBetweenDirectories": {
			reason: "Changed Directory GUID should require Update",
			args: args{
				client: newTestClient(&MockSubaccountClient{
					returnSubaccount: &accountclient.SubaccountResponseObject{
						Guid:              SAMPLE_GUID,
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
				}),
				cr: newSubaccount("unittest-sa",
					withExternalName(SAMPLE_GUID),
					withSpec(base.BaseSubaccountParameters{
						Description:       "someDesc",
						Subdomain:         "sub1",
						Region:            "eu12",
						DisplayName:       "unittest-sa",
						UsedForProduction: "",
						BetaEnabled:       false,
						DirectoryGuid:     "234",
					}),
				),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: false,
				},
			},
		},
		"NeedsUpdateFromGlobalToDirectory": {
			reason: "Changed Directory GUID from global account needs update",
			args: args{
				client: newTestClient(&MockSubaccountClient{
					returnSubaccount: &accountclient.SubaccountResponseObject{
						Guid:              SAMPLE_GUID,
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
				}),
				cr: newSubaccount("unittest-sa",
					withExternalName(SAMPLE_GUID),
					withSpec(base.BaseSubaccountParameters{
						Description:       "someDesc",
						Subdomain:         "sub1",
						Region:            "eu12",
						DisplayName:       "unittest-sa",
						UsedForProduction: "",
						BetaEnabled:       false,
						DirectoryGuid:     "234",
					}),
				),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: false,
				},
			},
		},
		"NeedsUpdateFromDirectoryToGlobal": {
			reason: "Empty Directory GUID with subaccount in a directory needs update (move to global)",
			args: args{
				client: newTestClient(&MockSubaccountClient{
					returnSubaccount: &accountclient.SubaccountResponseObject{
						Guid:              SAMPLE_GUID,
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
				}),
				cr: newSubaccount("unittest-sa",
					withExternalName(SAMPLE_GUID),
					withSpec(base.BaseSubaccountParameters{
						Description:       "someDesc",
						Subdomain:         "sub1",
						Region:            "eu12",
						DisplayName:       "unittest-sa",
						UsedForProduction: "",
						BetaEnabled:       false,
					}),
				),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: false,
				},
			},
		},
		"UpToDate": {
			reason: "No differences should indicate resource is up to date",
			args: args{
				client: newTestClient(&MockSubaccountClient{
					returnSubaccount: &accountclient.SubaccountResponseObject{
						Guid:              SAMPLE_GUID,
						Description:       "someDesc",
						Subdomain:         "sub1",
						Region:            "eu12",
						State:             "OK",
						Labels:            &map[string][]string{},
						StateMessage:      internal.Ptr("OK"),
						DisplayName:       "unittest-sa",
						UsedForProduction: "",
						BetaEnabled:       false,
						ParentGUID:        "parent-1",
						GlobalAccountGUID: "parent-1",
					},
				}),
				cr: newSubaccount("unittest-sa",
					withExternalName(SAMPLE_GUID),
					withSpec(base.BaseSubaccountParameters{
						Description:       "someDesc",
						Subdomain:         "sub1",
						Region:            "eu12",
						DisplayName:       "unittest-sa",
						UsedForProduction: "",
						BetaEnabled:       false,
					}),
				),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: true,
				},
			},
		},
		"UpToDateWithinDirectory": {
			reason: "Subaccount in correct directory should show up-to-date",
			args: args{
				client: newTestClient(&MockSubaccountClient{
					returnSubaccount: &accountclient.SubaccountResponseObject{
						Guid:              SAMPLE_GUID,
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
				}),
				cr: newSubaccount("unittest-sa",
					withExternalName(SAMPLE_GUID),
					withSpec(base.BaseSubaccountParameters{
						Description:       "someDesc",
						Subdomain:         "sub1",
						Region:            "eu12",
						DisplayName:       "unittest-sa",
						UsedForProduction: "",
						BetaEnabled:       false,
						DirectoryGuid:     "234",
					}),
				),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: true,
				},
			},
		},
		"UpToDateWithDirectoryGUID": {
			reason: "Directly referencing a directory via GUID should also work (without name ref)",
			args: args{
				client: newTestClient(&MockSubaccountClient{
					returnSubaccount: &accountclient.SubaccountResponseObject{
						Guid:              SAMPLE_GUID,
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
				}),
				cr: newSubaccount("unittest-sa",
					withExternalName(SAMPLE_GUID),
					withSpec(base.BaseSubaccountParameters{
						Description:       "someDesc",
						Subdomain:         "sub1",
						Region:            "eu12",
						DisplayName:       "unittest-sa",
						UsedForProduction: "",
						BetaEnabled:       false,
						DirectoryGuid:     "234",
					}),
				),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: true,
				},
			},
		},
		"UpToDateDespiteDifferentLabelNilTypes": {
			reason: "Labels pointer type mismatch should not lead to unexpected comparison results",
			args: args{
				client: newTestClient(&MockSubaccountClient{
					returnSubaccount: &accountclient.SubaccountResponseObject{
						Guid:              SAMPLE_GUID,
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
				}),
				cr: newSubaccount("unittest-sa",
					withExternalName(SAMPLE_GUID),
					withSpec(base.BaseSubaccountParameters{
						Description:       "someDesc",
						Subdomain:         "sub1",
						Region:            "eu12",
						DisplayName:       "unittest-sa",
						UsedForProduction: "",
						BetaEnabled:       false,
						DirectoryGuid:     "234",
						Labels:            nil,
					}),
				),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: true,
				},
			},
		},
		"NeedsUpdateLabel": {
			reason: "Adding label to an existing subaccount should require Update",
			args: args{
				client: newTestClient(&MockSubaccountClient{
					returnSubaccount: &accountclient.SubaccountResponseObject{
						Guid:              SAMPLE_GUID,
						Description:       "someDesc",
						Subdomain:         "sub1",
						Region:            "eu12",
						State:             "OK",
						Labels:            nil,
						StateMessage:      internal.Ptr("OK"),
						DisplayName:       "unittest-sa",
						UsedForProduction: "",
						BetaEnabled:       false,
						ParentGUID:        "parent-1",
						GlobalAccountGUID: "parent-1",
					},
				}),
				cr: newSubaccount("unittest-sa",
					withExternalName(SAMPLE_GUID),
					withSpec(base.BaseSubaccountParameters{
						Description:       "someDesc",
						Subdomain:         "sub1",
						Region:            "eu12",
						DisplayName:       "unittest-sa",
						Labels:            map[string][]string{"somekey": {"somevalue"}},
						UsedForProduction: "",
						BetaEnabled:       false,
					}),
				),
			},
			want: want{
				o: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: false,
				},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := Observe(tc.args.client, context.Background(), nil, tc.args.cr)
			if contained := testutils.ContainsError(err, tc.want.err); !contained {
				t.Errorf("\n%s\nObserve(...): error \"%v\" not part of \"%v\"", tc.reason, err, tc.want.err)
			}
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\n%s\nObserve(...): -want, +got:\n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestCreate(t *testing.T) {
	type args struct {
		client Client
		cr     *base.BaseSubaccount
	}
	type want struct {
		err error
		o   managed.ExternalCreation
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"RunningCreation": {
			reason: "Return gracefully if creation is already triggered",
			args: args{
				client: newTestClient(&MockSubaccountClient{}),
				cr: newSubaccount("unittest-sa",
					withStatus(base.BaseSubaccountObservation{Status: internal.Ptr("STARTED")})),
			},
			want: want{
				o: managed.ExternalCreation{},
			},
		},
		"APIErrorBadRequest": {
			reason: "API Error should prevent creation",
			args: args{
				client: newTestClient(&MockSubaccountClient{
					returnSubaccount: &accountclient.SubaccountResponseObject{},
					returnErr:        errors.New("badRequestError"),
				}),
				cr: newSubaccount("unittest-sa"),
			},
			want: want{
				o:   managed.ExternalCreation{},
				err: errors.New("badRequestError"),
			},
		},
		"CreateSuccess": {
			reason: "Should cache status and set external name on success",
			args: args{
				client: newTestClient(&MockSubaccountClient{
					returnSubaccount: &accountclient.SubaccountResponseObject{
						Guid:         "123",
						StateMessage: internal.Ptr("Success"),
					},
				}),
				cr: newSubaccount("unittest-sa"),
			},
			want: want{
				o: managed.ExternalCreation{},
			},
		},
		"MapDirectoryGuid": {
			reason: "DirectoryID needs to be passed as payload to API",
			args: args{
				client: newTestClient(&MockSubaccountClient{
					returnSubaccount: &accountclient.SubaccountResponseObject{
						Guid:         "123",
						StateMessage: internal.Ptr("Success"),
						ParentGUID:   "234",
					},
				}),
				cr: newSubaccount("unittest-sa",
					withSpec(base.BaseSubaccountParameters{DirectoryGuid: "234"})),
			},
			want: want{
				o: managed.ExternalCreation{},
			},
		},
		"ResourceAlreadyExistsError": {
			reason: "409 Conflict should NOT set external-name",
			args: args{
				client: newTestClient(&MockSubaccountClient{
					returnSubaccount: &accountclient.SubaccountResponseObject{},
					returnErr:        create409Error(),
					httpStatusCode:   http.StatusConflict,
				}),
				cr: newSubaccount("unittest-sa"),
			},
			want: want{
				o:   managed.ExternalCreation{},
				err: errors.New("creation failed - resource already exists. Please set external-name annotation to adopt the existing resource"),
			},
		},
		"OtherCreationError": {
			reason: "Other creation errors should NOT set external-name",
			args: args{
				client: newTestClient(&MockSubaccountClient{
					returnSubaccount: &accountclient.SubaccountResponseObject{},
					returnErr:        errors.New("500 Internal Server Error"),
				}),
				cr: newSubaccount("unittest-sa"),
			},
			want: want{
				o:   managed.ExternalCreation{},
				err: errors.New("500 Internal Server Error"),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := Create(tc.args.client, context.Background(), nil, tc.args.cr)
			if contained := testutils.ContainsError(err, tc.want.err); !contained {
				t.Errorf("\n%s\nCreate(...): error \"%v\" not part of \"%v\"", tc.reason, err, tc.want.err)
			}
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\n%s\nCreate(...): -want, +got:\n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestDelete(t *testing.T) {
	type args struct {
		client Client
		cr     *base.BaseSubaccount
	}
	type want struct {
		err error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"AsyncDeletionInProgress": {
			reason: "Deletion already in progress (DELETING state) should not trigger another DELETE API call",
			args: args{
				client: newTestClient(&MockSubaccountClient{
					mockDeleteSubaccountExecute: func(_ accountclient.ApiDeleteSubaccountRequest) (*accountclient.SubaccountResponseObject, *http.Response, error) {
						panic("should not be called")
					},
				}),
				cr: newSubaccount("unittest-sa",
					withExternalName(SAMPLE_GUID),
					withStatus(base.BaseSubaccountObservation{
						SubaccountGuid: internal.Ptr(SAMPLE_GUID),
						Status:         internal.Ptr("DELETING"),
					})),
			},
			want: want{
				err: nil,
			},
		},
		"DeleteSuccess": {
			reason: "Deletion should be successful",
			args: args{
				client: newTestClient(&MockSubaccountClient{
					mockDeleteSubaccountExecute: func(_ accountclient.ApiDeleteSubaccountRequest) (*accountclient.SubaccountResponseObject, *http.Response, error) {
						return &accountclient.SubaccountResponseObject{Guid: "123", State: "DELETING"}, &http.Response{StatusCode: 200}, nil
					},
				}),
				cr: newSubaccount("unittest-sa",
					withExternalName(SAMPLE_GUID),
					withStatus(base.BaseSubaccountObservation{SubaccountGuid: internal.Ptr("123")})),
			},
			want: want{
				err: nil,
			},
		},
		"DeleteAPI404": {
			reason: "Deletion should be successful if subaccount not found",
			args: args{
				client: newTestClient(&MockSubaccountClient{
					mockDeleteSubaccountExecute: func(_ accountclient.ApiDeleteSubaccountRequest) (*accountclient.SubaccountResponseObject, *http.Response, error) {
						return &accountclient.SubaccountResponseObject{}, &http.Response{StatusCode: 404}, nil
					},
				}),
				cr: newSubaccount("unittest-sa",
					withExternalName(SAMPLE_GUID),
					withStatus(base.BaseSubaccountObservation{SubaccountGuid: internal.Ptr(SAMPLE_GUID)})),
			},
			want: want{
				err: nil,
			},
		},
		"DeleteAPIError": {
			reason: "Deletion should fail if API returns error",
			args: args{
				client: newTestClient(&MockSubaccountClient{
					mockDeleteSubaccountExecute: func(_ accountclient.ApiDeleteSubaccountRequest) (*accountclient.SubaccountResponseObject, *http.Response, error) {
						return &accountclient.SubaccountResponseObject{}, &http.Response{StatusCode: 500}, errors.New("apiError")
					},
				}),
				cr: newSubaccount("unittest-sa",
					withExternalName(SAMPLE_GUID),
					withStatus(base.BaseSubaccountObservation{SubaccountGuid: internal.Ptr(SAMPLE_GUID)})),
			},
			want: want{
				err: errors.New("apiError"),
			},
		},
		"ExternallyRemovedResource": {
			reason: "Resource already deleted externally (404) should not be treated as error",
			args: args{
				client: newTestClient(&MockSubaccountClient{
					mockDeleteSubaccountExecute: func(_ accountclient.ApiDeleteSubaccountRequest) (*accountclient.SubaccountResponseObject, *http.Response, error) {
						return &accountclient.SubaccountResponseObject{}, &http.Response{StatusCode: 404}, nil
					},
				}),
				cr: newSubaccount("unittest-sa",
					withExternalName(SAMPLE_GUID),
					withStatus(base.BaseSubaccountObservation{
						SubaccountGuid: internal.Ptr(SAMPLE_GUID),
						Status:         internal.Ptr("OK"),
					})),
			},
			want: want{
				err: nil,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Delete(tc.args.client, context.Background(), nil, tc.args.cr)
			if contained := testutils.ContainsError(err, tc.want.err); !contained {
				t.Errorf("\n%s\nDelete(...): error \"%v\" not part of \"%v\"", tc.reason, err, tc.want.err)
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	type args struct {
		client Client
		cr     *base.BaseSubaccount
	}
	type want struct {
		err error
		o   managed.ExternalUpdate
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"SkipOnCreating": {
			reason: "Return gracefully if subaccount is still being created",
			args: args{
				client: newTestClient(&MockSubaccountClient{}),
				cr: newSubaccount("unittest-sa",
					withStatus(base.BaseSubaccountObservation{Status: internal.Ptr("CREATING")})),
			},
			want: want{
				o: managed.ExternalUpdate{},
			},
		},
		"BasicUpdateError": {
			reason: "Error from update API should be propagated",
			args: args{
				client: newTestClient(&MockSubaccountClient{
					returnErr: errors.New("apiError"),
				}),
				cr: newSubaccount("unittest-sa",
					withExternalName(SAMPLE_GUID),
					withSpec(base.BaseSubaccountParameters{
						DirectoryGuid: "234",
					}),
					withStatus(base.BaseSubaccountObservation{
						SubaccountGuid: internal.Ptr(SAMPLE_GUID),
						ParentGuid:     internal.Ptr("234"),
					}),
				),
			},
			want: want{
				o:   managed.ExternalUpdate{},
				err: errors.New("apiError"),
			},
		},
		"BasicUpdateSuccess": {
			reason: "Successful update returns no error",
			args: args{
				client: newTestClient(&MockSubaccountClient{
					returnSubaccount: &accountclient.SubaccountResponseObject{},
				}),
				cr: newSubaccount("unittest-sa",
					withExternalName(SAMPLE_GUID),
					withSpec(base.BaseSubaccountParameters{
						DirectoryGuid: "234",
					}),
					withStatus(base.BaseSubaccountObservation{
						SubaccountGuid: internal.Ptr(SAMPLE_GUID),
						ParentGuid:     internal.Ptr("234"),
					}),
				),
			},
			want: want{
				o: managed.ExternalUpdate{},
			},
		},
		"MoveSubaccount": {
			reason: "Directory change should trigger move API",
			args: args{
				client: newTestClient(&MockSubaccountClient{
					returnSubaccount: &accountclient.SubaccountResponseObject{},
				}),
				cr: newSubaccount("unittest-sa",
					withExternalName(SAMPLE_GUID),
					withSpec(base.BaseSubaccountParameters{
						DirectoryGuid: "new-dir-123",
					}),
					withStatus(base.BaseSubaccountObservation{
						SubaccountGuid:    internal.Ptr(SAMPLE_GUID),
						ParentGuid:        internal.Ptr("old-dir-456"),
						GlobalAccountGUID: internal.Ptr("global-123"),
					}),
				),
			},
			want: want{
				o: managed.ExternalUpdate{},
			},
		},
		"MoveToGlobalAccount": {
			reason: "Empty directory GUID should move to global account",
			args: args{
				client: newTestClient(&MockSubaccountClient{
					returnSubaccount: &accountclient.SubaccountResponseObject{},
				}),
				cr: newSubaccount("unittest-sa",
					withExternalName(SAMPLE_GUID),
					withSpec(base.BaseSubaccountParameters{
						DirectoryGuid: "",
					}),
					withStatus(base.BaseSubaccountObservation{
						SubaccountGuid:    internal.Ptr(SAMPLE_GUID),
						GlobalAccountGUID: internal.Ptr("global-123"),
						ParentGuid:        internal.Ptr("dir-123"),
					}),
				),
			},
			want: want{
				o: managed.ExternalUpdate{},
			},
		},
		"BasicUpdateSuccessWithLabels": {
			reason: "Update with labels should succeed",
			args: args{
				client: newTestClient(&MockSubaccountClient{
					returnSubaccount: &accountclient.SubaccountResponseObject{},
				}),
				cr: newSubaccount("unittest-sa",
					withExternalName(SAMPLE_GUID),
					withSpec(base.BaseSubaccountParameters{
						DirectoryGuid: "234",
						Labels:        map[string][]string{"somekey": {"somevalue"}},
					}),
					withStatus(base.BaseSubaccountObservation{
						SubaccountGuid: internal.Ptr(SAMPLE_GUID),
						ParentGuid:     internal.Ptr("234"),
					}),
				),
			},
			want: want{
				o: managed.ExternalUpdate{},
			},
		},
		"MoveAccountError": {
			reason: "Error attempting to move subaccount should propagate",
			args: args{
				client: newTestClient(&MockSubaccountClient{
					returnErr: errors.New("apiError"),
				}),
				cr: newSubaccount("unittest-sa",
					withExternalName(SAMPLE_GUID),
					withSpec(base.BaseSubaccountParameters{
						DirectoryGuid: "345",
					}),
					withStatus(base.BaseSubaccountObservation{
						SubaccountGuid: internal.Ptr(SAMPLE_GUID),
						ParentGuid:     internal.Ptr("234"),
					}),
				),
			},
			want: want{
				o:   managed.ExternalUpdate{},
				err: errors.New("apiError"),
			},
		},
		"LabelUpdateSuccess": {
			reason: "Removing label from subaccount should succeed",
			args: args{
				client: newTestClient(&MockSubaccountClient{
					returnSubaccount: &accountclient.SubaccountResponseObject{},
				}),
				cr: newSubaccount("unittest-sa",
					withExternalName(SAMPLE_GUID),
					withSpec(base.BaseSubaccountParameters{
						Labels: nil,
					}),
					withStatus(base.BaseSubaccountObservation{
						Labels:            &map[string][]string{"somekey": {"somevalue"}},
						ParentGuid:        internal.Ptr("parent-1"),
						GlobalAccountGUID: internal.Ptr("parent-1"),
					}),
				),
			},
			want: want{
				o: managed.ExternalUpdate{},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := Update(tc.args.client, context.Background(), nil, tc.args.cr)
			if contained := testutils.ContainsError(err, tc.want.err); !contained {
				t.Errorf("\n%s\nUpdate(...): error \"%v\" not part of \"%v\"", tc.reason, err, tc.want.err)
			}
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\n%s\nUpdate(...): -want, +got:\n%s\n", tc.reason, diff)
			}
		})
	}
}

func create409Error() error {
	apiExceptionError := accountclient.NewApiExceptionResponseObjectError()
	apiExceptionError.SetCode(409)

	apiException := accountclient.NewApiExceptionResponseObject(*apiExceptionError)

	err := &accountclient.GenericOpenAPIError{}
	errValue := reflect.ValueOf(err).Elem()

	modelField := errValue.FieldByName("model")
	if modelField.IsValid() {
		reflect.NewAt(modelField.Type(), unsafe.Pointer(modelField.UnsafeAddr())).
			Elem().Set(reflect.ValueOf(*apiException))
	}

	errorField := errValue.FieldByName("error")
	if errorField.IsValid() {
		reflect.NewAt(errorField.Type(), unsafe.Pointer(errorField.UnsafeAddr())).
			Elem().SetString("409 Conflict")
	}

	return err
}

func TestMigrateExternalName(t *testing.T) {
	type args struct {
		client Client
		mg     *fake.Managed
		cr     *base.BaseSubaccount
		kube   client.Client
	}
	type want struct {
		err          error
		externalName string
	}

	newManagedWithExternalName := func(name string) *fake.Managed {
		mg := &fake.Managed{}
		if name != "" {
			meta.SetExternalName(mg, name)
		}
		return mg
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"AlreadyValidGUID": {
			reason: "If external-name is already a valid GUID, no migration needed",
			args: args{
				client: newTestClient(&MockSubaccountClient{}),
				mg:     newManagedWithExternalName(SAMPLE_GUID),
				cr: newSubaccount("unittest-sa",
					withSpec(base.BaseSubaccountParameters{
						Subdomain: "unittest-sa",
						Region:    "eu10",
					}),
				),
				kube: &test.MockClient{},
			},
			want: want{
				externalName: SAMPLE_GUID,
			},
		},
		"EmptyExternalName": {
			reason: "Empty external-name should not trigger migration",
			args: args{
				client: newTestClient(&MockSubaccountClient{}),
				mg:     newManagedWithExternalName(""),
				cr:     newSubaccount("unittest-sa"),
				kube:   &test.MockClient{},
			},
			want: want{
				externalName: "",
			},
		},
		"MigrationSuccess": {
			reason: "Resource with name as external-name should be found by subdomain+region and external-name should be updated to GUID",
			args: args{
				client: newTestClient(&MockSubaccountClient{
					returnSubaccounts: &accountclient.ResponseCollection{
						Value: []accountclient.SubaccountResponseObject{
							{
								Guid:      SAMPLE_GUID,
								Subdomain: "unittest-sa",
								Region:    "eu10",
							},
						},
					},
				}),
				mg: newManagedWithExternalName("unittest-sa"),
				cr: newSubaccount("unittest-sa",
					withSpec(base.BaseSubaccountParameters{
						Subdomain: "unittest-sa",
						Region:    "eu10",
					}),
				),
				kube: &test.MockClient{
					MockUpdate: test.NewMockUpdateFn(nil),
				},
			},
			want: want{
				externalName: SAMPLE_GUID,
			},
		},
		"MigrationNotFound": {
			reason: "Resource with name as external-name should not be migrated if no matching subaccount found",
			args: args{
				client: newTestClient(&MockSubaccountClient{
					returnSubaccounts: &accountclient.ResponseCollection{
						Value: []accountclient.SubaccountResponseObject{},
					},
				}),
				mg: newManagedWithExternalName("unittest-sa"),
				cr: newSubaccount("unittest-sa",
					withSpec(base.BaseSubaccountParameters{
						Subdomain: "unittest-sa",
						Region:    "eu10",
					}),
				),
				kube: &test.MockClient{
					MockUpdate: test.NewMockUpdateFn(nil),
				},
			},
			want: want{
				externalName: "unittest-sa",
			},
		},
		"MigrationAPIError": {
			reason: "API error during migration should propagate",
			args: args{
				client: newTestClient(&MockSubaccountClient{
					returnErr: errors.New("api error"),
				}),
				mg: newManagedWithExternalName("unittest-sa"),
				cr: newSubaccount("unittest-sa",
					withSpec(base.BaseSubaccountParameters{
						Subdomain: "unittest-sa",
						Region:    "eu10",
					}),
				),
				kube: &test.MockClient{},
			},
			want: want{
				err:          errors.New("api error"),
				externalName: "unittest-sa",
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := MigrateExternalName(tc.args.client, context.Background(), tc.args.mg, tc.args.cr, tc.args.kube)
			if contained := testutils.ContainsError(err, tc.want.err); !contained {
				t.Errorf("\n%s\nMigrateExternalName(...): error \"%v\" not part of \"%v\"", tc.reason, err, tc.want.err)
			}
			if got := meta.GetExternalName(tc.args.mg); got != tc.want.externalName {
				t.Errorf("\n%s\nMigrateExternalName(...): external-name = %q, want %q", tc.reason, got, tc.want.externalName)
			}
		})
	}
}
