package serviceinstance

import (
	"context"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	smClient "github.com/sap/crossplane-provider-btp/internal/clients/servicemanager"
)

// siWithParams builds a ServiceInstance CR with the given desired parameters
// JSON and an external-name (BTP guid) so parametersDrifted proceeds past its
// guards.
func siWithParams(paramsJSON string) *v1alpha1.ServiceInstance {
	cr := &v1alpha1.ServiceInstance{}
	cr.SetName("cls-test")
	meta.SetExternalName(cr, "11111111-1111-1111-1111-111111111111")
	if paramsJSON != "" {
		cr.Spec.ForProvider.Parameters.Raw = []byte(paramsJSON)
	}
	return cr
}

func TestParametersDrifted(t *testing.T) {
	kube := &test.MockClient{} // not consulted: no ParameterSecretRefs in these cases

	cases := []struct {
		name         string
		cr           *v1alpha1.ServiceInstance
		lookuper     *lookuperFake
		noLookuperFn bool
		want         bool
		wantCalls    int
	}{
		{
			name:     "desired subset present on server -> no drift",
			cr:       siWithParams(`{"ingest_otlp":{"enabled":true}}`),
			lookuper: &lookuperFake{paramsFound: true, paramsResult: map[string]any{"ingest_otlp": map[string]any{"enabled": true}, "retention_period": float64(0)}},
			want:     false, wantCalls: 1,
		},
		{
			name:     "desired adds span_passthrough missing on server -> drift",
			cr:       siWithParams(`{"ingest_otlp":{"enabled":true,"span_passthrough":true}}`),
			lookuper: &lookuperFake{paramsFound: true, paramsResult: map[string]any{"ingest_otlp": map[string]any{"enabled": true}}},
			want:     true, wantCalls: 1,
		},
		{
			name:     "server returns no parameters (not retrievable) -> skip, no drift",
			cr:       siWithParams(`{"ingest_otlp":{"enabled":true}}`),
			lookuper: &lookuperFake{paramsFound: false},
			want:     false, wantCalls: 1,
		},
		{
			name:     "lookuper error -> fail-safe, no drift",
			cr:       siWithParams(`{"ingest_otlp":{"enabled":true}}`),
			lookuper: &lookuperFake{paramsErr: context.DeadlineExceeded},
			want:     false, wantCalls: 1,
		},
		{
			name:     "no desired parameters -> skip before any call",
			cr:       siWithParams(``),
			lookuper: &lookuperFake{paramsFound: true, paramsResult: map[string]any{"x": 1}},
			want:     false, wantCalls: 0,
		},
		{
			name: "no external-name yet -> skip before any call",
			cr: func() *v1alpha1.ServiceInstance {
				c := siWithParams(`{"a":1}`)
				meta.SetExternalName(c, "cls-test")
				return c
			}(),
			lookuper: &lookuperFake{paramsFound: true, paramsResult: map[string]any{"a": float64(1)}},
			want:     false, wantCalls: 0,
		},
		{
			name:         "no admin lookuper configured -> skip",
			cr:           siWithParams(`{"a":1}`),
			noLookuperFn: true,
			want:         false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := external{kube: kube}
			if !tc.noLookuperFn {
				e.newAdminLookuperFn = func(context.Context, *v1alpha1.ServiceInstance) (smClient.SemanticLookuper, func(), error) {
					return tc.lookuper, func() {}, nil
				}
			}
			got := e.parametersDrifted(context.Background(), tc.cr)
			if got != tc.want {
				t.Errorf("parametersDrifted = %v, want %v", got, tc.want)
			}
			if tc.lookuper != nil && tc.lookuper.paramsCalls != tc.wantCalls {
				t.Errorf("GetInstanceParameters calls = %d, want %d", tc.lookuper.paramsCalls, tc.wantCalls)
			}
		})
	}
}
