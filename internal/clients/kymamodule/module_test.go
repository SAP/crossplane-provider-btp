package kymamodule

import (
	"context"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	client "sigs.k8s.io/controller-runtime/pkg/client"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestEnableModule(t *testing.T) {
	type args struct {
		initialModules []Module
		moduleName     string
		moduleChannel  string
		customPolicy   string
	}
	type want struct {
		modules []Module
	}
	cases := map[string]struct {
		args args
		want want
	}{
		"Add New Module": {
			args: args{
				initialModules: []Module{},
				moduleName:     "mod1",
				moduleChannel:  "stable",
				customPolicy:   "policy1",
			},
			want: want{
				modules: []Module{
					{Name: "mod1", Channel: "stable", CustomResourcePolicy: "policy1"},
				},
			},
		},
		"Update Existing Module": {
			args: args{
				initialModules: []Module{
					{Name: "mod2", Channel: "old", CustomResourcePolicy: "oldpolicy"},
				},
				moduleName:    "mod2",
				moduleChannel: "new",
				customPolicy:  "newpolicy",
			},
			want: want{
				modules: []Module{
					{Name: "mod2", Channel: "new", CustomResourcePolicy: "newpolicy"},
				},
			},
		},
		"Add Module When Others Exist": {
			args: args{
				initialModules: []Module{
					{Name: "modA", Channel: "chanA", CustomResourcePolicy: "policyA"},
				},
				moduleName:    "modB",
				moduleChannel: "chanB",
				customPolicy:  "policyB",
			},
			want: want{
				modules: []Module{
					{Name: "modA", Channel: "chanA", CustomResourcePolicy: "policyA"},
					{Name: "modB", Channel: "chanB", CustomResourcePolicy: "policyB"},
				},
			},
		},
		"Update One Of Multiple Modules": {
			args: args{
				initialModules: []Module{
					{Name: "modA", Channel: "chanA", CustomResourcePolicy: "policyA"},
					{Name: "modB", Channel: "chanB", CustomResourcePolicy: "policyB"},
				},
				moduleName:    "modB",
				moduleChannel: "chanB2",
				customPolicy:  "policyB2",
			},
			want: want{
				modules: []Module{
					{Name: "modA", Channel: "chanA", CustomResourcePolicy: "policyA"},
					{Name: "modB", Channel: "chanB2", CustomResourcePolicy: "policyB2"},
				},
			},
		},
		"Add Module With Empty Channel And Policy": {
			args: args{
				initialModules: []Module{},
				moduleName:     "modX",
				moduleChannel:  "",
				customPolicy:   "",
			},
			want: want{
				modules: []Module{
					{Name: "modX", Channel: "", CustomResourcePolicy: ""},
				},
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			kyma := &KymaCr{
				Spec: KymaSpec{
					Modules: tc.args.initialModules,
				},
			}
			got := enableModule(kyma, tc.args.moduleName, tc.args.moduleChannel, tc.args.customPolicy)
			if diff := cmp.Diff(tc.want.modules, got.Spec.Modules); diff != "" {
				t.Errorf("enableModule() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestDisableModule(t *testing.T) {
	type args struct {
		initialModules []Module
		moduleName     string
	}
	type want struct {
		modules []Module
	}
	cases := map[string]struct {
		args args
		want want
	}{
		"Remove Existing Module": {
			args: args{
				initialModules: []Module{
					{Name: "mod1", Channel: "stable", CustomResourcePolicy: "policy1"},
					{Name: "mod2", Channel: "beta", CustomResourcePolicy: "policy2"},
				},
				moduleName: "mod1",
			},
			want: want{
				modules: []Module{
					{Name: "mod2", Channel: "beta", CustomResourcePolicy: "policy2"},
				},
			},
		},
		"Remove Nonexistent Module": {
			args: args{
				initialModules: []Module{
					{Name: "mod1", Channel: "stable", CustomResourcePolicy: "policy1"},
				},
				moduleName: "modX",
			},
			want: want{
				modules: []Module{
					{Name: "mod1", Channel: "stable", CustomResourcePolicy: "policy1"},
				},
			},
		},
		"Remove Last Module": {
			args: args{
				initialModules: []Module{
					{Name: "mod1", Channel: "stable", CustomResourcePolicy: "policy1"},
				},
				moduleName: "mod1",
			},
			want: want{
				modules: []Module{},
			},
		},
		"Remove From Empty List": {
			args: args{
				initialModules: []Module{},
				moduleName:     "mod1",
			},
			want: want{
				modules: []Module{},
			},
		},
		"Remove One Of Multiple Modules": {
			args: args{
				initialModules: []Module{
					{Name: "modA", Channel: "chanA", CustomResourcePolicy: "policyA"},
					{Name: "modB", Channel: "chanB", CustomResourcePolicy: "policyB"},
					{Name: "modC", Channel: "chanC", CustomResourcePolicy: "policyC"},
				},
				moduleName: "modB",
			},
			want: want{
				modules: []Module{
					{Name: "modA", Channel: "chanA", CustomResourcePolicy: "policyA"},
					{Name: "modC", Channel: "chanC", CustomResourcePolicy: "policyC"},
				},
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			kyma := &KymaCr{
				Spec: KymaSpec{
					Modules: tc.args.initialModules,
				},
			}
			got := disableModule(kyma, tc.args.moduleName)
			if diff := cmp.Diff(tc.want.modules, got.Spec.Modules); diff != "" {
				t.Errorf("disableModule() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestUnstructuredConversionToKymaCr(t *testing.T) {
	type want struct {
		payload *KymaCr
		err     error
	}

	cases := map[string]struct {
		objMap map[string]interface{}
		expect want
	}{
		"Successful Conversion": {
			objMap: map[string]interface{}{
				"apiVersion": GVKKyma.GroupVersion().String(),
				"kind":       GVKKyma.Kind,
				"metadata": map[string]interface{}{
					"name":      DefaultKymaName,
					"namespace": DefaultKymaNamespace,
				},
			},
			expect: want{
				payload: &KymaCr{
					ObjectMeta: metav1.ObjectMeta{
						Name:      DefaultKymaName,
						Namespace: DefaultKymaNamespace,
					},
				},
				err: nil,
			},
		},
		"Conversion Error": {
			objMap: map[string]interface{}{
				"apiVersion": make(chan int), // channels can't be converted - force conversion error - otherwise fields are ignored/set to nil
			},
			expect: want{
				payload: nil,
				err:     errors.Wrap(errors.New(""), errFailedConvertToUnstructured),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			obj := &unstructured.Unstructured{Object: tc.objMap}
			mg := &KymaCr{}
			err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, mg)

			if tc.expect.err != nil {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
			} else {
				if diff := cmp.Diff(tc.expect.payload.ObjectMeta, mg.ObjectMeta); diff != "" {
					t.Errorf("KymaCr ObjectMeta mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestGetDefaultKyma(t *testing.T) {

	type args struct {
		obj    *KymaCr
		client *KymaModuleClient
	}

	type want struct {
		payload *KymaCr
		err     error
	}

	cases := map[string]struct {
		args args
		want want
	}{
		"Happy Path": {
			args: args{
				obj: kymaCr(withSpec(KymaSpec{})),
				client: &KymaModuleClient{
					kube: &test.MockClient{MockGet: test.NewMockGetFn(nil)},
				},
			},
			want: want{
				payload: kymaCr(withGVK(GVKKyma)),
				err:     nil,
			},
		},
		"Api Not Available": {
			args: args{
				obj: kymaCr(withSpec(KymaSpec{})),
				client: &KymaModuleClient{
					kube: &test.MockClient{MockGet: test.NewMockGetFn(errors.New("CRASH"))},
				},
			},
			want: want{
				payload: nil,
				err:     errors.Wrap(errors.New("CRASH"), errFailedGetDefaultKyma),
			},
		},
		"Conversion Error": {
			args: args{
				client: &KymaModuleClient{
					kube: &test.MockClient{
						MockGet: func(_ context.Context, _ client.ObjectKey, obj client.Object) error {
							u := obj.(*unstructured.Unstructured)
							u.Object = map[string]interface{}{
								"apiVersion": make(chan int), // will cause conversion error
							}
							return nil
						},
					},
				},
			},
			want: want{
				payload: nil,
				err:     errors.Wrap(errors.New("unrecognized type: string"), errFailedConvertToUnstructured),
			},
		},
	}

	for name, tc := range cases {
		t.Run(
			name, func(t *testing.T) {

				kymaCR, err := getDefaultKyma(context.Background(), tc.args.client)

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

type kymaModifier func(kymaModule *KymaCr)

func kymaCr(m ...kymaModifier) *KymaCr {
	cr := &KymaCr{
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
	return func(r *KymaCr) {
		r.SetGroupVersionKind(gvk)
	}
}

func withSpec(spec KymaSpec) kymaModifier {
	return func(r *KymaCr) {
		r.Spec = spec
	}
}
