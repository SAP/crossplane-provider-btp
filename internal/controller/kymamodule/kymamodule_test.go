package kymamodule

import (
	"context"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	"github.com/sap/crossplane-provider-btp/internal/clients/kymamodule"
	"github.com/sap/crossplane-provider-btp/internal/controller/kymamodule/fake"
	"k8s.io/apimachinery/pkg/types"
)

func TestObserve(t *testing.T) {

	type args struct {
		cr     resource.Managed
		client kymamodule.Client
	}

	type want struct {
		cr  resource.Managed
		obs managed.ExternalObservation
		err error
	}

	cases := map[string]struct {
		args args
		want want
	}{
		"Happy Path": {
			args: args{
				cr: module(),
				client: &fake.MockKymaModuleClient{MockObserve: func(moduleCr *v1alpha1.KymaModule) (*v1alpha1.ModuleStatus, error) {
					return &v1alpha1.ModuleStatus{}, nil
				}},
			},
			want: want{
				cr:  module(),
				obs: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true},
				err: nil,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{client: tc.args.client}
			got, err := e.Observe(context.Background(), tc.args.cr)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\ne.Observe(...): -want error, +got error:\n%s\n", diff)
			}
			if diff := cmp.Diff(tc.want.obs, got); diff != "" {
				t.Errorf("\ne.Observe(...): -want, +got:\n%s\n", diff)
			}
		})
	}
}

func moduleStatus(state string) *v1alpha1.ModuleStatus {
	return &v1alpha1.ModuleStatus{State: state}
}

type moduleModifier func(kymaModule *v1alpha1.KymaModule)

func withConditions(c ...xpv1.Condition) moduleModifier {
	return func(r *v1alpha1.KymaModule) { r.Status.ConditionedStatus.Conditions = c }
}
func withUID(uid types.UID) moduleModifier {
	return func(r *v1alpha1.KymaModule) { r.UID = uid }
}

func withState(status string) moduleModifier {
	return func(r *v1alpha1.KymaModule) {
		r.Status.AtProvider.State = status
	}
}
func withData(data v1alpha1.KymaModuleParameters) moduleModifier {
	return func(r *v1alpha1.KymaModule) {
		r.Spec.ForProvider = data
	}
}

func withAnnotaions(annotations map[string]string) moduleModifier {
	return func(r *v1alpha1.KymaModule) {
		r.ObjectMeta.Annotations = annotations
	}
}

func module(m ...moduleModifier) *v1alpha1.KymaModule {
	cr := &v1alpha1.KymaModule{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kymaModule",
		},
	}
	for _, f := range m {
		f(cr)
	}
	return cr
}
