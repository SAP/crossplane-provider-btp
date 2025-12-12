package entitlement

import (
	"testing"

	"github.com/stretchr/testify/require"

	openapi "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-entitlements-service-api-go/pkg"
)

func Test_getEnableOk(t *testing.T) {
	r := require.New(t)

	tests := []struct {
		name                    string
		amount                  *float32
		parentAmount            *float32
		parentRemainingAmount   *float32
		unlimitedAmountAssigned *bool
		wantEnable              bool
		wantOk                  bool
	}{
		{
			name:                    "global unlimited quota",
			amount:                  float32Ptr(amountUnlimited),
			parentAmount:            float32Ptr(amountUnlimited),
			unlimitedAmountAssigned: boolPtr(true),
			wantEnable:              true,
			wantOk:                  true,
		},
		{
			name:                  "assigned, parent not getting less",
			amount:                float32Ptr(1),
			parentAmount:          float32Ptr(1),
			parentRemainingAmount: float32Ptr(1),
			wantEnable:            true,
			wantOk:                true,
		},
		{
			name:                  "assigned, parent getting less",
			amount:                float32Ptr(3),
			parentAmount:          float32Ptr(5),
			parentRemainingAmount: float32Ptr(2),
			wantEnable:            false,
			wantOk:                false,
		},
		{
			name:       "no relevant fields set",
			wantEnable: false,
			wantOk:     false,
		},
		{
			name:                    "unlimitedAmountAssigned false",
			amount:                  float32Ptr(amountUnlimited),
			parentAmount:            float32Ptr(amountUnlimited),
			unlimitedAmountAssigned: boolPtr(false),
			wantEnable:              false,
			wantOk:                  false,
		},
		{
			name:                    "unlimitedAmountAssigned true, but amount not unlimited",
			amount:                  float32Ptr(1),
			parentAmount:            float32Ptr(1),
			unlimitedAmountAssigned: boolPtr(true),
			wantEnable:              false,
			wantOk:                  false,
		},
		{
			name:                  "amount zero",
			amount:                float32Ptr(0),
			parentAmount:          float32Ptr(0),
			parentRemainingAmount: float32Ptr(0),
			wantEnable:            false,
			wantOk:                false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assignment := &openapi.AssignedServicePlanSubaccountDTO{
				Amount:                  tt.amount,
				ParentAmount:            tt.parentAmount,
				ParentRemainingAmount:   tt.parentRemainingAmount,
				UnlimitedAmountAssigned: tt.unlimitedAmountAssigned,
			}
			gotEnable, gotOk := getEnableOk(assignment)
			r.Equal(tt.wantEnable, gotEnable)
			r.Equal(tt.wantOk, gotOk)
		})
	}
}

func float32Ptr(v float32) *float32 { return &v }
func boolPtr(v bool) *bool          { return &v }
