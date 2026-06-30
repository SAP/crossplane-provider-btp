package testutils

import (
	"context"
	"reflect"
	"strings"

	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	"github.com/pkg/errors"
	"github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// FakeKubeClientBuilder simply implementation of a kubeclient mock, can be configured to store and return any resource
type FakeKubeClientBuilder struct {
	objects []client.Object

	getFn test.MockGetFn

	resourceConverters map[string]ResourceConverter
}

type ResourceConverter func(srcObj client.Object, destObj client.Object)

var DefaultSecretConverter = func(srcObj client.Object, destObj client.Object) {
	srcSecret := srcObj.(*v1.Secret)
	destSecret := destObj.(*v1.Secret)
	srcSecret.DeepCopyInto(destSecret)
}

var DefaultProviderConfigConverter = func(srcObj client.Object, destObj client.Object) {
	srcPC := srcObj.(*v1alpha1.ProviderConfig)
	destPC := destObj.(*v1alpha1.ProviderConfig)
	srcPC.DeepCopyInto(destPC)
}

func NewFakeKubeClientBuilder() FakeKubeClientBuilder {
	return FakeKubeClientBuilder{}.
		RegisterResourceConverter(&v1.Secret{}, DefaultSecretConverter).
		RegisterResourceConverter(&v1alpha1.ProviderConfig{}, DefaultProviderConfigConverter)
}

// RegisterResourceConverter to be able to read resources resource converters needs to be registered for the type
// the helper already comes with default converters for secret and providerconfig
func (b FakeKubeClientBuilder) RegisterResourceConverter(resType client.Object, conv ResourceConverter) FakeKubeClientBuilder {
	if b.resourceConverters == nil {
		b.resourceConverters = map[string]ResourceConverter{}
	}
	b.resourceConverters[typeName(resType)] = conv
	return b
}

func (b FakeKubeClientBuilder) AddResource(obj client.Object) FakeKubeClientBuilder {
	b.objects = append(b.objects, obj)
	return b
}
func (b FakeKubeClientBuilder) AddResources(obj ...client.Object) FakeKubeClientBuilder {
	b.objects = append(b.objects, obj...)
	return b
}

func (b FakeKubeClientBuilder) converterforType(obj client.Object) ResourceConverter {
	for name, v := range b.resourceConverters {
		if typeName(obj) == name {
			return v
		}
	}
	return nil
}

func typeName(obj interface{}) string {
	name := reflect.TypeOf(obj).String()
	return strings.TrimPrefix(name, "*")
}

func (b FakeKubeClientBuilder) Build() test.MockClient {
	//TODO: refactor
	b.getFn = func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
		converter := b.converterforType(obj)
		if converter == nil {
			return errors.New("No converter found for type")
		}
		for _, v := range b.objects {
			if reflect.TypeOf(v) == reflect.TypeOf(obj) && v.GetName() == key.Name {
				converter(v, obj)
				return nil
			}
		}
		return errors.Errorf("No resource with name %v available", key.Name)
	}
	return test.MockClient{
		MockGet:  b.getFn,
		MockList: test.NewMockListFn(nil),
	}
}
