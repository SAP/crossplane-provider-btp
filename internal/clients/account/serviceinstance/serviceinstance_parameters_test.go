package serviceinstanceclient

import (
	"context"
	"encoding/json"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// helper to decode JSON into map for comparison
func mustMap(t *testing.T, b []byte) map[string]interface{} {
	m := map[string]interface{}{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("failed to unmarshal test json: %v", err)
	}
	return m
}

func TestBuildComplexParameterJson(t *testing.T) {
	ctx := context.Background()

	type args struct {
		secretRefs []xpv1.SecretKeySelector
		specParams []byte
		client     client.Client
	}
	cases := map[string]struct {
		args    args
		want    map[string]interface{}
		wantErr bool
	}{
		"EmptyBoth": {
			args: args{specParams: []byte{}, secretRefs: []xpv1.SecretKeySelector{}},
			want: map[string]interface{}{},
		},
		"OnlySpecJSON": {
			args: args{specParams: []byte(`{"a":1,"b":"x"}`), secretRefs: []xpv1.SecretKeySelector{}},
			want: map[string]interface{}{"a": float64(1), "b": "x"},
		},
		"OnlySpecYAML": {
			args: args{specParams: []byte("a: 1\nb: x"), secretRefs: []xpv1.SecretKeySelector{}},
			want: map[string]interface{}{"a": 1, "b": "x"},
		},
		"OnlySecretPrimitive": {
			args: args{
				secretRefs: []xpv1.SecretKeySelector{{
					SecretReference: xpv1.SecretReference{Name: "s1", Namespace: "default"},
					Key:             "data",
				}},
				// no spec parameters provided
				specParams: []byte{},
				client: &test.MockClient{MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
					secret := obj.(*corev1.Secret)
					secret.Data = map[string][]byte{"data": []byte(`{"password":"fromSecret"}`)}
					return nil
				}},
			},
			want: map[string]interface{}{"password": "fromSecret"},
		},
		"SecretsAndSpecMergeNoOverlap": {
			args: args{
				secretRefs: []xpv1.SecretKeySelector{{
					SecretReference: xpv1.SecretReference{Name: "s1", Namespace: "default"},
					Key:             "data",
				}},
				specParams: []byte(`{"b":2}`),
				client: &test.MockClient{MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
					if key.Name == "s1" {
						secret := obj.(*corev1.Secret)
						secret.Data = map[string][]byte{"data": []byte(`{"a":1}`)}
					}
					return nil
				}},
			},
			want: map[string]interface{}{"a": float64(1), "b": float64(2)},
		},
		"SecretsAndSpecOverlapSpecWins": {
			args: args{
				secretRefs: []xpv1.SecretKeySelector{{
					SecretReference: xpv1.SecretReference{Name: "s1", Namespace: "default"},
					Key:             "data",
				}},
				specParams: []byte(`{"a":99}`),
				client: &test.MockClient{MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
					secret := obj.(*corev1.Secret)
					secret.Data = map[string][]byte{"data": []byte(`{"a":1,"nested":{"x":1}}`)}
					return nil
				}},
			},
			// spec overwrites top-level a; nested map preserved
			want: map[string]interface{}{"a": float64(99), "nested": map[string]interface{}{"x": float64(1)}},
		},
		"SecretsAndSpecDeepMerge": {
			args: args{
				secretRefs: []xpv1.SecretKeySelector{{
					SecretReference: xpv1.SecretReference{Name: "s1", Namespace: "default"},
					Key:             "data",
				}},
				specParams: []byte(`{"parent":{"b":2}}`),
				client: &test.MockClient{MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
					secret := obj.(*corev1.Secret)
					secret.Data = map[string][]byte{"data": []byte(`{"parent":{"a":1}}`)}
					return nil
				}},
			},
			want: map[string]interface{}{"parent": map[string]interface{}{"a": float64(1), "b": float64(2)}},
		},
		"SecretLookupError": {
			args: args{
				secretRefs: []xpv1.SecretKeySelector{{
					SecretReference: xpv1.SecretReference{Name: "missing", Namespace: "default"},
					Key:             "data",
				}},
				client: &test.MockClient{MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
					return k8serrors.NewNotFound(corev1.Resource("Secret"), key.Name)
				}},
			},
			wantErr: true,
		},
		"CorruptedSecretJSON": {
			args: args{
				secretRefs: []xpv1.SecretKeySelector{{
					SecretReference: xpv1.SecretReference{Name: "s1", Namespace: "default"},
					Key:             "data",
				}},
				specParams: []byte(`{"x":1}`),
				client: &test.MockClient{MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
					secret := obj.(*corev1.Secret)
					secret.Data = map[string][]byte{"data": []byte(`{not-json}`)}
					return nil
				}},
			},
			wantErr: true,
		},
		"SecretsAndSpecNestedSame": {
			args: args{
				secretRefs: []xpv1.SecretKeySelector{{
					SecretReference: xpv1.SecretReference{Name: "s1", Namespace: "default"},
					Key:             "data",
				}},
				specParams: []byte(`{"parent":{"password":"keep"}}`),
				client: &test.MockClient{MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
					secret := obj.(*corev1.Secret)
					secret.Data = map[string][]byte{"data": []byte(`{"parent":{"password":"overwritten"}}`)}
					return nil
				}},
			},
			want: map[string]interface{}{"parent": map[string]interface{}{"password": "keep"}},
		},
		"MapAndNonMapConflictKeepNonMap": {
			args: args{
				secretRefs: []xpv1.SecretKeySelector{{
					SecretReference: xpv1.SecretReference{Name: "s1", Namespace: "default"},
					Key:             "data",
				}},
				specParams: []byte(`{"parent":"not-a-map"}`),
				client: &test.MockClient{MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
					secret := obj.(*corev1.Secret)
					secret.Data = map[string][]byte{"data": []byte(`{"parent":{"a":1}}`)}
					return nil
				}},
			},
			want: map[string]interface{}{"parent": "not-a-map"},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := BuildComplexParameterJson(ctx, tc.args.client, tc.args.secretRefs, tc.args.specParams)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantErr {
				return
			}
			gotMap := mustMap(t, got)
			if diff := cmpDiffMaps(tc.want, gotMap); diff != "" {
				t.Errorf("result mismatch: %s", diff)
			}
		})
	}
}

// cmpDiffMaps creates a deterministic diff for two maps using JSON serialization
func cmpDiffMaps(want, got map[string]interface{}) string {
	wantB, _ := json.Marshal(want)
	gotB, _ := json.Marshal(got)
	if string(wantB) == string(gotB) {
		return ""
	}
	return "want=" + string(wantB) + " got=" + string(gotB)
}

func TestAddMap(t *testing.T) {
	type args struct {
		mergedData map[string]interface{}
		toAdd      map[string]interface{}
	}
	cases := map[string]struct {
		args args
		want map[string]interface{}
	}{
		"SimpleMerge": {
			args: args{
				mergedData: map[string]interface{}{"a": 1},
				toAdd:      map[string]interface{}{"b": 2},
			},
			want: map[string]interface{}{"a": 1, "b": 2},
		},
		"MergedDataEmpty": {
			args: args{
				mergedData: map[string]interface{}{},
				toAdd:      map[string]interface{}{"parent": map[string]interface{}{"a": 1, "b": 2}},
			},
			want: map[string]interface{}{"parent": map[string]interface{}{"a": 1, "b": 2}},
		},
		"toAddEmpty": {
			args: args{
				mergedData: map[string]interface{}{"parent": map[string]interface{}{"a": 1, "b": 2}},
				toAdd:      map[string]interface{}{},
			},
			want: map[string]interface{}{"parent": map[string]interface{}{"a": 1, "b": 2}},
		},
		"DeepMerge": {
			args: args{
				mergedData: map[string]interface{}{"parent": map[string]interface{}{"a": 1}},
				toAdd:      map[string]interface{}{"parent": map[string]interface{}{"a": 2}},
			},
			want: map[string]interface{}{"parent": map[string]interface{}{"a": 2}},
		},
		"OverWriteNonMap": {
			args: args{
				mergedData: map[string]interface{}{"parent": "not-a-map"},
				toAdd:      map[string]interface{}{"parent": map[string]interface{}{"a": 1}},
			},
			want: map[string]interface{}{"parent": map[string]interface{}{"a": 1}},
		},
		"KeepNonMap": {
			args: args{
				mergedData: map[string]interface{}{"parent": map[string]interface{}{"a": 1}},
				toAdd:      map[string]interface{}{"parent": "not-a-map"},
			},
			want: map[string]interface{}{"parent": "not-a-map"},
		},
		"EmptyMapToAdd": {
			args: args{
				mergedData: map[string]interface{}{"a": 1, "b": map[string]interface{}{"x": 1}},
				toAdd:      map[string]interface{}{"b": map[string]interface{}{}},
			},
			want: map[string]interface{}{"a": 1, "b": map[string]interface{}{"x": 1}},
		},
		"PartialOverwrite": {
			args: args{
				mergedData: map[string]interface{}{"a": 1, "b": 2},
				toAdd:      map[string]interface{}{"a": 3},
			},
			want: map[string]interface{}{"a": 3, "b": 2},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			addMap(tc.args.mergedData, tc.args.toAdd)
			if diff := cmpDiffMaps(tc.want, tc.args.mergedData); diff != "" {
				t.Errorf("result mismatch: %s", diff)
			}
		})
	}
}
