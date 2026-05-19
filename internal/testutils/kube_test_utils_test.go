package testutils

import (
	"context"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	"github.com/google/go-cmp/cmp"
	v1alpha1 "github.com/sap/crossplane-provider-btp/apis/cluster/account/v1alpha1"
	v1alpha12 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestGet(t *testing.T) {
	type want struct {
		err bool
		res client.Object
	}
	type args struct {
		kube      test.MockClient
		getName   string
		passedRes client.Object
	}

	type testcase struct {
		args args
		want want
	}

	tests := map[string]testcase{
		"No Converter for type": {
			args: args{
				kube: NewFakeKubeClientBuilder().
					AddResource(NewDirectory("TestResource")).
					Build(),
				getName:   "TestResource",
				passedRes: &v1alpha1.Directory{},
			},
			want: want{
				err: true,
				res: &v1alpha1.Directory{},
			},
		},
		"No Resource with Found with Name": {
			args: args{
				kube: NewFakeKubeClientBuilder().
					RegisterResourceConverter(&v1alpha1.Directory{}, func(srcObj client.Object, destObj client.Object) {
						dir := srcObj.(*v1alpha1.Directory)
						vDir := destObj.(*v1alpha1.Directory)
						vDir.DeepCopyInto(dir)
					}).
					AddResource(NewDirectory("TestResource")).
					Build(),
				getName:   "OtherResource",
				passedRes: &v1alpha1.Directory{},
			},
			want: want{
				err: true,
				res: &v1alpha1.Directory{},
			},
		},
		"Found Resource": {
			args: args{
				kube: NewFakeKubeClientBuilder().
					RegisterResourceConverter(&v1alpha1.Directory{}, func(srcObj client.Object, destObj client.Object) {
						srcDir := srcObj.(*v1alpha1.Directory)
						destDir := destObj.(*v1alpha1.Directory)
						srcDir.DeepCopyInto(destDir)
					}).
					AddResource(NewDirectory("TestResource")).
					Build(),
				getName:   "TestResource",
				passedRes: &v1alpha1.Directory{},
			},
			want: want{
				err: false,
				res: NewDirectory("TestResource"),
			},
		},
		"No Secret with Name": {
			args: args{
				kube: NewFakeKubeClientBuilder().
					AddResource(NewSecret("TestSecret", nil)).
					Build(),
				getName:   "OtherSecret",
				passedRes: &v1.Secret{},
			},
			want: want{
				err: true,
				res: &v1.Secret{},
			},
		},
		"Found Secret": {
			args: args{
				kube: NewFakeKubeClientBuilder().
					AddResource(NewSecret("TestSecret", nil)).
					Build(),
				getName:   "TestSecret",
				passedRes: &v1.Secret{},
			},
			want: want{
				err: false,
				res: NewSecret("TestSecret", nil),
			},
		},
		"No ProviderConfig with Name": {
			args: args{
				kube: NewFakeKubeClientBuilder().
					AddResource(NewProviderConfig("TestPC", "cis-provider-secret", "sa-provider-secret")).
					Build(),
				getName:   "OtherPC",
				passedRes: &v1alpha12.ProviderConfig{},
			},
			want: want{
				err: true,
				res: &v1alpha12.ProviderConfig{},
			},
		},
		"Found ProviderConfig": {
			args: args{
				kube: NewFakeKubeClientBuilder().
					AddResource(NewProviderConfig("TestPC", "cis-provider-secret", "sa-provider-secret")).
					Build(),
				getName:   "TestPC",
				passedRes: &v1alpha12.ProviderConfig{},
			},
			want: want{
				err: false,
				res: NewProviderConfig("TestPC", "cis-provider-secret", "sa-provider-secret"),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			getKey := client.ObjectKey{Namespace: "", Name: tc.args.getName}

			err := tc.args.kube.Get(context.Background(), getKey, tc.args.passedRes)
			if tc.want.err == (err == nil) {
				t.Fatalf("Unexpected receive error result %v", err)
			}
			if diff := cmp.Diff(tc.want.res, tc.args.passedRes, test.EquateConditions()); diff != "" {
				t.Errorf("\ne.Get(...): -want error, +got error:\n%s\n", diff)
			}
		})
	}
}
