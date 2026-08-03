package entitlement

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sap/crossplane-provider-btp/cmd/exporter/btpcli"
)

func TestServiceToEntitlement_ExcludesAutoAssignedUnlessExplicitlyIncluded(t *testing.T) {
	t.Parallel()

	assignments := []btpcli.AssignedService{
		{
			Name: "service",
			ServicePlans: []btpcli.AssignedServicePlan{
				{
					Name: "plan",
					AssignmentInfo: []btpcli.AssignmentInfo{
						{EntityID: "subaccount", ModifiedDate: 1},
						{EntityID: "subaccount", ModifiedDate: 2, AutoAssigned: true},
					},
				},
			},
		},
	}

	withoutAutoAssigned := serviceToEntitlement(assignments, false)
	withAutoAssigned := serviceToEntitlement(assignments, true)

	require.Len(t, withoutAutoAssigned, 1)
	require.False(t, withoutAutoAssigned[0].assignment.AutoAssigned)
	require.Len(t, withAutoAssigned, 2)
}
