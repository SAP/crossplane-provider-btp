package kymamodule

import (
	"context"
	"errors"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetDefaultKyma(t *testing.T) {

	type args struct {
		obj    *v1alpha1.KymaCr
		client *KymaModuleClient
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
				client: &KymaModuleClient{
					kube: &test.MockClient{MockGet: test.NewMockGetFn(nil)},
				},
			},
			want: want{
				payload: kymaCr(withGVK(GVKKyma)),
				err:     nil,
			},
		},
		"Boom!": {
			args: args{
				obj: kymaCr(withSpec(v1alpha1.KymaSpec{})),
				client: &KymaModuleClient{
					kube: &test.MockClient{MockGet: test.NewMockGetFn(errors.New("BOOM"))},
				},
			},
			want: want{
				payload: nil,
				err:     errors.New("BOOM"),
			},
		},
	}

	for name, tc := range cases {
		t.Run(
			name, func(t *testing.T) {

				kymaCR, err := getDefaultKyma(context.TODO(), tc.args.client)

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
			Name:      DefaultKymaName,
			Namespace: DefaultKymaNamespace,
		},
	}
	for _, f := range m {
		f(cr)
	}
	return cr
}

func withGVK(gvk schema.GroupVersionKind) kymaModifier {
	return func(r *v1alpha1.KymaCr) {
		r.SetGroupVersionKind(gvk)
	}
}

func withSpec(spec v1alpha1.KymaSpec) kymaModifier {
	return func(r *v1alpha1.KymaCr) {
		r.Spec = spec
	}
}
