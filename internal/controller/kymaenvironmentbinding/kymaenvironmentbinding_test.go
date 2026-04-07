package kymaenvironmentbinding

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	managed "github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal/clients/kymaenvironmentbinding"
	provisioningclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-provisioning-service-api-go/pkg"
	"github.com/sap/crossplane-provider-btp/internal/tracking"
)

var timeNow = time.Now()

func Test_external_validateBindings(t *testing.T) {
	type args struct {
		cr *v1alpha1.KymaEnvironmentBinding
	}
	tests := []struct {
		name            string
		args            args
		wantValid       bool
		wantValidCount  int
		wantActiveCount int
	}{
		{
			name: "needs rotation, secret expired before time.now()",
			args: args{
				cr: &v1alpha1.KymaEnvironmentBinding{
					Status: v1alpha1.KymaEnvironmentBindingStatus{
						AtProvider: v1alpha1.KymaEnvironmentBindingObservation{
							Bindings: []v1alpha1.Binding{
								{
									Id:        "id",
									IsActive:  true,
									CreatedAt: metav1.NewTime(timeNow.Add(time.Hour * -1)),
									ExpiresAt: metav1.NewTime(timeNow.Add(time.Minute * 10 * -1)),
								},
							},
						},
					},
				},
			},
			wantValid:       false,
			wantValidCount:  0,
			wantActiveCount: 0,
		},
		{
			name: "needs rotation, rotation interval reached",
			args: args{
				cr: &v1alpha1.KymaEnvironmentBinding{
					Spec: v1alpha1.KymaEnvironmentBindingSpec{
						ForProvider: v1alpha1.KymaEnvironmentBindingParameters{
							RotationInterval: metav1.Duration{Duration: time.Hour * 1},
						},
					},
					Status: v1alpha1.KymaEnvironmentBindingStatus{
						AtProvider: v1alpha1.KymaEnvironmentBindingObservation{
							Bindings: []v1alpha1.Binding{
								{
									Id:        "id",
									IsActive:  true,
									CreatedAt: metav1.NewTime(timeNow.Add(time.Hour * -1)),
									ExpiresAt: metav1.NewTime(timeNow.Add(time.Hour * +1)),
								},
							},
						},
					},
				},
			},
			wantValid:       false,
			wantValidCount:  1,
			wantActiveCount: 0,
		},
		{
			name: "no need to rotate, rotation interval not reached",
			args: args{
				cr: &v1alpha1.KymaEnvironmentBinding{
					Spec: v1alpha1.KymaEnvironmentBindingSpec{
						ForProvider: v1alpha1.KymaEnvironmentBindingParameters{
							RotationInterval: metav1.Duration{Duration: time.Hour * 2},
						},
					},
					Status: v1alpha1.KymaEnvironmentBindingStatus{
						AtProvider: v1alpha1.KymaEnvironmentBindingObservation{
							Bindings: []v1alpha1.Binding{
								{
									Id:        "id",
									IsActive:  true,
									CreatedAt: metav1.NewTime(timeNow.Add(time.Hour * -1)),
									ExpiresAt: metav1.NewTime(timeNow.Add(time.Hour * +2)),
								},
							},
						},
					},
				},
			},
			wantValid:       true,
			wantValidCount:  1,
			wantActiveCount: 1,
		},
		{
			name: "needs to rotate, secret expired, rotation interval not reached",
			args: args{
				cr: &v1alpha1.KymaEnvironmentBinding{
					Spec: v1alpha1.KymaEnvironmentBindingSpec{
						ForProvider: v1alpha1.KymaEnvironmentBindingParameters{
							RotationInterval: metav1.Duration{Duration: time.Hour * 2},
						},
					},
					Status: v1alpha1.KymaEnvironmentBindingStatus{
						AtProvider: v1alpha1.KymaEnvironmentBindingObservation{
							Bindings: []v1alpha1.Binding{
								{
									Id:        "id",
									IsActive:  true,
									CreatedAt: metav1.NewTime(timeNow.Add(time.Hour * -1)),
									ExpiresAt: metav1.NewTime(timeNow.Add(time.Minute * 10 * -1)),
								},
							},
						},
					},
				},
			},
			wantValid:       false,
			wantValidCount:  0,
			wantActiveCount: 0,
		},
		{
			name: "no need to rotate, no bindings",
			args: args{
				cr: &v1alpha1.KymaEnvironmentBinding{
					Status: v1alpha1.KymaEnvironmentBindingStatus{
						AtProvider: v1alpha1.KymaEnvironmentBindingObservation{
							Bindings: []v1alpha1.Binding{},
						},
					},
				},
			},
			wantValid:       false,
			wantValidCount:  0,
			wantActiveCount: 0,
		},
		{
			name: "no need to rotate, bindings is nil",
			args: args{
				cr: &v1alpha1.KymaEnvironmentBinding{
					Status: v1alpha1.KymaEnvironmentBindingStatus{
						AtProvider: v1alpha1.KymaEnvironmentBindingObservation{
							Bindings: nil,
						},
					},
				},
			},
			wantValid:       false,
			wantValidCount:  0,
			wantActiveCount: 0,
		},
		{
			name: "no need to rotate, no active bindings",
			args: args{
				cr: &v1alpha1.KymaEnvironmentBinding{
					Status: v1alpha1.KymaEnvironmentBindingStatus{
						AtProvider: v1alpha1.KymaEnvironmentBindingObservation{
							Bindings: []v1alpha1.Binding{
								{
									Id:        "id",
									IsActive:  false,
									CreatedAt: metav1.NewTime(timeNow.Add(time.Hour * -1)),
									ExpiresAt: metav1.NewTime(timeNow.Add(time.Hour * +1)),
								},
							},
						},
					},
				},
			},
			wantValid:       false,
			wantValidCount:  1,
			wantActiveCount: 0,
		},
		{
			name: "needs to rotate, multiple bindings with one active and expired",
			args: args{
				cr: &v1alpha1.KymaEnvironmentBinding{
					Status: v1alpha1.KymaEnvironmentBindingStatus{
						AtProvider: v1alpha1.KymaEnvironmentBindingObservation{
							Bindings: []v1alpha1.Binding{
								{
									Id:        "id1",
									IsActive:  false,
									CreatedAt: metav1.NewTime(timeNow.Add(time.Hour * -1)),
									ExpiresAt: metav1.NewTime(timeNow.Add(time.Hour * +1)),
								},
								{
									Id:        "id2",
									IsActive:  true,
									CreatedAt: metav1.NewTime(timeNow.Add(time.Hour * -1)),
									ExpiresAt: metav1.NewTime(timeNow.Add(time.Minute * 10 * -1)),
								},
							},
						},
					},
				},
			},
			wantValid:       false,
			wantValidCount:  1,
			wantActiveCount: 0,
		},
		{
			name: "needs to rotate, exactly at expiration time",
			args: args{
				cr: &v1alpha1.KymaEnvironmentBinding{
					Status: v1alpha1.KymaEnvironmentBindingStatus{
						AtProvider: v1alpha1.KymaEnvironmentBindingObservation{
							Bindings: []v1alpha1.Binding{
								{
									Id:        "id",
									IsActive:  true,
									CreatedAt: metav1.NewTime(timeNow.Add(time.Hour * -1)),
									ExpiresAt: metav1.NewTime(timeNow),
								},
							},
						},
					},
				},
			},
			wantValid:       false,
			wantValidCount:  0,
			wantActiveCount: 0,
		},
		{
			name: "needs to rotate, exactly at rotation interval",
			args: args{
				cr: &v1alpha1.KymaEnvironmentBinding{
					Spec: v1alpha1.KymaEnvironmentBindingSpec{
						ForProvider: v1alpha1.KymaEnvironmentBindingParameters{
							RotationInterval: metav1.Duration{Duration: time.Hour * 1},
						},
					},
					Status: v1alpha1.KymaEnvironmentBindingStatus{
						AtProvider: v1alpha1.KymaEnvironmentBindingObservation{
							Bindings: []v1alpha1.Binding{
								{
									Id:        "id",
									IsActive:  true,
									CreatedAt: metav1.NewTime(timeNow.Add(time.Hour * -1)),
									ExpiresAt: metav1.NewTime(timeNow.Add(time.Hour * 2)),
								},
							},
						},
					},
				},
			},
			wantValid:       false,
			wantValidCount:  1,
			wantActiveCount: 0,
		},
		{
			name: "keep inactive but non-expired bindings",
			args: args{
				cr: &v1alpha1.KymaEnvironmentBinding{
					Spec: v1alpha1.KymaEnvironmentBindingSpec{
						ForProvider: v1alpha1.KymaEnvironmentBindingParameters{
							RotationInterval: metav1.Duration{Duration: time.Hour * 1},
						},
					},
					Status: v1alpha1.KymaEnvironmentBindingStatus{
						AtProvider: v1alpha1.KymaEnvironmentBindingObservation{
							Bindings: []v1alpha1.Binding{
								{
									Id:        "id1",
									IsActive:  true,
									CreatedAt: metav1.NewTime(timeNow.Add(time.Hour * -1)),
									ExpiresAt: metav1.NewTime(timeNow.Add(time.Hour * 2)),
								},
								{
									Id:        "id2",
									IsActive:  false,
									CreatedAt: metav1.NewTime(timeNow.Add(time.Hour * -2)),
									ExpiresAt: metav1.NewTime(timeNow.Add(time.Hour * 1)),
								},
							},
						},
					},
				},
			},
			wantValid:       false,
			wantValidCount:  2,
			wantActiveCount: 0,
		},
		{
			name: "remove expired inactive bindings",
			args: args{
				cr: &v1alpha1.KymaEnvironmentBinding{
					Spec: v1alpha1.KymaEnvironmentBindingSpec{
						ForProvider: v1alpha1.KymaEnvironmentBindingParameters{
							RotationInterval: metav1.Duration{Duration: time.Hour * 1},
						},
					},
					Status: v1alpha1.KymaEnvironmentBindingStatus{
						AtProvider: v1alpha1.KymaEnvironmentBindingObservation{
							Bindings: []v1alpha1.Binding{
								{
									Id:        "id1",
									IsActive:  true,
									CreatedAt: metav1.NewTime(timeNow.Add(time.Hour * -1)),
									ExpiresAt: metav1.NewTime(timeNow.Add(time.Hour * 2)),
								},
								{
									Id:        "id2",
									IsActive:  false,
									CreatedAt: metav1.NewTime(timeNow.Add(time.Hour * -2)),
									ExpiresAt: metav1.NewTime(timeNow.Add(time.Minute * 10 * -1)),
								},
							},
						},
					},
				},
			},
			wantValid:       false,
			wantValidCount:  1,
			wantActiveCount: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &external{kube: test.NewMockClient()}
			gotValid, gotValidBindings := c.validateBindings(tt.args.cr)

			// Count active bindings
			activeCount := 0
			for _, b := range gotValidBindings {
				if b.IsActive {
					activeCount++
				}
			}

			if gotValid != tt.wantValid {
				t.Errorf("validateBindings() valid = %v, want %v", gotValid, tt.wantValid)
			}
			if len(gotValidBindings) != tt.wantValidCount {
				t.Errorf("validateBindings() valid count = %v, want %v", len(gotValidBindings), tt.wantValidCount)
			}
			if activeCount != tt.wantActiveCount {
				t.Errorf("validateBindings() active count = %v, want %v", activeCount, tt.wantActiveCount)
			}
		})
	}
}

func Test_external_Observe(t *testing.T) {
	type args struct {
		ctx context.Context
		mg  resource.Managed
	}
	tests := []struct {
		name           string
		args           args
		client         *fakeClient
		want           managed.ExternalObservation
		wantErr        bool
		expectedStatus v1alpha1.KymaEnvironmentBindingObservation
	}{
		{
			name: "not a KymaEnvironmentBinding",
			args: args{
				ctx: context.Background(),
				mg:  &v1alpha1.KymaEnvironmentBinding{},
			},
			want:           managed.ExternalObservation{},
			wantErr:        true,
			expectedStatus: v1alpha1.KymaEnvironmentBindingObservation{},
		},
		{
			name: "no connection secret reference",
			args: args{
				ctx: context.Background(),
				mg: &v1alpha1.KymaEnvironmentBinding{
					Spec: v1alpha1.KymaEnvironmentBindingSpec{
						ResourceSpec: xpv1.ResourceSpec{},
					},
				},
			},
			want:    managed.ExternalObservation{},
			wantErr: true,
		},
		{
			name: "needs rotation, no valid bindings",
			args: args{
				ctx: context.Background(),
				mg: &v1alpha1.KymaEnvironmentBinding{
					Spec: v1alpha1.KymaEnvironmentBindingSpec{
						ResourceSpec: xpv1.ResourceSpec{
							WriteConnectionSecretToReference: &xpv1.SecretReference{},
						},
					},
					Status: v1alpha1.KymaEnvironmentBindingStatus{
						AtProvider: v1alpha1.KymaEnvironmentBindingObservation{
							Bindings: []v1alpha1.Binding{
								{
									Id:        "id",
									IsActive:  true,
									CreatedAt: metav1.NewTime(timeNow.Add(time.Hour * -1)),
									ExpiresAt: metav1.NewTime(timeNow.Add(time.Minute * 10 * -1)),
								},
							},
						},
					},
				},
			},
			client: &fakeClient{
				describeInstanceFunc: func(ctx context.Context, kymaInstanceId string) ([]provisioningclient.EnvironmentInstanceBindingMetadata, error) {
					return []provisioningclient.EnvironmentInstanceBindingMetadata{
						{BindingId: &[]string{"id"}[0]},
					}, nil
				},
			},
			want: managed.ExternalObservation{
				ResourceExists:   false,
				ResourceUpToDate: true,
			},
			wantErr: false,
			expectedStatus: v1alpha1.KymaEnvironmentBindingObservation{
				Bindings: []v1alpha1.Binding{},
			},
		},
		{
			name: "valid binding exists",
			args: args{
				ctx: context.Background(),
				mg: &v1alpha1.KymaEnvironmentBinding{
					Spec: v1alpha1.KymaEnvironmentBindingSpec{
						ResourceSpec: xpv1.ResourceSpec{
							WriteConnectionSecretToReference: &xpv1.SecretReference{},
						},
						ForProvider: v1alpha1.KymaEnvironmentBindingParameters{
							RotationInterval: metav1.Duration{Duration: time.Hour * 2},
						},
					},
					Status: v1alpha1.KymaEnvironmentBindingStatus{
						AtProvider: v1alpha1.KymaEnvironmentBindingObservation{
							Bindings: []v1alpha1.Binding{
								{
									Id:        "id",
									IsActive:  true,
									CreatedAt: metav1.NewTime(timeNow.Add(time.Hour * -1)),
									ExpiresAt: metav1.NewTime(timeNow.Add(time.Hour * 2)),
								},
							},
						},
					},
				},
			},
			client: &fakeClient{
				describeInstanceFunc: func(ctx context.Context, kymaInstanceId string) ([]provisioningclient.EnvironmentInstanceBindingMetadata, error) {
					return []provisioningclient.EnvironmentInstanceBindingMetadata{
						{BindingId: &[]string{"id"}[0]},
					}, nil
				},
			},
			want: managed.ExternalObservation{
				ResourceExists:   true,
				ResourceUpToDate: true,
			},
			wantErr: false,
			expectedStatus: v1alpha1.KymaEnvironmentBindingObservation{
				Bindings: []v1alpha1.Binding{
					{
						Id:        "id",
						IsActive:  true,
						CreatedAt: metav1.NewTime(timeNow.Add(time.Hour * -1)),
						ExpiresAt: metav1.NewTime(timeNow.Add(time.Hour * 2)),
					},
				},
			},
		},
		{
			name: "needs rotation, rotation interval reached",
			args: args{
				ctx: context.Background(),
				mg: &v1alpha1.KymaEnvironmentBinding{
					Spec: v1alpha1.KymaEnvironmentBindingSpec{
						ResourceSpec: xpv1.ResourceSpec{
							WriteConnectionSecretToReference: &xpv1.SecretReference{},
						},
						ForProvider: v1alpha1.KymaEnvironmentBindingParameters{
							RotationInterval: metav1.Duration{Duration: time.Hour * 1},
						},
					},
					Status: v1alpha1.KymaEnvironmentBindingStatus{
						AtProvider: v1alpha1.KymaEnvironmentBindingObservation{
							Bindings: []v1alpha1.Binding{
								{
									Id:        "id",
									IsActive:  true,
									CreatedAt: metav1.NewTime(timeNow.Add(time.Hour * -1)),
									ExpiresAt: metav1.NewTime(timeNow.Add(time.Hour * 2)),
								},
							},
						},
					},
				},
			},
			client: &fakeClient{
				describeInstanceFunc: func(ctx context.Context, kymaInstanceId string) ([]provisioningclient.EnvironmentInstanceBindingMetadata, error) {
					return []provisioningclient.EnvironmentInstanceBindingMetadata{
						{BindingId: &[]string{"id"}[0]},
					}, nil
				},
			},
			want: managed.ExternalObservation{
				ResourceExists:   false,
				ResourceUpToDate: true,
			},
			wantErr: false,
			expectedStatus: v1alpha1.KymaEnvironmentBindingObservation{
				Bindings: []v1alpha1.Binding{
					{
						Id:        "id",
						IsActive:  false,
						CreatedAt: metav1.NewTime(timeNow.Add(time.Hour * -1)),
						ExpiresAt: metav1.NewTime(timeNow.Add(time.Hour * 2)),
					},
				},
			},
		},
		{
			name: "inactive but non-expired bindings exist",
			args: args{
				ctx: context.Background(),
				mg: &v1alpha1.KymaEnvironmentBinding{
					Spec: v1alpha1.KymaEnvironmentBindingSpec{
						ResourceSpec: xpv1.ResourceSpec{
							WriteConnectionSecretToReference: &xpv1.SecretReference{},
						},
						ForProvider: v1alpha1.KymaEnvironmentBindingParameters{
							RotationInterval: metav1.Duration{Duration: time.Hour * 1},
						},
					},
					Status: v1alpha1.KymaEnvironmentBindingStatus{
						AtProvider: v1alpha1.KymaEnvironmentBindingObservation{
							Bindings: []v1alpha1.Binding{
								{
									Id:        "id",
									IsActive:  false,
									CreatedAt: metav1.NewTime(timeNow.Add(time.Hour * -1)),
									ExpiresAt: metav1.NewTime(timeNow.Add(time.Hour * 2)),
								},
							},
						},
					},
				},
			},
			client: &fakeClient{
				describeInstanceFunc: func(ctx context.Context, kymaInstanceId string) ([]provisioningclient.EnvironmentInstanceBindingMetadata, error) {
					return []provisioningclient.EnvironmentInstanceBindingMetadata{
						{BindingId: &[]string{"id"}[0]},
					}, nil
				},
			},
			want: managed.ExternalObservation{
				ResourceExists:   false,
				ResourceUpToDate: true,
			},
			wantErr: false,
			expectedStatus: v1alpha1.KymaEnvironmentBindingObservation{
				Bindings: []v1alpha1.Binding{

					{
						Id:        "id",
						IsActive:  false,
						CreatedAt: metav1.NewTime(timeNow.Add(time.Hour * -1)),
						ExpiresAt: metav1.NewTime(timeNow.Add(time.Hour * 2)),
					},
				},
			},
		},
		{
			name: "multiple bindings with one active and valid",
			args: args{
				ctx: context.Background(),
				mg: &v1alpha1.KymaEnvironmentBinding{
					Spec: v1alpha1.KymaEnvironmentBindingSpec{
						ResourceSpec: xpv1.ResourceSpec{
							WriteConnectionSecretToReference: &xpv1.SecretReference{},
						},
						ForProvider: v1alpha1.KymaEnvironmentBindingParameters{
							RotationInterval: metav1.Duration{Duration: time.Hour * 2},
						},
					},
					Status: v1alpha1.KymaEnvironmentBindingStatus{
						AtProvider: v1alpha1.KymaEnvironmentBindingObservation{
							Bindings: []v1alpha1.Binding{
								{
									Id:        "id1",
									IsActive:  false,
									CreatedAt: metav1.NewTime(timeNow.Add(time.Hour * -2)),
									ExpiresAt: metav1.NewTime(timeNow.Add(time.Hour * 1)),
								},
								{
									Id:        "id2",
									IsActive:  true,
									CreatedAt: metav1.NewTime(timeNow.Add(time.Hour * -1)),
									ExpiresAt: metav1.NewTime(timeNow.Add(time.Hour * 2)),
								},
							},
						},
					},
				},
			},
			client: &fakeClient{
				describeInstanceFunc: func(ctx context.Context, kymaInstanceId string) ([]provisioningclient.EnvironmentInstanceBindingMetadata, error) {
					return []provisioningclient.EnvironmentInstanceBindingMetadata{
						{BindingId: &[]string{"id1"}[0]}, {BindingId: &[]string{"id2"}[0]},
					}, nil
				},
			},
			want: managed.ExternalObservation{
				ResourceExists:   true,
				ResourceUpToDate: true,
			},
			wantErr: false,
			expectedStatus: v1alpha1.KymaEnvironmentBindingObservation{
				Bindings: []v1alpha1.Binding{
					{
						Id:        "id1",
						IsActive:  false,
						CreatedAt: metav1.NewTime(timeNow.Add(time.Hour * -2)),
						ExpiresAt: metav1.NewTime(timeNow.Add(time.Hour * 1)),
					},
					{
						Id:        "id2",
						IsActive:  true,
						CreatedAt: metav1.NewTime(timeNow.Add(time.Hour * -1)),
						ExpiresAt: metav1.NewTime(timeNow.Add(time.Hour * 2)),
					},
				},
			},
		},
		{
			name: "service response has extra bindings not in status",
			args: args{
				ctx: context.Background(),
				mg: &v1alpha1.KymaEnvironmentBinding{
					Spec: v1alpha1.KymaEnvironmentBindingSpec{
						ForProvider: v1alpha1.KymaEnvironmentBindingParameters{
							RotationInterval: metav1.Duration{Duration: time.Hour * 2},
						},
						ResourceSpec: xpv1.ResourceSpec{
							WriteConnectionSecretToReference: &xpv1.SecretReference{},
						},
					},
					Status: v1alpha1.KymaEnvironmentBindingStatus{
						AtProvider: v1alpha1.KymaEnvironmentBindingObservation{
							Bindings: []v1alpha1.Binding{
								{
									Id:        "id1",
									IsActive:  true,
									CreatedAt: metav1.NewTime(timeNow.Add(time.Hour * -1)),
									ExpiresAt: metav1.NewTime(timeNow.Add(time.Hour * 2)),
								},
							},
						},
					},
				},
			},
			client: &fakeClient{
				describeInstanceFunc: func(ctx context.Context, kymaInstanceId string) ([]provisioningclient.EnvironmentInstanceBindingMetadata, error) {
					return []provisioningclient.EnvironmentInstanceBindingMetadata{
						{BindingId: &[]string{"id1"}[0]},
						{BindingId: &[]string{"id2"}[0]}, // Extra binding
					}, nil
				},
			},
			want: managed.ExternalObservation{
				ResourceExists:   true,
				ResourceUpToDate: true,
			},
			wantErr: false,
			expectedStatus: v1alpha1.KymaEnvironmentBindingObservation{
				Bindings: []v1alpha1.Binding{
					{
						Id:        "id1",
						IsActive:  true,
						CreatedAt: metav1.NewTime(timeNow.Add(time.Hour * -1)),
						ExpiresAt: metav1.NewTime(timeNow.Add(time.Hour * 2)),
					},
				},
			},
		},
		{
			name: "service response is missing bindings present in status",
			args: args{
				ctx: context.Background(),
				mg: &v1alpha1.KymaEnvironmentBinding{
					Spec: v1alpha1.KymaEnvironmentBindingSpec{
						ForProvider: v1alpha1.KymaEnvironmentBindingParameters{
							RotationInterval: metav1.Duration{Duration: time.Hour * 2},
						},
						ResourceSpec: xpv1.ResourceSpec{
							WriteConnectionSecretToReference: &xpv1.SecretReference{},
						},
					},
					Status: v1alpha1.KymaEnvironmentBindingStatus{
						AtProvider: v1alpha1.KymaEnvironmentBindingObservation{
							Bindings: []v1alpha1.Binding{
								{
									Id:        "id1",
									IsActive:  true,
									CreatedAt: metav1.NewTime(timeNow.Add(time.Hour * -1)),
									ExpiresAt: metav1.NewTime(timeNow.Add(time.Hour * 2)),
								},
								{
									Id:        "id2",
									IsActive:  false,
									CreatedAt: metav1.NewTime(timeNow.Add(time.Hour * -2)),
									ExpiresAt: metav1.NewTime(timeNow.Add(time.Hour * 1)),
								},
							},
						},
					},
				},
			},
			client: &fakeClient{
				describeInstanceFunc: func(ctx context.Context, kymaInstanceId string) ([]provisioningclient.EnvironmentInstanceBindingMetadata, error) {
					return []provisioningclient.EnvironmentInstanceBindingMetadata{
						{BindingId: &[]string{"id1"}[0]}, // Missing "id2"
					}, nil
				},
			},
			want: managed.ExternalObservation{
				ResourceExists:   true,
				ResourceUpToDate: true,
			},
			wantErr: false,
			expectedStatus: v1alpha1.KymaEnvironmentBindingObservation{
				Bindings: []v1alpha1.Binding{
					{
						Id:        "id1",
						IsActive:  true,
						CreatedAt: metav1.NewTime(timeNow.Add(time.Hour * -1)),
						ExpiresAt: metav1.NewTime(timeNow.Add(time.Hour * 2)),
					},
				},
			},
		},
		{
			name: "service response has no bindings while status has bindings",
			args: args{
				ctx: context.Background(),
				mg: &v1alpha1.KymaEnvironmentBinding{
					Spec: v1alpha1.KymaEnvironmentBindingSpec{
						ResourceSpec: xpv1.ResourceSpec{
							WriteConnectionSecretToReference: &xpv1.SecretReference{},
						},
					},
					Status: v1alpha1.KymaEnvironmentBindingStatus{
						AtProvider: v1alpha1.KymaEnvironmentBindingObservation{
							Bindings: []v1alpha1.Binding{
								{
									Id:        "id1",
									IsActive:  true,
									CreatedAt: metav1.NewTime(timeNow.Add(time.Hour * -1)),
									ExpiresAt: metav1.NewTime(timeNow.Add(time.Hour * 2)),
								},
							},
						},
					},
				},
			},
			client: &fakeClient{
				describeInstanceFunc: func(ctx context.Context, kymaInstanceId string) ([]provisioningclient.EnvironmentInstanceBindingMetadata, error) {
					return []provisioningclient.EnvironmentInstanceBindingMetadata{}, nil // No bindings
				},
			},
			want: managed.ExternalObservation{
				ResourceExists:   false,
				ResourceUpToDate: true,
			},
			wantErr: false,
			expectedStatus: v1alpha1.KymaEnvironmentBindingObservation{
				Bindings: []v1alpha1.Binding{},
			},
		},
		{
			name: "service response has bindings while status has none",
			args: args{
				ctx: context.Background(),
				mg: &v1alpha1.KymaEnvironmentBinding{
					Spec: v1alpha1.KymaEnvironmentBindingSpec{
						ResourceSpec: xpv1.ResourceSpec{
							WriteConnectionSecretToReference: &xpv1.SecretReference{},
						},
					},
					Status: v1alpha1.KymaEnvironmentBindingStatus{
						AtProvider: v1alpha1.KymaEnvironmentBindingObservation{
							Bindings: []v1alpha1.Binding{}, // No bindings in status
						},
					},
				},
			},
			client: &fakeClient{
				describeInstanceFunc: func(ctx context.Context, kymaInstanceId string) ([]provisioningclient.EnvironmentInstanceBindingMetadata, error) {
					return []provisioningclient.EnvironmentInstanceBindingMetadata{
						{BindingId: &[]string{"id1"}[0]}, // Basically an unknown to us
					}, nil
				},
			},
			want: managed.ExternalObservation{
				ResourceExists:   false,
				ResourceUpToDate: true,
			},
			wantErr: false,
			expectedStatus: v1alpha1.KymaEnvironmentBindingObservation{
				Bindings: []v1alpha1.Binding{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &external{kube: test.NewMockClient(), client: tt.client}
			got, err := c.Observe(tt.args.ctx, tt.args.mg)
			if (err != nil) != tt.wantErr {
				t.Errorf("Observe() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if diff := cmp.Diff(tt.want, got,
				cmp.FilterPath(func(p cmp.Path) bool {
					return p.Last().String() == "[\"created_at\"]" || p.Last().String() == "[\"expires_at\"]"
				}, cmp.Ignore()),
				cmp.AllowUnexported(managed.ExternalObservation{})); diff != "" {
				t.Errorf("Observe() mismatch (-want +got):\n%s", diff)
			}
			// Assert status update
			cr := tt.args.mg.(*v1alpha1.KymaEnvironmentBinding)
			if diff := cmp.Diff(tt.expectedStatus, cr.Status.AtProvider); diff != "" {
				t.Errorf("Status mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func Test_external_Delete(t *testing.T) {
	tests := []struct {
		name    string
		mg      resource.Managed
		client  *fakeClient
		wantErr bool
	}{
		{
			name:    "not a KymaEnvironmentBinding",
			mg:      &v1alpha1.KymaEnvironment{},
			client:  &fakeClient{},
			wantErr: true,
		},
		{
			name: "successful deletion with multiple bindings",
			mg: &v1alpha1.KymaEnvironmentBinding{
				Spec: v1alpha1.KymaEnvironmentBindingSpec{
					KymaEnvironmentId: "test-instance",
				},
				Status: v1alpha1.KymaEnvironmentBindingStatus{
					AtProvider: v1alpha1.KymaEnvironmentBindingObservation{
						Bindings: []v1alpha1.Binding{
							{
								Id:        "id1",
								IsActive:  true,
								CreatedAt: metav1.NewTime(timeNow.Add(time.Hour * -1)),
								ExpiresAt: metav1.NewTime(timeNow.Add(time.Hour * 2)),
							},
							{
								Id:        "id2",
								IsActive:  false,
								CreatedAt: metav1.NewTime(timeNow.Add(time.Hour * -2)),
								ExpiresAt: metav1.NewTime(timeNow.Add(time.Hour * 1)),
							},
						},
					},
				},
			},
			client: &fakeClient{
				deleteInstanceFunc: func(ctx context.Context, bindings []v1alpha1.Binding, kymaInstanceId string) error {
					return nil
				},
			},
			wantErr: false,
		},
		{
			name: "service returns error during deletion",
			mg: &v1alpha1.KymaEnvironmentBinding{
				Spec: v1alpha1.KymaEnvironmentBindingSpec{
					KymaEnvironmentId: "error-instance",
				},
				Status: v1alpha1.KymaEnvironmentBindingStatus{
					AtProvider: v1alpha1.KymaEnvironmentBindingObservation{
						Bindings: []v1alpha1.Binding{
							{
								Id:        "id1",
								IsActive:  true,
								CreatedAt: metav1.NewTime(timeNow.Add(time.Hour * -1)),
								ExpiresAt: metav1.NewTime(timeNow.Add(time.Hour * 2)),
							},
						},
					},
				},
			},
			client: &fakeClient{
				deleteInstanceFunc: func(ctx context.Context, bindings []v1alpha1.Binding, kymaInstanceId string) error {
					return errors.New("service error")
				},
			},
			wantErr: true,
		},
		{
			name: "service returns error for non-existent binding",
			mg: &v1alpha1.KymaEnvironmentBinding{
				Spec: v1alpha1.KymaEnvironmentBindingSpec{
					KymaEnvironmentId: "non-existent-instance",
				},
				Status: v1alpha1.KymaEnvironmentBindingStatus{
					AtProvider: v1alpha1.KymaEnvironmentBindingObservation{
						Bindings: []v1alpha1.Binding{
							{
								Id:        "non-existent-id",
								IsActive:  true,
								CreatedAt: metav1.NewTime(timeNow.Add(time.Hour * -1)),
								ExpiresAt: metav1.NewTime(timeNow.Add(time.Hour * 2)),
							},
						},
					},
				},
			},
			client: &fakeClient{
				deleteInstanceFunc: func(ctx context.Context, bindings []v1alpha1.Binding, kymaInstanceId string) error {
					return errors.New("binding not found")
				},
			},
			wantErr: true,
		},
		{
			name: "successful deletion with no bindings",
			mg: &v1alpha1.KymaEnvironmentBinding{
				Spec: v1alpha1.KymaEnvironmentBindingSpec{
					KymaEnvironmentId: "test-instance",
				},
				Status: v1alpha1.KymaEnvironmentBindingStatus{
					AtProvider: v1alpha1.KymaEnvironmentBindingObservation{
						Bindings: []v1alpha1.Binding{},
					},
				},
			},
			client: &fakeClient{
				deleteInstanceFunc: func(ctx context.Context, bindings []v1alpha1.Binding, kymaInstanceId string) error {
					return nil
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &external{kube: test.NewMockClient(), client: tt.client, tracker: tracking.NewDefaultReferenceResolverTracker(test.NewMockClient())}
			_, err := c.Delete(context.Background(), tt.mg)
			if (err != nil) != tt.wantErr {
				t.Errorf("Delete() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_external_Create(t *testing.T) {
	conflictErr := kerrors.NewConflict(schema.GroupResource{}, "test", errors.New("conflict"))

	defaultBinding := &kymaenvironmentbinding.Binding{
		Metadata:    &kymaenvironmentbinding.Metadata{Id: "new-id", ExpiresAt: timeNow.Add(time.Hour * 2)},
		Credentials: &kymaenvironmentbinding.Credentials{Kubeconfig: "kubeconfig-data"},
	}
	defaultCreateFn := func(ctx context.Context, _ string, _ int) (*kymaenvironmentbinding.Binding, error) {
		return defaultBinding, nil
	}

	type wantResult struct {
		err               bool
		errContains       string
		rollbackCalled    bool
		connectionDetails managed.ConnectionDetails
	}
	tests := []struct {
		name            string
		mg              resource.Managed
		client          *fakeClient
		statusUpdateFns []test.MockSubResourceUpdateFn
		getFn           test.MockGetFn
		rollbackErr     error
		want            wantResult
	}{
		{
			name:            "not a KymaEnvironmentBinding",
			mg:              &v1alpha1.KymaEnvironment{},
			client:          &fakeClient{},
			statusUpdateFns: []test.MockSubResourceUpdateFn{},
			want:            wantResult{err: true},
		},
		{
			name: "create new binding when no valid bindings exist",
			mg: &v1alpha1.KymaEnvironmentBinding{
				Spec: v1alpha1.KymaEnvironmentBindingSpec{
					KymaEnvironmentId: "test-instance",
					ForProvider: v1alpha1.KymaEnvironmentBindingParameters{
						RotationInterval: metav1.Duration{Duration: time.Hour * 1},
					},
				},
				Status: v1alpha1.KymaEnvironmentBindingStatus{
					AtProvider: v1alpha1.KymaEnvironmentBindingObservation{
						Bindings: []v1alpha1.Binding{
							{
								Id:        "id",
								IsActive:  true,
								CreatedAt: metav1.NewTime(timeNow.Add(time.Hour * -1)),
								ExpiresAt: metav1.NewTime(timeNow.Add(time.Minute * 10 * -1)),
							},
						},
					},
				},
			},
			client: &fakeClient{
				createInstanceFunc: func(ctx context.Context, kymaInstanceId string, ttl int) (*kymaenvironmentbinding.Binding, error) {
					return &kymaenvironmentbinding.Binding{
						Metadata:    &kymaenvironmentbinding.Metadata{Id: "new-binding-id", ExpiresAt: timeNow.Add(time.Hour * 2)},
						Credentials: &kymaenvironmentbinding.Credentials{Kubeconfig: "new-binding-secret"},
					}, nil
				},
			},
			statusUpdateFns: []test.MockSubResourceUpdateFn{test.NewMockSubResourceUpdateFn(nil)},
			want: wantResult{
				connectionDetails: managed.ConnectionDetails{
					"binding_id": []byte("new-binding-id"),
					"kubeconfig": []byte("new-binding-secret"),
				},
			},
		},
		{
			name: "reuse existing valid binding",
			mg: &v1alpha1.KymaEnvironmentBinding{
				Spec: v1alpha1.KymaEnvironmentBindingSpec{
					KymaEnvironmentId: "test-instance",
					ForProvider: v1alpha1.KymaEnvironmentBindingParameters{
						RotationInterval: metav1.Duration{Duration: time.Hour * 2},
					},
				},
				Status: v1alpha1.KymaEnvironmentBindingStatus{
					AtProvider: v1alpha1.KymaEnvironmentBindingObservation{
						Bindings: []v1alpha1.Binding{
							{
								Id:        "valid-id",
								IsActive:  true,
								CreatedAt: metav1.NewTime(timeNow.Add(time.Hour * -1)),
								ExpiresAt: metav1.NewTime(timeNow.Add(time.Hour * 2)),
							},
						},
					},
				},
			},
			client: &fakeClient{
				createInstanceFunc: func(ctx context.Context, kymaInstanceId string, ttl int) (*kymaenvironmentbinding.Binding, error) {
					return &kymaenvironmentbinding.Binding{
						Metadata:    &kymaenvironmentbinding.Metadata{Id: "valid-id", ExpiresAt: timeNow.Add(time.Hour * 2)},
						Credentials: &kymaenvironmentbinding.Credentials{Kubeconfig: "valid-id"},
					}, nil
				},
			},
			statusUpdateFns: []test.MockSubResourceUpdateFn{test.NewMockSubResourceUpdateFn(nil)},
			want: wantResult{
				connectionDetails: managed.ConnectionDetails{
					"binding_id": []byte("valid-id"),
					"kubeconfig": []byte("valid-id"),
				},
			},
		},
		{
			name: "service returns error during creation",
			mg: &v1alpha1.KymaEnvironmentBinding{
				Spec: v1alpha1.KymaEnvironmentBindingSpec{
					KymaEnvironmentId: "error-instance",
					ForProvider: v1alpha1.KymaEnvironmentBindingParameters{
						RotationInterval: metav1.Duration{Duration: time.Hour * 1},
					},
				},
				Status: v1alpha1.KymaEnvironmentBindingStatus{
					AtProvider: v1alpha1.KymaEnvironmentBindingObservation{Bindings: []v1alpha1.Binding{}},
				},
			},
			client: &fakeClient{
				createInstanceFunc: func(ctx context.Context, kymaInstanceId string, ttl int) (*kymaenvironmentbinding.Binding, error) {
					return nil, errors.New("service error")
				},
			},
			statusUpdateFns: []test.MockSubResourceUpdateFn{},
			want:            wantResult{err: true},
		},
		{
			name: "service returns error for invalid instance",
			mg: &v1alpha1.KymaEnvironmentBinding{
				Spec: v1alpha1.KymaEnvironmentBindingSpec{
					KymaEnvironmentId: "invalid-instance",
					ForProvider: v1alpha1.KymaEnvironmentBindingParameters{
						RotationInterval: metav1.Duration{Duration: time.Hour * 1},
					},
				},
				Status: v1alpha1.KymaEnvironmentBindingStatus{
					AtProvider: v1alpha1.KymaEnvironmentBindingObservation{Bindings: []v1alpha1.Binding{}},
				},
			},
			client: &fakeClient{
				createInstanceFunc: func(ctx context.Context, kymaInstanceId string, ttl int) (*kymaenvironmentbinding.Binding, error) {
					return nil, errors.New("invalid instance")
				},
			},
			statusUpdateFns: []test.MockSubResourceUpdateFn{},
			want:            wantResult{err: true},
		},
		{
			name: "status write succeeds after re-fetch",
			mg: &v1alpha1.KymaEnvironmentBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Spec:       v1alpha1.KymaEnvironmentBindingSpec{KymaEnvironmentId: "kyma-id"},
				Status: v1alpha1.KymaEnvironmentBindingStatus{
					AtProvider: v1alpha1.KymaEnvironmentBindingObservation{Bindings: []v1alpha1.Binding{}},
				},
			},
			client:          &fakeClient{createInstanceFunc: defaultCreateFn},
			statusUpdateFns: []test.MockSubResourceUpdateFn{test.NewMockSubResourceUpdateFn(nil)},
			getFn:           test.NewMockGetFn(nil),
			want: wantResult{
				connectionDetails: managed.ConnectionDetails{
					"binding_id": []byte("new-id"),
					"kubeconfig": []byte("kubeconfig-data"),
				},
			},
		},
		{
			name: "status conflict resolved on retry",
			mg: &v1alpha1.KymaEnvironmentBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Spec:       v1alpha1.KymaEnvironmentBindingSpec{KymaEnvironmentId: "kyma-id"},
				Status: v1alpha1.KymaEnvironmentBindingStatus{
					AtProvider: v1alpha1.KymaEnvironmentBindingObservation{Bindings: []v1alpha1.Binding{}},
				},
			},
			client: &fakeClient{createInstanceFunc: defaultCreateFn},
			statusUpdateFns: []test.MockSubResourceUpdateFn{
				test.NewMockSubResourceUpdateFn(conflictErr),
				test.NewMockSubResourceUpdateFn(nil),
			},
			getFn: test.NewMockGetFn(nil),
			want: wantResult{
				connectionDetails: managed.ConnectionDetails{
					"binding_id": []byte("new-id"),
					"kubeconfig": []byte("kubeconfig-data"),
				},
			},
		},
		{
			name: "all retries exhausted triggers rollback",
			mg: &v1alpha1.KymaEnvironmentBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Spec:       v1alpha1.KymaEnvironmentBindingSpec{KymaEnvironmentId: "kyma-id"},
				Status: v1alpha1.KymaEnvironmentBindingStatus{
					AtProvider: v1alpha1.KymaEnvironmentBindingObservation{Bindings: []v1alpha1.Binding{}},
				},
			},
			client: &fakeClient{createInstanceFunc: defaultCreateFn},
			statusUpdateFns: []test.MockSubResourceUpdateFn{
				test.NewMockSubResourceUpdateFn(conflictErr),
				test.NewMockSubResourceUpdateFn(conflictErr),
				test.NewMockSubResourceUpdateFn(conflictErr),
				test.NewMockSubResourceUpdateFn(conflictErr),
				test.NewMockSubResourceUpdateFn(conflictErr),
			},
			getFn: test.NewMockGetFn(nil),
			want:  wantResult{err: true, rollbackCalled: true},
		},
		{
			name: "all retries exhausted and rollback also fails",
			mg: &v1alpha1.KymaEnvironmentBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Spec:       v1alpha1.KymaEnvironmentBindingSpec{KymaEnvironmentId: "kyma-id"},
				Status: v1alpha1.KymaEnvironmentBindingStatus{
					AtProvider: v1alpha1.KymaEnvironmentBindingObservation{Bindings: []v1alpha1.Binding{}},
				},
			},
			client: &fakeClient{createInstanceFunc: defaultCreateFn},
			statusUpdateFns: []test.MockSubResourceUpdateFn{
				test.NewMockSubResourceUpdateFn(conflictErr),
				test.NewMockSubResourceUpdateFn(conflictErr),
				test.NewMockSubResourceUpdateFn(conflictErr),
				test.NewMockSubResourceUpdateFn(conflictErr),
				test.NewMockSubResourceUpdateFn(conflictErr),
			},
			getFn:       test.NewMockGetFn(nil),
			rollbackErr: errors.New("rollback failed"),
			want:        wantResult{err: true, rollbackCalled: true, errContains: errRollbackBinding},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rollbackCalled := false
			callIdx := 0

			kube := &test.MockClient{
				MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
					if tt.getFn != nil {
						return tt.getFn(ctx, key, obj)
					}
					return nil
				},
				MockStatusUpdate: func(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
					fn := tt.statusUpdateFns[callIdx]
					callIdx++
					return fn(ctx, obj, opts...)
				},
			}
			fc := &fakeClient{
				createInstanceFunc: tt.client.createInstanceFunc,
				deleteInstanceFunc: func(ctx context.Context, bindings []v1alpha1.Binding, kymaInstanceId string) error {
					rollbackCalled = true
					return tt.rollbackErr
				},
			}

			c := &external{kube: kube, client: fc}
			got, err := c.Create(context.Background(), tt.mg)
			if (err != nil) != tt.want.err {
				t.Errorf("Create() error = %v, want.err %v", err, tt.want.err)
				return
			}
			if rollbackCalled != tt.want.rollbackCalled {
				t.Errorf("Create() rollbackCalled = %v, want %v", rollbackCalled, tt.want.rollbackCalled)
			}
			if tt.want.errContains != "" && !strings.Contains(err.Error(), tt.want.errContains) {
				t.Errorf("expected error containing %q, got %v", tt.want.errContains, err)
			}
			if diff := cmp.Diff(tt.want.connectionDetails, got.ConnectionDetails,
				cmp.FilterPath(func(p cmp.Path) bool {
					return p.Last().String() == "[\"created_at\"]" || p.Last().String() == "[\"expires_at\"]"
				}, cmp.Ignore())); diff != "" {
				t.Errorf("Create() connectionDetails mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func Test_external_Update(t *testing.T) {
	tests := []struct {
		name    string
		mg      resource.Managed
		client  *fakeClient
		want    managed.ExternalUpdate
		wantErr bool
	}{
		{
			name:    "not a KymaEnvironmentBinding",
			mg:      &v1alpha1.KymaEnvironment{},
			want:    managed.ExternalUpdate{},
			wantErr: true,
		},
		{
			name: "update not implemented",
			mg: &v1alpha1.KymaEnvironmentBinding{
				Spec: v1alpha1.KymaEnvironmentBindingSpec{
					KymaEnvironmentId: "test-instance",
				},
			},
			want:    managed.ExternalUpdate{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &external{kube: test.NewMockClient(), client: tt.client}
			got, err := c.Update(context.Background(), tt.mg)
			if (err != nil) != tt.wantErr {
				t.Errorf("Update() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if diff := cmp.Diff(tt.want, got,
				cmp.FilterPath(func(p cmp.Path) bool {
					return p.Last().String() == "[\"created_at\"]" || p.Last().String() == "[\"expires_at\"]"
				}, cmp.Ignore()),
				cmp.AllowUnexported(managed.ExternalUpdate{})); diff != "" {
				t.Errorf("Update() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

type fakeClient struct {
	describeInstanceFunc func(ctx context.Context, kymaInstanceId string) ([]provisioningclient.EnvironmentInstanceBindingMetadata, error)
	createInstanceFunc   func(ctx context.Context, kymaInstanceId string, ttl int) (*kymaenvironmentbinding.Binding, error)
	deleteInstanceFunc   func(ctx context.Context, bindings []v1alpha1.Binding, kymaInstanceId string) error
}

func (f fakeClient) DescribeInstance(ctx context.Context, kymaInstanceId string) ([]provisioningclient.EnvironmentInstanceBindingMetadata, error) {
	if f.describeInstanceFunc != nil {
		return f.describeInstanceFunc(ctx, kymaInstanceId)
	}
	return nil, nil
}

func (f fakeClient) CreateInstance(ctx context.Context, kymaInstanceId string, ttl int) (*kymaenvironmentbinding.Binding, error) {
	if f.createInstanceFunc != nil {
		return f.createInstanceFunc(ctx, kymaInstanceId, ttl)
	}
	return nil, nil
}

func (f fakeClient) DeleteInstances(ctx context.Context, bindings []v1alpha1.Binding, kymaInstanceId string) error {
	if f.deleteInstanceFunc != nil {
		return f.deleteInstanceFunc(ctx, bindings, kymaInstanceId)
	}
	return nil
}

var _ kymaenvironmentbinding.Client = &fakeClient{}

func Test_external_updateStatusWithRetry(t *testing.T) {
	conflictErr := kerrors.NewConflict(schema.GroupResource{}, "test", errors.New("conflict"))

	tests := []struct {
		name            string
		statusUpdateFns []test.MockSubResourceUpdateFn
		getFn           test.MockGetFn
		cancelCtx       bool
		wantErr         bool
		wantErrContains string
	}{
		{
			name: "success on first attempt",
			statusUpdateFns: []test.MockSubResourceUpdateFn{
				test.NewMockSubResourceUpdateFn(nil),
			},
			wantErr: false,
		},
		{
			name: "conflict on first attempt, success on retry after re-fetch",
			statusUpdateFns: []test.MockSubResourceUpdateFn{
				test.NewMockSubResourceUpdateFn(conflictErr),
				test.NewMockSubResourceUpdateFn(nil),
			},
			getFn:   test.NewMockGetFn(nil),
			wantErr: false,
		},
		{
			name: "non-conflict error returns immediately without retrying",
			statusUpdateFns: []test.MockSubResourceUpdateFn{
				test.NewMockSubResourceUpdateFn(errors.New("some other error")),
			},
			wantErr:         true,
			wantErrContains: "some other error",
		},
		{
			name: "conflict exhausts all retries",
			statusUpdateFns: []test.MockSubResourceUpdateFn{
				test.NewMockSubResourceUpdateFn(conflictErr),
				test.NewMockSubResourceUpdateFn(conflictErr),
				test.NewMockSubResourceUpdateFn(conflictErr),
				test.NewMockSubResourceUpdateFn(conflictErr),
				test.NewMockSubResourceUpdateFn(conflictErr),
			},
			getFn:           test.NewMockGetFn(nil),
			wantErr:         true,
			wantErrContains: errStatusUpdate,
		},
		{
			name: "re-fetch fails returns immediately",
			statusUpdateFns: []test.MockSubResourceUpdateFn{
				test.NewMockSubResourceUpdateFn(conflictErr),
			},
			getFn:           test.NewMockGetFn(errors.New("get failed")),
			wantErr:         true,
			wantErrContains: "get failed",
		},
		{
			name: "context cancelled during backoff returns immediately",
			statusUpdateFns: []test.MockSubResourceUpdateFn{
				test.NewMockSubResourceUpdateFn(conflictErr),
			},
			getFn:           test.NewMockGetFn(nil),
			cancelCtx:       true,
			wantErr:         true,
			wantErrContains: errStatusUpdate,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callIdx := 0
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			kube := &test.MockClient{
				MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
					if tt.cancelCtx {
						cancel()
					}
					if tt.getFn != nil {
						return tt.getFn(ctx, key, obj)
					}
					return nil
				},
				MockStatusUpdate: func(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
					fn := tt.statusUpdateFns[callIdx]
					callIdx++
					return fn(ctx, obj, opts...)
				},
			}
			c := &external{kube: kube}
			cr := &v1alpha1.KymaEnvironmentBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
			}
			desiredBindings := []v1alpha1.Binding{{Id: "id1", IsActive: true}}
			err := c.updateStatusWithRetry(ctx, cr, desiredBindings)
			if (err != nil) != tt.wantErr {
				t.Errorf("updateStatusWithRetry() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErrContains != "" && err != nil {
				if !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Errorf("expected error containing %q, got %v", tt.wantErrContains, err)
				}
			}
		})
	}
}

