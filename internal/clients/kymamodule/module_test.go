package kymamodule

import (
	"context"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	"k8s.io/apimachinery/pkg/types"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetDefaultKyma(t *testing.T) {

	type args struct {
		obj *v1alpha1.KymaCr
	}

	type want struct {
		payload *v1alpha1.KymaCr
		err     error
	}

	cases := map[string]struct {
		args args
		want want
	}{
		"Happy Path": {
			args: args{
				obj: kymaCr(withSpec(v1alpha1.KymaSpec{})),
			},
			want: want{
				payload: kymaCr(),
				err:     nil,
			},
		},
	}

	for name, tc := range cases {
		t.Run(
			name, func(t *testing.T) {

				kymaCR, err := getDefaultKyma(context.TODO(), &KymaModuleClient{
					kube: &test.MockClient{MockGet: test.NewMockGetFn(nil)},
				})

				if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
					t.Errorf("\nKymaModuleClient\n.getDefaultKyma(...): -want error, +got error:\n%s\n", diff)
				}

				if diff := cmp.Diff(tc.want.payload, kymaCR); diff != "" {
					t.Errorf("\nKymaModuleClient\n.getDefaultKyma(...): -want, +got:\n%s\n", diff)
				}
			},
		)
	}

}

type kymaModifier func(kymaModule *v1alpha1.KymaCr)

func kymaCr(m ...kymaModifier) *v1alpha1.KymaCr {
	cr := &v1alpha1.KymaCr{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kymaCr",
		},
	}
	for _, f := range m {
		f(cr)
	}
	return cr
}

func withConditions(c ...xpv1.Condition) kymaModifier {
	return func(r *v1alpha1.KymaCr) {}
}
func withUID(uid types.UID) kymaModifier {
	return func(r *v1alpha1.KymaCr) { r.UID = uid }
}

func withStatus(status v1alpha1.KymaStatus) kymaModifier {
	return func(r *v1alpha1.KymaCr) {
		r.Status = status
	}
}
func withSpec(spec v1alpha1.KymaSpec) kymaModifier {
	return func(r *v1alpha1.KymaCr) {
		r.Spec = spec
	}
}

func withAnnotaions(annotations map[string]string) kymaModifier {
	return func(r *v1alpha1.KymaCr) {
		r.ObjectMeta.Annotations = annotations
	}
}

func kymaDefaultSpec() v1alpha1.KymaSpec {
	return v1alpha1.KymaSpec{
		Channel: "fast",
		Modules: []v1alpha1.Module{
			{
				Name:                 "istio",
				CustomResourcePolicy: "CreateAndDelete",
			},
			{
				Name:                 "api-gateway",
				CustomResourcePolicy: "CreateAndDelete",
			},
			{
				Name:                 "btp-operator",
				CustomResourcePolicy: "CreateAndDelete",
			},
		},
	}
}
