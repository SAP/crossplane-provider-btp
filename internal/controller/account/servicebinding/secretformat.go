package servicebinding

import (
	"bytes"
	"context"
	"encoding/json"
	"sort"

	"github.com/pkg/errors"
	kubeclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
)

const (
	SecretFormatSAPKubernetes = "sap-kubernetes"

	errGetServiceInstance      = "cannot get service instance for secret enrichment"
	errBuildMetadataDescriptor = "cannot build .metadata descriptor"
)

type secretMetadataProperty struct {
	Name      string `json:"name"`
	Format    string `json:"format"`
	Container bool   `json:"container,omitempty"`
}

type secretMetadata struct {
	MetaDataProperties   []secretMetadataProperty `json:"metaDataProperties"`
	CredentialProperties []secretMetadataProperty `json:"credentialProperties"`
}

var metadataKeys = map[string]bool{
	"type":          true,
	"label":         true,
	"plan":          true,
	"tags":          true,
	"instance_name": true,
	"instance_guid": true,
	".metadata":     true,
}

func enrichConnectionDetails(
	credentialData map[string][]byte,
	instanceName string,
	instanceGUID string,
	offeringName string,
	planName string,
) (map[string][]byte, error) {
	if credentialData == nil {
		credentialData = make(map[string][]byte)
	}

	credentialData["type"] = []byte(offeringName)
	credentialData["label"] = []byte(offeringName)
	credentialData["plan"] = []byte(planName)
	credentialData["instance_name"] = []byte(instanceName)
	credentialData["instance_guid"] = []byte(instanceGUID)
	credentialData["tags"] = []byte("[]")

	metaDataProps := []secretMetadataProperty{
		{Name: "instance_name", Format: "text"},
		{Name: "instance_guid", Format: "text"},
		{Name: "plan", Format: "text"},
		{Name: "label", Format: "text"},
		{Name: "type", Format: "text"},
		{Name: "tags", Format: "json"},
	}

	var credKeys []string
	for k := range credentialData {
		if !metadataKeys[k] {
			credKeys = append(credKeys, k)
		}
	}
	sort.Strings(credKeys)

	credentialProps := make([]secretMetadataProperty, 0, len(credKeys))
	for _, k := range credKeys {
		credentialProps = append(credentialProps, secretMetadataProperty{
			Name:   k,
			Format: detectFormat(credentialData[k]),
		})
	}

	metadata := secretMetadata{
		MetaDataProperties:   metaDataProps,
		CredentialProperties: credentialProps,
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, errors.Wrap(err, errBuildMetadataDescriptor)
	}
	credentialData[".metadata"] = metadataJSON

	return credentialData, nil
}

func detectFormat(value []byte) string {
	trimmed := bytes.TrimSpace(value)
	if len(trimmed) == 0 {
		return "text"
	}
	switch trimmed[0] {
	case '{', '[':
		if json.Valid(trimmed) {
			return "json"
		}
	}
	return "text"
}

func fetchServiceInstance(ctx context.Context, kube kubeclient.Client, refName string) (*v1alpha1.ServiceInstance, error) {
	si := &v1alpha1.ServiceInstance{}
	err := kube.Get(ctx, kubeclient.ObjectKey{Name: refName}, si)
	if err != nil {
		return nil, errors.Wrap(err, errGetServiceInstance)
	}
	return si, nil
}

func (e *external) enrichWithSAPMetadata(
	ctx context.Context,
	cr *v1alpha1.ServiceBinding,
	connDetails map[string][]byte,
) (map[string][]byte, error) {
	var siRefName string
	if cr.Spec.ForProvider.ServiceInstanceRef != nil {
		siRefName = cr.Spec.ForProvider.ServiceInstanceRef.Name
	}
	if siRefName == "" {
		return connDetails, nil
	}

	si, err := fetchServiceInstance(ctx, e.kube, siRefName)
	if err != nil {
		return nil, err
	}

	return enrichConnectionDetails(
		connDetails,
		si.Spec.ForProvider.Name,
		si.Status.AtProvider.ID,
		si.Spec.ForProvider.OfferingName,
		si.Spec.ForProvider.PlanName,
	)
}
