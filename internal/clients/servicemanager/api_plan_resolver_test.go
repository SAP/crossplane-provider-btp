package servicemanager

import (
	"context"
	"net/http"
	"reflect"
	"testing"
	"unsafe"

	"github.com/pkg/errors"
	"github.com/sap/crossplane-provider-btp/internal"
	servicemanager "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-service-manager-api-go/pkg"
	"github.com/stretchr/testify/assert"
)

func TestNewServiceManagerClient(t *testing.T) {
	tests := []struct {
		name    string
		creds   *BindingCredentials
		success bool
	}{
		{
			name: "Invalid SM URL",
			creds: &BindingCredentials{
				Clientid:     internal.Ptr("someClientId"),
				Clientsecret: internal.Ptr("someClientSecret"),
				SmUrl:        internal.Ptr("::noUrl::"),
				Url:          internal.Ptr("https://valid.url"),
			},
			success: false,
		},
		{
			name:    "Success",
			creds:   &BindingCredentials{},
			success: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client, err := NewServiceManagerClient(context.TODO(), tc.creds)
			if tc.success != (err == nil) {
				t.Errorf("Unexpected error return; Expected error: %v, Returned: %v", !tc.success, err)
			}
			if tc.success != (client != nil) {
				t.Errorf("Unexpected client return; Returned: %v", client)
			}
		})
	}
}

func TestPlanIDByName(t *testing.T) {
	type args struct {
		listOfferingsMockFn func() (*servicemanager.ServiceOfferingResponseList, *http.Response, error)
		listPlansMockFn     func() (*servicemanager.ServicePlanResponseList, *http.Response, error)
		dataCenter          string
	}
	tests := []struct {
		name string
		args args

		wantErr bool
		wantID  string
	}{
		{
			name: "offeringError",
			args: args{
				listOfferingsMockFn: func() (*servicemanager.ServiceOfferingResponseList, *http.Response, error) {
					return nil, nil, errors.New("offeringApiError")
				},
			},
			wantErr: true,
		},
		{
			name: "offeringNotFound",
			args: args{
				listOfferingsMockFn: func() (*servicemanager.ServiceOfferingResponseList, *http.Response, error) {
					return &servicemanager.ServiceOfferingResponseList{
						Items: []servicemanager.ServiceOfferingResponseObject{},
					}, nil, nil
				},
			},
			wantErr: true,
		},
		{
			name: "plansError",
			args: args{
				listOfferingsMockFn: func() (*servicemanager.ServiceOfferingResponseList, *http.Response, error) {
					return &servicemanager.ServiceOfferingResponseList{
						Items: []servicemanager.ServiceOfferingResponseObject{
							{
								Name: internal.Ptr("someOffering"),
								Id:   internal.Ptr("someID"),
							},
						},
					}, nil, nil
				},
				listPlansMockFn: func() (*servicemanager.ServicePlanResponseList, *http.Response, error) {
					return nil, nil, errors.New("plansApiError")
				},
			},
			wantErr: true,
		},
		{
			name: "empty response",
			args: args{
				listOfferingsMockFn: func() (*servicemanager.ServiceOfferingResponseList, *http.Response, error) {
					return &servicemanager.ServiceOfferingResponseList{
						Items: []servicemanager.ServiceOfferingResponseObject{
							{
								Name: internal.Ptr("someOffering"),
								Id:   internal.Ptr("someID"),
							},
						},
					}, nil, nil
				},
				listPlansMockFn: func() (*servicemanager.ServicePlanResponseList, *http.Response, error) {
					return &servicemanager.ServicePlanResponseList{
						Items: []servicemanager.ServicePlanResponseObject{},
					}, nil, nil
				},
			},
			wantErr: true,
		},
		{
			name: "success",
			args: args{
				listOfferingsMockFn: func() (*servicemanager.ServiceOfferingResponseList, *http.Response, error) {
					return &servicemanager.ServiceOfferingResponseList{
						Items: []servicemanager.ServiceOfferingResponseObject{
							{
								Name: internal.Ptr("someOffering"),
								Id:   internal.Ptr("someID"),
							},
						},
					}, nil, nil
				},
				listPlansMockFn: func() (*servicemanager.ServicePlanResponseList, *http.Response, error) {
					return &servicemanager.ServicePlanResponseList{
						Items: []servicemanager.ServicePlanResponseObject{
							{
								Name: internal.Ptr("somePlan"),
								Id:   internal.Ptr("somePlanID"),
							},
						},
					}, nil, nil
				},
			},
			wantErr: false,
			wantID:  "somePlanID",
		},
		{
			name: "offeringNotFoundWithDataCenter",
			args: args{
				dataCenter: "cf-eu10",
				listOfferingsMockFn: func() (*servicemanager.ServiceOfferingResponseList, *http.Response, error) {
					return &servicemanager.ServiceOfferingResponseList{
						Items: []servicemanager.ServiceOfferingResponseObject{},
					}, nil, nil
				},
			},
			wantErr: true,
		},
		{
			name: "successWithDataCenter",
			args: args{
				dataCenter: "cf-eu10",
				listOfferingsMockFn: func() (*servicemanager.ServiceOfferingResponseList, *http.Response, error) {
					return &servicemanager.ServiceOfferingResponseList{
						Items: []servicemanager.ServiceOfferingResponseObject{
							{
								Name: internal.Ptr("hana-cloud"),
								Id:   internal.Ptr("hana-offering-id-eu10"),
							},
						},
					}, nil, nil
				},
				listPlansMockFn: func() (*servicemanager.ServicePlanResponseList, *http.Response, error) {
					return &servicemanager.ServicePlanResponseList{
						Items: []servicemanager.ServicePlanResponseObject{
							{
								Name: internal.Ptr("hana"),
								Id:   internal.Ptr("hana-plan-id-eu10"),
							},
						},
					}, nil, nil
				},
			},
			wantErr: false,
			wantID:  "hana-plan-id-eu10",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			smClient := &ServiceManagerClient{
				OfferingServiceFake{listOfferingsMockFn: tc.args.listOfferingsMockFn},
				PlansServiceFake{listPlansMockFn: tc.args.listPlansMockFn},
			}
			planID, err := smClient.PlanIDByName(context.TODO(), "Not relevant, since mocked", "Not relevant, since mocked", tc.args.dataCenter)

			if tc.wantErr != (err != nil) {
				t.Errorf("Unexpected error return; Expected error: %v, Returned: %v", tc.wantErr, err)
			}
			if tc.wantID != planID {
				t.Errorf("Unexpected returned PlanID; Expected: %s, Returned: %s", tc.wantID, planID)
			}

		})
	}
}

func TestPlanIDByName_OfferingQuery(t *testing.T) {
	// Regression test for #687: PlanIDByName used to always include
	// `data_center eq ''` in the field query when dataCenter was empty,
	// which made the SM API return no offerings and caused downstream
	// ServiceInstance reconciles to fail.
	tests := []struct {
		name       string
		dataCenter string
		wantQuery  string
	}{
		{
			name:       "without dataCenter omits data_center clause",
			dataCenter: "",
			wantQuery:  "catalog_name eq 'someOffering'",
		},
		{
			name:       "with dataCenter appends data_center clause",
			dataCenter: "cf-eu10",
			wantQuery:  "catalog_name eq 'someOffering' and data_center eq 'cf-eu10'",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var captured string
			smClient := &ServiceManagerClient{
				OfferingServiceFake{
					listOfferingsMockFn: func() (*servicemanager.ServiceOfferingResponseList, *http.Response, error) {
						return &servicemanager.ServiceOfferingResponseList{
							Items: []servicemanager.ServiceOfferingResponseObject{
								{Name: internal.Ptr("someOffering"), Id: internal.Ptr("someID")},
							},
						}, nil, nil
					},
					capturedFieldQuery: &captured,
				},
				PlansServiceFake{
					listPlansMockFn: func() (*servicemanager.ServicePlanResponseList, *http.Response, error) {
						return &servicemanager.ServicePlanResponseList{
							Items: []servicemanager.ServicePlanResponseObject{
								{Name: internal.Ptr("somePlan"), Id: internal.Ptr("somePlanID")},
							},
						}, nil, nil
					},
				},
			}
			_, err := smClient.PlanIDByName(context.TODO(), "someOffering", "somePlan", tc.dataCenter)
			assert.NoError(t, err)
			assert.Equal(t, tc.wantQuery, captured)
		})
	}
}

func TestPlanIDByName_MultipleOfferings(t *testing.T) {
	// Regression test: when the same catalog_name exists in multiple
	// data centers (e.g. cloud-logging, hana-cloud) and dataCenter is
	// not provided, the offering query returns multiple items. The
	// resolver must not pin the plan lookup to an arbitrary first
	// offering — if the plan exists only under a later offering, it
	// must still be found. Otherwise the resolver flakes with
	// "no service plan found" depending on SM API ordering.
	tests := []struct {
		name      string
		offerings []servicemanager.ServiceOfferingResponseObject
		plansByID map[string][]servicemanager.ServicePlanResponseObject
		wantErr   bool
		wantID    string
	}{
		{
			name: "plan only under second offering is still found",
			offerings: []servicemanager.ServiceOfferingResponseObject{
				{Name: internal.Ptr("cloud-logging"), Id: internal.Ptr("offering-eu10")},
				{Name: internal.Ptr("cloud-logging"), Id: internal.Ptr("offering-us10")},
			},
			plansByID: map[string][]servicemanager.ServicePlanResponseObject{
				"offering-eu10": {},
				"offering-us10": {{Name: internal.Ptr("standard"), Id: internal.Ptr("plan-us10")}},
			},
			wantID: "plan-us10",
		},
		{
			name: "plan exists in none of the offerings returns error mentioning data centers",
			offerings: []servicemanager.ServiceOfferingResponseObject{
				{Name: internal.Ptr("cloud-logging"), Id: internal.Ptr("offering-eu10")},
				{Name: internal.Ptr("cloud-logging"), Id: internal.Ptr("offering-us10")},
			},
			plansByID: map[string][]servicemanager.ServicePlanResponseObject{
				"offering-eu10": {},
				"offering-us10": {},
			},
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			smClient := &ServiceManagerClient{
				OfferingServiceFake{
					listOfferingsMockFn: func() (*servicemanager.ServiceOfferingResponseList, *http.Response, error) {
						return &servicemanager.ServiceOfferingResponseList{Items: tc.offerings}, nil, nil
					},
				},
				PlansServiceFake{
					listPlansByQueryMockFn: func(fieldQuery string) (*servicemanager.ServicePlanResponseList, *http.Response, error) {
						for id, plans := range tc.plansByID {
							if fieldQuery == "catalog_name eq 'standard' and service_offering_id eq '"+id+"'" {
								return &servicemanager.ServicePlanResponseList{Items: plans}, nil, nil
							}
						}
						return &servicemanager.ServicePlanResponseList{}, nil, nil
					},
				},
			}
			planID, err := smClient.PlanIDByName(context.TODO(), "cloud-logging", "standard", "")
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.wantID, planID)
		})
	}
}

func TestNewCredsFromOperatorSecret(t *testing.T) {
	tests := []struct {
		name   string
		secret map[string][]byte
		o      BindingCredentials
		err    error
	}{
		{
			name: "Missing Attribute Error Client ID",
			secret: map[string][]byte{
				"clientsecret": []byte("someSecret"),
				"sm_url":       []byte("https://valid.url"),
				"tokenurl":     []byte("https://valid.url"),
				"xsappname":    []byte("someXsAppName"),
			},
			err: errors.New(ErrMissingClientId),
		},
		{
			name: "Missing Attribute Error Client Secret",
			secret: map[string][]byte{
				"clientid":  []byte("someClientId"),
				"sm_url":    []byte("https://valid.url"),
				"tokenurl":  []byte("https://valid.url"),
				"xsappname": []byte("someXsAppName"),
			},
			err: errors.New(ErrMissingClientSecret),
		},
		{
			name: "Missing Attribute Error token URL",
			secret: map[string][]byte{
				"clientid":     []byte("someClientId"),
				"clientsecret": []byte("someSecret"),
				"sm_url":       []byte("https://valid.url"),
				"xsappname":    []byte("someXsAppName"),
			},
			err: errors.New(ErrMissingUrl),
		},
		{
			name: "Missing Attribute Error SmUrl",
			secret: map[string][]byte{
				"clientid":     []byte("someClientId"),
				"clientsecret": []byte("someSecret"),
				"tokenurl":     []byte("https://valid.url"),
				"xsappname":    []byte("someXsAppName"),
			},
			err: errors.New(ErrMissingSmUrl),
		},
		{
			name: "Missing Attribute Error Xsappname",
			secret: map[string][]byte{
				"clientid":     []byte("someClientId"),
				"clientsecret": []byte("someSecret"),
				"sm_url":       []byte("https://valid.url"),
				"tokenurl":     []byte("https://valid.url"),
			},
			err: errors.New(ErrMissingXsappname),
		},
		{
			name: "Successful Mapping",
			secret: map[string][]byte{
				"clientid":     []byte("someClientId"),
				"clientsecret": []byte("someSecret"),
				"sm_url":       []byte("https://valid.url"),
				"tokenurl":     []byte("https://valid.url"),
				"xsappname":    []byte("someXsAppName"),
			},
			o: BindingCredentials{
				Clientid:     internal.Ptr("someClientId"),
				Clientsecret: internal.Ptr("someSecret"),
				SmUrl:        internal.Ptr("https://valid.url"),
				Url:          internal.Ptr("https://valid.url"),
				Xsappname:    internal.Ptr("someXsAppName"),
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			o, err := NewCredsFromOperatorSecret(tc.secret)
			assert.Equal(t, tc.o, o)
			if tc.err != nil {
				assert.EqualError(t, err, tc.err.Error())
			}

		})
	}
}

var _ servicemanager.ServiceOfferingsAPI = &OfferingServiceFake{}

type OfferingServiceFake struct {
	listOfferingsMockFn func() (*servicemanager.ServiceOfferingResponseList, *http.Response, error)
	capturedFieldQuery  *string
}

func (f OfferingServiceFake) GetServiceOfferingById(ctx context.Context, serviceOfferingID string) servicemanager.ApiGetServiceOfferingByIdRequest {
	panic("implement me")
}

func (f OfferingServiceFake) GetServiceOfferingByIdExecute(r servicemanager.ApiGetServiceOfferingByIdRequest) (*servicemanager.ServiceOfferingResponseObject, *http.Response, error) {
	panic("implement me")
}

func (f OfferingServiceFake) GetServiceOfferings(ctx context.Context) servicemanager.ApiGetServiceOfferingsRequest {
	return servicemanager.ApiGetServiceOfferingsRequest{ApiService: f}
}

func (f OfferingServiceFake) GetServiceOfferingsExecute(r servicemanager.ApiGetServiceOfferingsRequest) (*servicemanager.ServiceOfferingResponseList, *http.Response, error) {
	if f.capturedFieldQuery != nil {
		// fieldQuery is an unexported *string on the request struct; read it via reflect+unsafe
		// so the test can assert on the exact query string generated by PlanIDByName.
		rv := reflect.ValueOf(&r).Elem().FieldByName("fieldQuery")
		rv = reflect.NewAt(rv.Type(), unsafe.Pointer(rv.UnsafeAddr())).Elem()
		if p := rv.Interface().(*string); p != nil {
			*f.capturedFieldQuery = *p
		}
	}
	return f.listOfferingsMockFn()
}

var _ servicemanager.ServicePlansAPI = &PlansServiceFake{}

type PlansServiceFake struct {
	listPlansMockFn        func() (*servicemanager.ServicePlanResponseList, *http.Response, error)
	listPlansByQueryMockFn func(fieldQuery string) (*servicemanager.ServicePlanResponseList, *http.Response, error)
}

func (p PlansServiceFake) GetServicePlansByServiceId(ctx context.Context, servicePlanID string) servicemanager.ApiGetServicePlansByServiceIdRequest {
	panic("implement me")
}

func (p PlansServiceFake) GetServicePlansByServiceIdExecute(r servicemanager.ApiGetServicePlansByServiceIdRequest) (*servicemanager.ServicePlanResponseObject, *http.Response, error) {
	panic("implement me")
}

func (p PlansServiceFake) GetAllServicePlans(ctx context.Context) servicemanager.ApiGetAllServicePlansRequest {
	return servicemanager.ApiGetAllServicePlansRequest{ApiService: p}
}

func (p PlansServiceFake) GetAllServicePlansExecute(r servicemanager.ApiGetAllServicePlansRequest) (*servicemanager.ServicePlanResponseList, *http.Response, error) {
	if p.listPlansByQueryMockFn != nil {
		rv := reflect.ValueOf(&r).Elem().FieldByName("fieldQuery")
		rv = reflect.NewAt(rv.Type(), unsafe.Pointer(rv.UnsafeAddr())).Elem()
		var fq string
		if ptr := rv.Interface().(*string); ptr != nil {
			fq = *ptr
		}
		return p.listPlansByQueryMockFn(fq)
	}
	return p.listPlansMockFn()
}
