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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
		name             string
		args             args
		wantValid        bool
		wantValidCount   int
		wantActiveCount  int
		wantExpiredCount int
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
			wantValid:        false,
			wantValidCount:   0,
			wantActiveCount:  0,
			wantExpiredCount: 1,
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
			wantValid:        false,
			wantValidCount:   1,
			wantActiveCount:  0,
			wantExpiredCount: 0,
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
			wantValid:        true,
			wantValidCount:   1,
			wantActiveCount:  1,
			wantExpiredCount: 0,
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
			wantValid:        false,
			wantValidCount:   0,
			wantActiveCount:  0,
			wantExpiredCount: 1,
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
			wantValid:        false,
			wantValidCount:   0,
			wantActiveCount:  0,
			wantExpiredCount: 0,
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
			wantValid:        false,
			wantValidCount:   0,
			wantActiveCount:  0,
			wantExpiredCount: 0,
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
			wantValid:        false,
			wantValidCount:   1,
			wantActiveCount:  0,
			wantExpiredCount: 0,
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
			wantValid:        false,
			wantValidCount:   1,
			wantActiveCount:  0,
			wantExpiredCount: 1,
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
			wantValid:        false,
			wantValidCount:   0,
			wantActiveCount:  0,
			wantExpiredCount: 1,
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
			wantValid:        false,
			wantValidCount:   1,
			wantActiveCount:  0,
			wantExpiredCount: 0,
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
			wantValid:        false,
			wantValidCount:   2,
			wantActiveCount:  0,
			wantExpiredCount: 0,
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
			wantValid:        false,
			wantValidCount:   1,
			wantActiveCount:  0,
			wantExpiredCount: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &external{kube: test.NewMockClient()}
			gotValid, gotValidBindings, gotExpiredBindings := c.validateBindings(tt.args.cr)

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
			if len(gotExpiredBindings) != tt.wantExpiredCount {
				t.Errorf("validateBindings() expired count = %v, want %v", len(gotExpiredBindings), tt.wantExpiredCount)
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
		name               string
		args               args
		client             *fakeClient
		want               managed.ExternalObservation
		wantErr            bool
		expectedStatus     v1alpha1.KymaEnvironmentBindingObservation
		wantDeletedIds     []string
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
			wantDeletedIds: []string{"id"},
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
			// Track which binding IDs are deleted via the BTP API
			var deletedIds []string
			testClient := tt.client
			if testClient != nil {
				originalDeleteFunc := testClient.deleteInstanceFunc
				testClient.deleteInstanceFunc = func(ctx context.Context, bindings []v1alpha1.Binding, kymaInstanceId string) error {
					for _, b := range bindings {
						deletedIds = append(deletedIds, b.Id)
					}
					if originalDeleteFunc != nil {
						return originalDeleteFunc(ctx, bindings, kymaInstanceId)
					}
					return nil
				}
			}

			c := &external{kube: test.NewMockClient(), client: testClient}
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
			// Assert expired bindings were deleted via BTP API
			if diff := cmp.Diff(tt.wantDeletedIds, deletedIds); diff != "" {
				t.Errorf("Deleted binding IDs mismatch (-want +got):\n%s", diff)
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
						KymaEnvironmentId: "test-instance",
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
			c := &external{kube: test.NewMockClient(), client: tt.client, tracker: tracking.NewDefaultReferenceResolverTracker(test.NewMockClient())}
			_, err := c.Delete(tt.args.ctx, tt.args.mg)
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
						KymaEnvironmentId: "error-instance",
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
						KymaEnvironmentId: "invalid-instance",
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
						KymaEnvironmentId: "test-instance",
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

// fakeStatusWriter is a mock implementation for testing status update retry logic
type fakeStatusWriter struct {
	updateFn func(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error
}

func (f *fakeStatusWriter) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	if f.updateFn != nil {
		return f.updateFn(ctx, obj, opts...)
	}
	return nil
}

func (f *fakeStatusWriter) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	return nil
}

func (f *fakeStatusWriter) Create(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceCreateOption) error {
	return nil
}

// fakeKubeClient is a mock implementation for testing status update retry logic
type fakeKubeClient struct {
	test.MockClient
	statusWriter *fakeStatusWriter
	getFn        func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error
}

func (f *fakeKubeClient) Status() client.SubResourceWriter {
	return f.statusWriter
}

func (f *fakeKubeClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if f.getFn != nil {
		return f.getFn(ctx, key, obj, opts...)
	}
	return nil
}

func Test_external_updateStatusWithRetry(t *testing.T) {
	type args struct {
		ctx        context.Context
		cr         *v1alpha1.KymaEnvironmentBinding
		maxRetries int
		mutate     func(*v1alpha1.KymaEnvironmentBinding)
	}

	noop := func(*v1alpha1.KymaEnvironmentBinding) {}

	tests := []struct {
		name     string
		args     args
		updateFn func(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error
		getFn    func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error
		wantErr  bool
	}{
		{
			name: "success on first try",
			args: args{
				ctx: context.Background(),
				cr: &v1alpha1.KymaEnvironmentBinding{
					ObjectMeta: metav1.ObjectMeta{Name: "test-binding", Namespace: "default"},
				},
				maxRetries: 5,
				mutate:     noop,
			},
			updateFn: func(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
				return nil
			},
			wantErr: false,
		},
		{
			name: "all retries fail with conflict",
			args: args{
				ctx: context.Background(),
				cr: &v1alpha1.KymaEnvironmentBinding{
					ObjectMeta: metav1.ObjectMeta{Name: "test-binding", Namespace: "default"},
				},
				maxRetries: 3,
				mutate:     noop,
			},
			getFn: func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				return nil
			},
			updateFn: func(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
				return apierrors.NewConflict(schema.GroupResource{}, "test-binding", errors.New("conflict"))
			},
			wantErr: true,
		},
		{
			name: "conflict resolved by re-fetch and retry",
			args: args{
				ctx: context.Background(),
				cr: &v1alpha1.KymaEnvironmentBinding{
					ObjectMeta: metav1.ObjectMeta{Name: "test-binding", Namespace: "default", ResourceVersion: "1"},
				},
				maxRetries: 3,
				mutate: func(cr *v1alpha1.KymaEnvironmentBinding) {
					cr.Status.AtProvider.Bindings = append(cr.Status.AtProvider.Bindings, v1alpha1.Binding{Id: "new-id", IsActive: true})
				},
			},
			getFn: func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				// Simulate re-fetch returning a fresher resource version
				cr := obj.(*v1alpha1.KymaEnvironmentBinding)
				cr.ResourceVersion = "2"
				cr.Status.AtProvider.Bindings = nil
				return nil
			},
			updateFn: func() func(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
				attempt := 0
				return func(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
					attempt++
					if attempt == 1 {
						return apierrors.NewConflict(schema.GroupResource{}, "test-binding", errors.New("conflict"))
					}
					// On second attempt verify the mutate was re-applied after re-fetch
					cr := obj.(*v1alpha1.KymaEnvironmentBinding)
					if len(cr.Status.AtProvider.Bindings) != 1 || cr.Status.AtProvider.Bindings[0].Id != "new-id" {
						return errors.New("mutate was not re-applied after re-fetch")
					}
					return nil
				}
			}(),
			wantErr: false,
		},
		{
			name: "get failure on retry returns error",
			args: args{
				ctx: context.Background(),
				cr: &v1alpha1.KymaEnvironmentBinding{
					ObjectMeta: metav1.ObjectMeta{Name: "test-binding", Namespace: "default"},
				},
				maxRetries: 3,
				mutate:     noop,
			},
			getFn: func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				return errors.New("api server down")
			},
			updateFn: func(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
				return apierrors.NewConflict(schema.GroupResource{}, "test-binding", errors.New("conflict"))
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &fakeKubeClient{
				statusWriter: &fakeStatusWriter{updateFn: tt.updateFn},
				getFn:        tt.getFn,
			}
			c := &external{kube: mockClient}
			err := c.updateStatusWithRetry(tt.args.ctx, tt.args.cr, tt.args.maxRetries, tt.args.mutate)
			if (err != nil) != tt.wantErr {
				t.Errorf("updateStatusWithRetry() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
