package reconcilerutil_test

import (
	"context"
	"errors"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal/reconcilerutil"
)

// fakeStatusWriter is a mock for testing status update retry logic
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

// fakeKubeClient is a mock for testing status update retry logic
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

func TestUpdateStatusWithRetry(t *testing.T) {
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
		{
			name: "context cancelled during backoff returns error",
			args: args{
				ctx: func() context.Context {
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					return ctx
				}(),
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &fakeKubeClient{
				statusWriter: &fakeStatusWriter{updateFn: tt.updateFn},
				getFn:        tt.getFn,
			}
			err := reconcilerutil.UpdateStatusWithRetry(tt.args.ctx, mockClient, tt.args.cr, tt.args.maxRetries, tt.args.mutate)
			if (err != nil) != tt.wantErr {
				t.Errorf("UpdateStatusWithRetry() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
