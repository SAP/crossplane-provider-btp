package kymaenvironmentbinding

import (
	"context"
	"errors"
	"testing"
	"time"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	managed "github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal/clients/kymaenvironmentbinding"
	provisioningclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-provisioning-service-api-go/pkg"
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
	type args struct {
		ctx context.Context
		mg  resource.Managed
	}
	tests := []struct {
		name    string
		args    args
		client  *fakeClient
		wantErr bool
	}{
		{
			name: "not a KymaEnvironmentBinding",
			args: args{
				ctx: context.Background(),
				mg:  &v1alpha1.KymaEnvironment{},
			},
			client:  &fakeClient{},
			wantErr: true,
		},
		{
			name: "successful deletion with multiple bindings",
			args: args{
				ctx: context.Background(),
				mg: &v1alpha1.KymaEnvironmentBinding{
					Spec: v1alpha1.KymaEnvironmentBindingSpec{
						KymaInstanceId: "test-instance",
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
				deleteInstanceFunc: func(ctx context.Context, bindings []v1alpha1.Binding, kymaInstanceId string) error {
					return nil
				},
			},
			wantErr: false,
		},
		{
			name: "service returns error during deletion",
			args: args{
				ctx: context.Background(),
				mg: &v1alpha1.KymaEnvironmentBinding{
					Spec: v1alpha1.KymaEnvironmentBindingSpec{
						KymaInstanceId: "error-instance",
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
				deleteInstanceFunc: func(ctx context.Context, bindings []v1alpha1.Binding, kymaInstanceId string) error {
					return errors.New("service error")
				},
			},
			wantErr: true,
		},
		{
			name: "service returns error for non-existent binding",
			args: args{
				ctx: context.Background(),
				mg: &v1alpha1.KymaEnvironmentBinding{
					Spec: v1alpha1.KymaEnvironmentBindingSpec{
						KymaInstanceId: "non-existent-instance",
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
			args: args{
				ctx: context.Background(),
				mg: &v1alpha1.KymaEnvironmentBinding{
					Spec: v1alpha1.KymaEnvironmentBindingSpec{
						KymaInstanceId: "test-instance",
					},
					Status: v1alpha1.KymaEnvironmentBindingStatus{
						AtProvider: v1alpha1.KymaEnvironmentBindingObservation{
							Bindings: []v1alpha1.Binding{},
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &external{kube: test.NewMockClient(), client: tt.client}
			err := c.Delete(tt.args.ctx, tt.args.mg)
			if (err != nil) != tt.wantErr {
				t.Errorf("Delete() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_external_Create(t *testing.T) {
	type args struct {
		ctx context.Context
		mg  resource.Managed
	}
	tests := []struct {
		name    string
		args    args
		client  *fakeClient
		want    managed.ExternalCreation
		wantErr bool
	}{
		{
			name: "not a KymaEnvironmentBinding",
			args: args{
				ctx: context.Background(),
				mg:  &v1alpha1.KymaEnvironment{},
			},
			client:  &fakeClient{},
			want:    managed.ExternalCreation{},
			wantErr: true,
		},
		{
			name: "create new binding when no valid bindings exist",
			args: args{
				ctx: context.Background(),
				mg: &v1alpha1.KymaEnvironmentBinding{
					Spec: v1alpha1.KymaEnvironmentBindingSpec{
						KymaInstanceId: "test-instance",
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
			},
			client: &fakeClient{
				createInstanceFunc: func(ctx context.Context, kymaInstanceId string, ttl int) (*kymaenvironmentbinding.Binding, error) {
					return &kymaenvironmentbinding.Binding{
						Metadata: &kymaenvironmentbinding.Metadata{
							Id:        "new-binding-id",
							ExpiresAt: timeNow.Add(time.Hour * 2),
						},
						Credentials: &kymaenvironmentbinding.Credentials{
							Kubeconfig: "new-binding-secret",
						},
					}, nil
				},
			},
			want: managed.ExternalCreation{
				ConnectionDetails: managed.ConnectionDetails{
					"binding_id": []byte("new-binding-id"),
					"kubeconfig": []byte("new-binding-secret"),
				},
			},
			wantErr: false,
		},
		{
			name: "reuse existing valid binding",
			args: args{
				ctx: context.Background(),
				mg: &v1alpha1.KymaEnvironmentBinding{
					Spec: v1alpha1.KymaEnvironmentBindingSpec{
						KymaInstanceId: "test-instance",
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
			},
			client: &fakeClient{
				createInstanceFunc: func(ctx context.Context, kymaInstanceId string, ttl int) (*kymaenvironmentbinding.Binding, error) {
					return &kymaenvironmentbinding.Binding{
						Metadata: &kymaenvironmentbinding.Metadata{
							Id:        "valid-id",
							ExpiresAt: timeNow.Add(time.Hour * 2),
						},
						Credentials: &kymaenvironmentbinding.Credentials{
							Kubeconfig: "valid-id",
						},
					}, nil
				},
			},
			want: managed.ExternalCreation{
				ConnectionDetails: managed.ConnectionDetails{
					"binding_id": []byte("valid-id"),
					"kubeconfig": []byte("valid-id"),
				},
			},
			wantErr: false,
		},
		{
			name: "service returns error during creation",
			args: args{
				ctx: context.Background(),
				mg: &v1alpha1.KymaEnvironmentBinding{
					Spec: v1alpha1.KymaEnvironmentBindingSpec{
						KymaInstanceId: "error-instance",
						ForProvider: v1alpha1.KymaEnvironmentBindingParameters{
							RotationInterval: metav1.Duration{Duration: time.Hour * 1},
						},
					},
					Status: v1alpha1.KymaEnvironmentBindingStatus{
						AtProvider: v1alpha1.KymaEnvironmentBindingObservation{
							Bindings: []v1alpha1.Binding{},
						},
					},
				},
			},
			client: &fakeClient{
				createInstanceFunc: func(ctx context.Context, kymaInstanceId string, ttl int) (*kymaenvironmentbinding.Binding, error) {
					return nil, errors.New("service error")
				},
			},
			want:    managed.ExternalCreation{},
			wantErr: true,
		},
		{
			name: "service returns error for invalid instance",
			args: args{
				ctx: context.Background(),
				mg: &v1alpha1.KymaEnvironmentBinding{
					Spec: v1alpha1.KymaEnvironmentBindingSpec{
						KymaInstanceId: "invalid-instance",
						ForProvider: v1alpha1.KymaEnvironmentBindingParameters{
							RotationInterval: metav1.Duration{Duration: time.Hour * 1},
						},
					},
					Status: v1alpha1.KymaEnvironmentBindingStatus{
						AtProvider: v1alpha1.KymaEnvironmentBindingObservation{
							Bindings: []v1alpha1.Binding{},
						},
					},
				},
			},
			client: &fakeClient{
				createInstanceFunc: func(ctx context.Context, kymaInstanceId string, ttl int) (*kymaenvironmentbinding.Binding, error) {
					return nil, errors.New("invalid instance")
				},
			},
			want:    managed.ExternalCreation{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &external{kube: test.NewMockClient(), client: tt.client}
			got, err := c.Create(tt.args.ctx, tt.args.mg)
			if (err != nil) != tt.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if diff := cmp.Diff(tt.want, got,
				cmp.FilterPath(func(p cmp.Path) bool {
					return p.Last().String() == "[\"created_at\"]" || p.Last().String() == "[\"expires_at\"]"
				}, cmp.Ignore()),
				cmp.AllowUnexported(managed.ExternalCreation{})); diff != "" {
				t.Errorf("Create() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func Test_external_Update(t *testing.T) {
	type args struct {
		ctx context.Context
		mg  resource.Managed
	}
	tests := []struct {
		name    string
		args    args
		client  *fakeClient
		want    managed.ExternalUpdate
		wantErr bool
	}{
		{
			name: "not a KymaEnvironmentBinding",
			args: args{
				ctx: context.Background(),
				mg:  &v1alpha1.KymaEnvironment{},
			},
			want:    managed.ExternalUpdate{},
			wantErr: true,
		},
		{
			name: "update not implemented",
			args: args{
				ctx: context.Background(),
				mg: &v1alpha1.KymaEnvironmentBinding{
					Spec: v1alpha1.KymaEnvironmentBindingSpec{
						KymaInstanceId: "test-instance",
					},
				},
			},
			want:    managed.ExternalUpdate{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &external{kube: test.NewMockClient(), client: tt.client}
			got, err := c.Update(tt.args.ctx, tt.args.mg)
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
