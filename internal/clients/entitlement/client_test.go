package entitlement

import (
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	entClient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-entitlements-service-api-go/pkg"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal"
)

func TestGenerateObservation(t *testing.T) {
	type args struct {
		instance            *Instance
		relatedEntitlements *v1alpha1.EntitlementList
	}
	tests := []struct {
		name    string
		args    args
		want    *v1alpha1.EntitlementObservation
		wantErr bool
	}{
		{
			name: "happy path", args: args{
				instance: &Instance{
					EntitledServicePlan: &entClient.ServicePlanResponseObject{
						Amount:                    internal.Ptr(float32(1)),
						AutoAssign:                internal.Ptr(false),
						AutoDistributeAmount:      internal.Ptr(int32(0)),
						MaxAllowedSubaccountQuota: internal.Ptr(int32(999)),
						Unlimited:                 internal.Ptr(false),
					},
					Assignment: &entClient.AssignedServicePlanSubaccountDTO{
						Amount:                  internal.Ptr(float32(1)),
						AutoAssign:              internal.Ptr(true),
						AutoAssigned:            internal.Ptr(true),
						AutoDistributeAmount:    internal.Ptr(int32(123)),
						CreatedDate:             internal.Ptr(float64(0)),
						EntityId:                internal.Ptr("123"),
						EntityState:             internal.Ptr("State"),
						EntityType:              internal.Ptr("Type"),
						ParentAmount:            internal.Ptr(float32(0)),
						ParentId:                internal.Ptr(""),
						ParentRemainingAmount:   internal.Ptr(float32(0)),
						ParentType:              internal.Ptr(""),
						RequestedAmount:         internal.Ptr(float32(0)),
						Resources:               nil,
						StateMessage:            internal.Ptr("StateMsg"),
						UnlimitedAmountAssigned: internal.Ptr(true),
					},
				},
				relatedEntitlements: &v1alpha1.EntitlementList{
					Items: append(
						entitlementSlice(),
						amountEntitlement(1),
					),
				},
			}, want: &v1alpha1.EntitlementObservation{
				Required: &v1alpha1.EntitlementSummary{
					Enable:            nil,
					Amount:            internal.Ptr(1),
					EntitlementsCount: internal.Ptr(1),
				},
				Entitled: v1alpha1.Entitled{
					Amount:                    1,
					AutoAssign:                false,
					AutoDistributeAmount:      0,
					MaxAllowedSubaccountQuota: 999,
					Unlimited:                 false,
					Resources:                 []*v1alpha1.Resource{},
				},
				Assigned: &v1alpha1.Assignable{
					Amount:                  internal.Ptr(1),
					AutoAssign:              true,
					AutoAssigned:            true,
					AutoDistributeAmount:    123,
					EntityID:                "123",
					EntityState:             "State",
					EntityType:              "Type",
					RequestedAmount:         0,
					StateMessage:            "StateMsg",
					UnlimitedAmountAssigned: true,
					Resources:               []*v1alpha1.Resource{},
				},
			}, wantErr: false,
		},
		{
			name: "assigned unique identifier plumbed into Assignable", args: args{
				instance: &Instance{
					EntitledServicePlan: &entClient.ServicePlanResponseObject{
						UniqueIdentifier: internal.Ptr("entitled-uid"),
					},
					Assignment: &entClient.AssignedServicePlanSubaccountDTO{
						EntityId: internal.Ptr("123"),
					},
					AssignedUniqueIdentifier: internal.Ptr("assigned-uid"),
				},
				relatedEntitlements: &v1alpha1.EntitlementList{
					Items: entitlementSlice(),
				},
			}, want: &v1alpha1.EntitlementObservation{
				Required: &v1alpha1.EntitlementSummary{
					EntitlementsCount: internal.Ptr(0),
				},
				Entitled: v1alpha1.Entitled{
					UniqueIdentifier: "entitled-uid",
					Resources:        []*v1alpha1.Resource{},
				},
				Assigned: &v1alpha1.Assignable{
					EntityID:         "123",
					UniqueIdentifier: "assigned-uid",
					Resources:        []*v1alpha1.Resource{},
				},
			}, wantErr: false,
		},
		{
			name: "error merging", args: args{
				instance: nil,
				relatedEntitlements: &v1alpha1.EntitlementList{
					Items: append(
						entitlementSlice(),
						boolEntitlement(true),
						boolEntitlement(false),
					),
				},
			}, want: &v1alpha1.EntitlementObservation{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				got, err := GenerateObservation(
					tt.args.instance,
					tt.args.relatedEntitlements,
				)
				if (err != nil) != tt.wantErr {
					t.Errorf(
						"GenerateObservation() error = %v, wantErr %v",
						err,
						tt.wantErr,
					)
					return
				}

				if diff := cmp.Diff(tt.want, got); diff != "" {
					t.Errorf(
						"\n%s\nGenerateObservation(...): -want, +got:\n%s\n",
						tt.name,
						diff,
					)
				}
			},
		)
	}
}

func TestMergeRelatedEntitlements(t *testing.T) {
	type args struct {
		entitlements []v1alpha1.Entitlement
	}
	tests := []struct {
		name    string
		args    args
		want    *v1alpha1.EntitlementSummary
		wantErr bool
	}{
		{
			name:    "No Entitlements",
			args:    args{entitlements: entitlementSlice()},
			want:    &v1alpha1.EntitlementSummary{EntitlementsCount: internal.Ptr(0)},
			wantErr: false,
		},
		{
			name: "Amount: One Entitlement", args: args{
				entitlements: append(
					entitlementSlice(),
					amountEntitlement(1),
				),
			}, want: &v1alpha1.EntitlementSummary{Amount: internal.Ptr(1), Enable: nil, EntitlementsCount: internal.Ptr(1)}, wantErr: false,
		},
		{
			name: "Amount: Two Entitlements", args: args{
				entitlements: append(
					entitlementSlice(),
					amountEntitlement(1),
					anyEntitlement(
						internal.Ptr(3),
						nil,
					),
				),
			}, want: &v1alpha1.EntitlementSummary{Amount: internal.Ptr(4), Enable: nil, EntitlementsCount: internal.Ptr(2)}, wantErr: false,
		},
		{
			name: "Amount: Many Entitlements", args: args{
				entitlements: append(
					entitlementSlice(),
					amountEntitlement(1),
					anyEntitlement(
						internal.Ptr(7),
						nil,
					),
					anyEntitlement(
						internal.Ptr(3),
						nil,
					),
				),
			}, want: &v1alpha1.EntitlementSummary{Amount: internal.Ptr(11), Enable: nil, EntitlementsCount: internal.Ptr(3)}, wantErr: false,
		},
		{
			name: "Enable: Many Entitlements true", args: args{
				entitlements: append(
					entitlementSlice(),
					anyEntitlement(
						nil,
						internal.Ptr(true),
					),
					anyEntitlement(
						nil,
						internal.Ptr(true),
					),
				),
			}, want: &v1alpha1.EntitlementSummary{Amount: nil, Enable: internal.Ptr(true), EntitlementsCount: internal.Ptr(2)}, wantErr: false,
		},
		{
			name: "Enable: Many Entitlements false", args: args{
				entitlements: append(
					entitlementSlice(),
					anyEntitlement(
						nil,
						internal.Ptr(false),
					),
					anyEntitlement(
						nil,
						internal.Ptr(false),
					),
				),
			}, want: &v1alpha1.EntitlementSummary{Amount: nil, Enable: internal.Ptr(false), EntitlementsCount: internal.Ptr(2)}, wantErr: false,
		},
		{
			name: "Enable: Conflicting Entitlements", args: args{
				entitlements: append(
					entitlementSlice(),
					anyEntitlement(
						nil,
						internal.Ptr(false),
					),
					anyEntitlement(
						nil,
						internal.Ptr(true),
					),
				),
			}, want: &v1alpha1.EntitlementSummary{}, wantErr: true,
		},
		{
			name: "Amount: Negative Entitlement", args: args{
				entitlements: append(
					entitlementSlice(),
					anyEntitlement(
						internal.Ptr(-42),
						nil,
					),
				),
			}, want: &v1alpha1.EntitlementSummary{}, wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				got, err := MergeRelatedEntitlements(
					&v1alpha1.EntitlementList{
						Items: tt.args.entitlements,
					},
				)
				if (err != nil) != tt.wantErr {
					t.Errorf(
						"MergeRelatedEntitlements() error = %v, wantErr %v",
						err,
						tt.wantErr,
					)
					return
				}
				if !reflect.DeepEqual(
					got,
					tt.want,
				) {
					t.Errorf(
						"MergeRelatedEntitlements() got = %v, want %v",
						got,
						tt.want,
					)
				}
			},
		)
	}
}

func boolEntitlement(bool bool) v1alpha1.Entitlement {
	return anyEntitlement(
		nil,
		internal.Ptr(bool),
	)
}

func amountEntitlement(int int) v1alpha1.Entitlement {
	return anyEntitlement(
		internal.Ptr(int),
		nil,
	)
}

func anyEntitlement(amount *int, enable *bool) v1alpha1.Entitlement {
	return v1alpha1.Entitlement{
		Spec: v1alpha1.EntitlementSpec{
			ForProvider: v1alpha1.EntitlementParameters{
				Enable: enable,
				Amount: amount,
			},
		},
	}
}

func entitlementSlice() []v1alpha1.Entitlement {
	return make(
		[]v1alpha1.Entitlement,
		0,
	)
}

func Test_float64Pointer(t *testing.T) {
	type args struct {
		val *int
	}
	tests := []struct {
		name string
		args args
		want *float64
	}{
		{name: "nil", args: args{val: nil}, want: nil},
		{name: "with value", args: args{val: internal.Ptr(42)}, want: internal.Ptr(float64(42))},
	}
	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				if got := float64Pointer(tt.args.val); !reflect.DeepEqual(
					got,
					tt.want,
				) {
					t.Errorf(
						"float64Pointer() = %v, want %v",
						got,
						tt.want,
					)
				}
			},
		)
	}
}

func Test_isCompleteDeletion(t *testing.T) {
	type args struct {
		cr *v1alpha1.Entitlement
	}
	tests := []struct {
		name string
		want bool
		args args
	}{
		{
			name: "AmountNilButStillEnabled", want: false, args: args{
				cr: &v1alpha1.Entitlement{
					Status: v1alpha1.EntitlementStatus{
						AtProvider: &v1alpha1.EntitlementObservation{
							Required: &v1alpha1.EntitlementSummary{
								Enable: internal.Ptr(true),
								Amount: nil,
							},
						},
					},
				},
			},
		},
		{
			name: "AmountGt1AndEnabled", want: false, args: args{
				cr: &v1alpha1.Entitlement{
					Status: v1alpha1.EntitlementStatus{
						AtProvider: &v1alpha1.EntitlementObservation{
							Required: &v1alpha1.EntitlementSummary{
								Enable: internal.Ptr(true),
								Amount: internal.Ptr(42),
							},
						},
					},
				},
			},
		},
		{
			name: "AmountGt1AndEnabledNil", want: false, args: args{
				cr: &v1alpha1.Entitlement{
					Status: v1alpha1.EntitlementStatus{
						AtProvider: &v1alpha1.EntitlementObservation{
							Required: &v1alpha1.EntitlementSummary{
								Enable: nil,
								Amount: internal.Ptr(42),
							},
						},
					},
				},
			},
		},
		{
			name: "AmountNilAndEnabledNil", want: true, args: args{
				cr: &v1alpha1.Entitlement{
					Status: v1alpha1.EntitlementStatus{
						AtProvider: &v1alpha1.EntitlementObservation{
							Required: &v1alpha1.EntitlementSummary{
								Enable: nil,
								Amount: nil,
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				if got := isCompleteDeletion(tt.args.cr); got != tt.want {
					t.Errorf(
						"isCompleteDeletion() = %v, want %v",
						got,
						tt.want,
					)
				}
			},
		)
	}
}
