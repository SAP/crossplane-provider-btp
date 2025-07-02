package di

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestLoadSecretData(t *testing.T) {
	tests := []struct {
		name      string
		objects   []corev1.Secret
		secret    string
		namespace string
		wantData  map[string][]byte
		wantErr   bool
	}{
		{
			name: "secret found",
			objects: []corev1.Secret{{
				ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "default"},
				Data:       map[string][]byte{"key1": []byte("value1")},
			}},
			secret:    "my-secret",
			namespace: "default",
			wantData:  map[string][]byte{"key1": []byte("value1")},
			wantErr:   false,
		},
		{
			name:      "secret not found",
			objects:   []corev1.Secret{},
			secret:    "notfound",
			namespace: "default",
			wantData:  nil,
			wantErr:   true,
		},
		{
			name:      "empty secret name and namespace",
			objects:   []corev1.Secret{},
			secret:    "",
			namespace: "",
			wantData:  nil,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			objs := make([]client.Object, len(tt.objects))
			for i := range tt.objects {
				obj := tt.objects[i]
				objs[i] = &obj
			}
			fakeClient := fake.NewClientBuilder().WithObjects(objs...).Build()
			data, err := LoadSecretData(fakeClient, ctx, tt.secret, tt.namespace)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantData, data)
			}
		})
	}
}
