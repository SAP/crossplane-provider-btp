package directory

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	"github.com/google/go-cmp/cmp"
	stderrors "errors"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	base "github.com/sap/crossplane-provider-btp/apis/base/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/internal"
	accountclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-accounts-service-api-go/pkg"
)

const validGUID = "12345678-1234-1234-1234-123456789012"

// --- Mock DirectoryOperationsAPI ---

type MockDirectoryAPI struct {
	getResponse    *accountclient.DirectoryResponseObject
	getHTTP        *http.Response
	getErr         error
	createResponse *accountclient.DirectoryResponseObject
	createHTTP     *http.Response
	createErr      error
	deleteResponse *accountclient.DirectoryResponseObject
	deleteHTTP     *http.Response
	deleteErr      error
	updateResponse *accountclient.DirectoryResponseObject
	updateHTTP     *http.Response
	updateErr      error
	updateFeaturesResponse *accountclient.DirectoryResponseObject
	updateFeaturesHTTP     *http.Response
	updateFeaturesErr      error
}

var _ accountclient.DirectoryOperationsAPI = &MockDirectoryAPI{}

func (m *MockDirectoryAPI) GetDirectory(_ context.Context, _ string) accountclient.ApiGetDirectoryRequest {
	return accountclient.ApiGetDirectoryRequest{ApiService: m}
}
func (m *MockDirectoryAPI) GetDirectoryExecute(_ accountclient.ApiGetDirectoryRequest) (*accountclient.DirectoryResponseObject, *http.Response, error) {
	return m.getResponse, m.getHTTP, m.getErr
}
func (m *MockDirectoryAPI) CreateDirectory(_ context.Context) accountclient.ApiCreateDirectoryRequest {
	return accountclient.ApiCreateDirectoryRequest{ApiService: m}
}
func (m *MockDirectoryAPI) CreateDirectoryExecute(_ accountclient.ApiCreateDirectoryRequest) (*accountclient.DirectoryResponseObject, *http.Response, error) {
	return m.createResponse, m.createHTTP, m.createErr
}
func (m *MockDirectoryAPI) DeleteDirectory(_ context.Context, _ string) accountclient.ApiDeleteDirectoryRequest {
	return accountclient.ApiDeleteDirectoryRequest{ApiService: m}
}
func (m *MockDirectoryAPI) DeleteDirectoryExecute(_ accountclient.ApiDeleteDirectoryRequest) (*accountclient.DirectoryResponseObject, *http.Response, error) {
	return m.deleteResponse, m.deleteHTTP, m.deleteErr
}
func (m *MockDirectoryAPI) UpdateDirectory(_ context.Context, _ string) accountclient.ApiUpdateDirectoryRequest {
	return accountclient.ApiUpdateDirectoryRequest{ApiService: m}
}
func (m *MockDirectoryAPI) UpdateDirectoryExecute(_ accountclient.ApiUpdateDirectoryRequest) (*accountclient.DirectoryResponseObject, *http.Response, error) {
	return m.updateResponse, m.updateHTTP, m.updateErr
}
func (m *MockDirectoryAPI) UpdateDirectoryFeatures(_ context.Context, _ string) accountclient.ApiUpdateDirectoryFeaturesRequest {
	return accountclient.ApiUpdateDirectoryFeaturesRequest{ApiService: m}
}
func (m *MockDirectoryAPI) UpdateDirectoryFeaturesExecute(_ accountclient.ApiUpdateDirectoryFeaturesRequest) (*accountclient.DirectoryResponseObject, *http.Response, error) {
	return m.updateFeaturesResponse, m.updateFeaturesHTTP, m.updateFeaturesErr
}

// Unused methods — panic on call
func (m *MockDirectoryAPI) CreateDirectoryLabels(_ context.Context, _ string) accountclient.ApiCreateDirectoryLabelsRequest {
	panic("not implemented")
}
func (m *MockDirectoryAPI) CreateDirectoryLabelsExecute(_ accountclient.ApiCreateDirectoryLabelsRequest) (*accountclient.LabelsResponseObject, *http.Response, error) {
	panic("not implemented")
}
func (m *MockDirectoryAPI) CreateOrUpdateDirectorySettings(_ context.Context, _ string) accountclient.ApiCreateOrUpdateDirectorySettingsRequest {
	panic("not implemented")
}
func (m *MockDirectoryAPI) CreateOrUpdateDirectorySettingsExecute(_ accountclient.ApiCreateOrUpdateDirectorySettingsRequest) (*accountclient.DataResponseObject, *http.Response, error) {
	panic("not implemented")
}
func (m *MockDirectoryAPI) DeleteDirectoryLabels(_ context.Context, _ string) accountclient.ApiDeleteDirectoryLabelsRequest {
	panic("not implemented")
}
func (m *MockDirectoryAPI) DeleteDirectoryLabelsExecute(_ accountclient.ApiDeleteDirectoryLabelsRequest) (*accountclient.LabelsResponseObject, *http.Response, error) {
	panic("not implemented")
}
func (m *MockDirectoryAPI) DeleteDirectorySettings(_ context.Context, _ string) accountclient.ApiDeleteDirectorySettingsRequest {
	panic("not implemented")
}
func (m *MockDirectoryAPI) DeleteDirectorySettingsExecute(_ accountclient.ApiDeleteDirectorySettingsRequest) (*accountclient.DataResponseObject, *http.Response, error) {
	panic("not implemented")
}
func (m *MockDirectoryAPI) GetDirectoryCustomProperties(_ context.Context, _ string) accountclient.ApiGetDirectoryCustomPropertiesRequest {
	panic("not implemented")
}
func (m *MockDirectoryAPI) GetDirectoryCustomPropertiesExecute(_ accountclient.ApiGetDirectoryCustomPropertiesRequest) (*accountclient.ResponseCollection, *http.Response, error) {
	panic("not implemented")
}
func (m *MockDirectoryAPI) GetDirectoryLabels(_ context.Context, _ string) accountclient.ApiGetDirectoryLabelsRequest {
	panic("not implemented")
}
func (m *MockDirectoryAPI) GetDirectoryLabelsExecute(_ accountclient.ApiGetDirectoryLabelsRequest) (*accountclient.LabelsResponseObject, *http.Response, error) {
	panic("not implemented")
}
func (m *MockDirectoryAPI) GetDirectorySettings(_ context.Context, _ string) accountclient.ApiGetDirectorySettingsRequest {
	panic("not implemented")
}
func (m *MockDirectoryAPI) GetDirectorySettingsExecute(_ accountclient.ApiGetDirectorySettingsRequest) (*accountclient.DataResponseObject, *http.Response, error) {
	panic("not implemented")
}

// --- CR Builders ---

type dirModifier func(*base.BaseDirectory)

func newDir(name string, mods ...dirModifier) *base.BaseDirectory {
	cr := &base.BaseDirectory{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}
	for _, m := range mods {
		m(cr)
	}
	return cr
}

func withExternalName(name string) dirModifier {
	return func(cr *base.BaseDirectory) {
		meta.SetExternalName(cr, name)
	}
}

func withStatus(obs base.BaseDirectoryObservation) dirModifier {
	return func(cr *base.BaseDirectory) {
		cr.Status.AtProvider = obs
	}
}

func withConditions(c ...xpv1.Condition) dirModifier {
	return func(cr *base.BaseDirectory) {
		cr.Status.Conditions = c
	}
}

func withSpec(params base.BaseDirectoryParameters) dirModifier {
	return func(cr *base.BaseDirectory) {
		cr.Spec.ForProvider = params
	}
}

func newTestClient(mock *MockDirectoryAPI) Client {
	return Client{
		btp: btp.Client{
			AccountsServiceClient: &accountclient.APIClient{
				DirectoryOperationsAPI: mock,
			},
		},
	}
}

// --- Tests ---

func TestObserve(t *testing.T) {
	const invalidGUID = "not-a-valid-guid"

	type args struct {
		client Client
		cr     *base.BaseDirectory
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
			reason: "Empty external-name indicates resource needs creation",
			args: args{
				client: newTestClient(&MockDirectoryAPI{}),
				cr:     newDir("dir-unittests"),
			},
			want: want{
				o: managed.ExternalObservation{ResourceExists: false},
			},
		},
		"InvalidGUIDFormat": {
			reason: "External-name with invalid GUID format should return error",
			args: args{
				client: newTestClient(&MockDirectoryAPI{}),
				cr:     newDir("dir-unittests", withExternalName(invalidGUID)),
			},
			want: want{
				err: fmt.Errorf("external-name '%s' is not a valid GUID", invalidGUID),
			},
		},
		"APIErrorOnRead": {
			reason: "When GetDirectory returns an error we should propagate it",
			args: args{
				client: newTestClient(&MockDirectoryAPI{
					getErr: errors.New("internalServerError"),
				}),
				cr: newDir("dir-unittests", withExternalName(validGUID)),
			},
			want: want{
				o:   managed.ExternalObservation{},
				err: errors.New("internalServerError"),
			},
		},
		"NotFound404": {
			reason: "Valid GUID that doesn't exist (404 response) should return ResourceExists: false",
			args: args{
				client: newTestClient(&MockDirectoryAPI{
					getResponse: nil,
					getHTTP:     &http.Response{StatusCode: http.StatusNotFound},
					getErr:      nil,
				}),
				cr: newDir("dir-unittests", withExternalName(validGUID)),
			},
			want: want{
				o: managed.ExternalObservation{ResourceExists: false},
			},
		},
		"Unavailable": {
			reason: "If entity state does not indicate OK it's unavailable",
			args: args{
				client: newTestClient(&MockDirectoryAPI{
					getResponse: &accountclient.DirectoryResponseObject{
						Guid:              validGUID,
						DisplayName:       "dir-unittests",
						EntityState:       internal.Ptr("CREATING"),
						DirectoryFeatures: []string{"DEFAULT"},
					},
				}),
				cr: newDir("dir-unittests",
					withExternalName(validGUID),
					withSpec(base.BaseDirectoryParameters{
						DisplayName: internal.Ptr("dir-unittests"),
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
		"RequiresUpdate": {
			reason: "If spec doesn't match API response, resource needs update",
			args: args{
				client: newTestClient(&MockDirectoryAPI{
					getResponse: &accountclient.DirectoryResponseObject{
						Guid:              validGUID,
						DisplayName:       "old-name",
						Description:       internal.Ptr("old-desc"),
						EntityState:       internal.Ptr("OK"),
						DirectoryFeatures: []string{"DEFAULT"},
						Labels:            &map[string][]string{},
					},
				}),
				cr: newDir("dir-unittests",
					withExternalName(validGUID),
					withSpec(base.BaseDirectoryParameters{
						DisplayName:       internal.Ptr("new-name"),
						Description:       internal.Ptr("new-desc"),
						DirectoryFeatures: []string{"DEFAULT"},
						Labels:            map[string][]string{},
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
			reason: "If spec matches API response, resource is up to date",
			args: args{
				client: newTestClient(&MockDirectoryAPI{
					getResponse: &accountclient.DirectoryResponseObject{
						Guid:              validGUID,
						DisplayName:       "dir-unittests",
						Description:       internal.Ptr("some-desc"),
						EntityState:       internal.Ptr("OK"),
						DirectoryFeatures: []string{"DEFAULT"},
						Labels:            &map[string][]string{},
					},
				}),
				cr: newDir("dir-unittests",
					withExternalName(validGUID),
					withSpec(base.BaseDirectoryParameters{
						DisplayName:       internal.Ptr("dir-unittests"),
						Description:       internal.Ptr("some-desc"),
						DirectoryFeatures: []string{"DEFAULT"},
						Labels:            map[string][]string{},
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
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := Observe(tc.args.client, context.Background(), tc.args.cr)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nObserve(...): -want error, +got error:\n%s\n", tc.reason, diff)
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
		cr     *base.BaseDirectory
	}
	type want struct {
		o   managed.ExternalCreation
		err error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"Failure": {
			reason: "We expect to return an error if Create fails",
			args: args{
				client: newTestClient(&MockDirectoryAPI{
					createErr: errors.New("CreateError"),
				}),
				cr: newDir("dir-unittests", withSpec(base.BaseDirectoryParameters{
					DisplayName: internal.Ptr("dir-unittests"),
				})),
			},
			want: want{
				o:   managed.ExternalCreation{},
				err: errors.New("CreateError"),
			},
		},
		"AlreadyExistsError": {
			reason: "409 Conflict should return an informative error",
			args: args{
				client: newTestClient(&MockDirectoryAPI{
					createErr:  stderrors.New("conflict"),
					createHTTP: &http.Response{StatusCode: http.StatusConflict},
				}),
				cr: newDir("dir-unittests", withSpec(base.BaseDirectoryParameters{
					DisplayName: internal.Ptr("dir-unittests"),
				})),
			},
			want: want{
				err: fmt.Errorf("creation failed - directory already exists. Please set external-name annotation to adopt the existing resource: %w", stderrors.New("conflict")),
			},
		},
		"Success": {
			reason: "Successful creation should set external name from response",
			args: args{
				client: newTestClient(&MockDirectoryAPI{
					createResponse: &accountclient.DirectoryResponseObject{
						Guid:        validGUID,
						DisplayName: "dir-unittests",
					},
				}),
				cr: newDir("dir-unittests", withSpec(base.BaseDirectoryParameters{
					DisplayName: internal.Ptr("dir-unittests"),
				})),
			},
			want: want{
				o: managed.ExternalCreation{},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := Create(tc.args.client, context.Background(), tc.args.cr)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nCreate(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\n%s\nCreate(...): -want, +got:\n%s\n", tc.reason, diff)
			}
			if tc.want.err == nil && err == nil {
				if meta.GetExternalName(tc.args.cr) != validGUID {
					t.Errorf("Expected external-name to be set to %s, got %s", validGUID, meta.GetExternalName(tc.args.cr))
				}
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	type args struct {
		client Client
		cr     *base.BaseDirectory
	}
	type want struct {
		o   managed.ExternalUpdate
		err error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"NoExternalName": {
			reason: "Update without external name should return error",
			args: args{
				client: newTestClient(&MockDirectoryAPI{}),
				cr:     newDir("dir-unittests"),
			},
			want: want{
				o:   managed.ExternalUpdate{},
				err: stderrors.New("can not request API without GUID"),
			},
		},
		"UpdateDirectoryError": {
			reason: "Error from UpdateDirectory API should be propagated",
			args: args{
				client: newTestClient(&MockDirectoryAPI{
					updateErr: errors.New("updateError"),
				}),
				cr: newDir("dir-unittests",
					withExternalName(validGUID),
					withSpec(base.BaseDirectoryParameters{
						DisplayName: internal.Ptr("new-name"),
					}),
				),
			},
			want: want{
				o:   managed.ExternalUpdate{},
				err: errors.New("updateError"),
			},
		},
		"UpdateFeaturesError": {
			reason: "Error from UpdateDirectoryFeatures API should be propagated",
			args: args{
				client: newTestClient(&MockDirectoryAPI{
					updateFeaturesErr: errors.New("featuresError"),
				}),
				cr: newDir("dir-unittests",
					withExternalName(validGUID),
					withSpec(base.BaseDirectoryParameters{
						DisplayName:       internal.Ptr("name"),
						DirectoryFeatures: []string{"DEFAULT", "ENTITLEMENTS"},
					}),
				),
			},
			want: want{
				o:   managed.ExternalUpdate{},
				err: errors.New("featuresError"),
			},
		},
		"Success": {
			reason: "Successful update returns no error",
			args: args{
				client: newTestClient(&MockDirectoryAPI{
					updateResponse:         &accountclient.DirectoryResponseObject{},
					updateFeaturesResponse: &accountclient.DirectoryResponseObject{},
				}),
				cr: newDir("dir-unittests",
					withExternalName(validGUID),
					withSpec(base.BaseDirectoryParameters{
						DisplayName:       internal.Ptr("dir-unittests"),
						DirectoryFeatures: []string{"DEFAULT"},
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
			got, err := Update(tc.args.client, context.Background(), tc.args.cr)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nUpdate(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\n%s\nUpdate(...): -want, +got:\n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestDelete(t *testing.T) {
	type args struct {
		client Client
		cr     *base.BaseDirectory
	}
	type want struct {
		err error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"DeletingStateSkipsDelete": {
			reason: "Resource in DELETING state should skip deletion call",
			args: args{
				client: newTestClient(&MockDirectoryAPI{
					deleteErr: errors.New("should not be called"),
				}),
				cr: newDir("dir-unittests",
					withExternalName(validGUID),
					withStatus(base.BaseDirectoryObservation{
						Guid:        internal.Ptr(validGUID),
						EntityState: internal.Ptr("DELETING"),
					}),
				),
			},
			want: want{
				err: nil,
			},
		},
		"NoExternalName": {
			reason: "Delete without external name should return error",
			args: args{
				client: newTestClient(&MockDirectoryAPI{}),
				cr:     newDir("dir-unittests"),
			},
			want: want{
				err: stderrors.New("can not request API without GUID"),
			},
		},
		"Failure": {
			reason: "We expect to return an error if Delete fails",
			args: args{
				client: newTestClient(&MockDirectoryAPI{
					deleteErr: errors.New("DeleteError"),
				}),
				cr: newDir("dir-unittests", withExternalName(validGUID)),
			},
			want: want{
				err: errors.New("DeleteError"),
			},
		},
		"NotFoundIsNotError": {
			reason: "404 response should NOT be treated as error",
			args: args{
				client: newTestClient(&MockDirectoryAPI{
					deleteHTTP: &http.Response{StatusCode: http.StatusNotFound},
					deleteErr:  nil,
				}),
				cr: newDir("dir-unittests", withExternalName(validGUID)),
			},
			want: want{
				err: nil,
			},
		},
		"Success": {
			reason: "Successful deletion returns no error",
			args: args{
				client: newTestClient(&MockDirectoryAPI{
					deleteResponse: &accountclient.DirectoryResponseObject{},
				}),
				cr: newDir("dir-unittests", withExternalName(validGUID)),
			},
			want: want{
				err: nil,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Delete(tc.args.client, context.Background(), tc.args.cr)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nDelete(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
		})
	}
}
