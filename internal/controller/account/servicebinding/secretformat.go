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
	errBundleCredentials       = "cannot bundle credentials into secret key"
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
	secretKey *string,
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

	var credentialProps []secretMetadataProperty
	if secretKey != nil {
		credentialProps = []secretMetadataProperty{
			{Name: *secretKey, Format: "json", Container: true},
		}
	} else {
		var credKeys []string
		for k := range credentialData {
			if !metadataKeys[k] {
				credKeys = append(credKeys, k)
			}
		}
		sort.Strings(credKeys)

		credentialProps = make([]secretMetadataProperty, 0, len(credKeys))
		for _, k := range credKeys {
			credentialProps = append(credentialProps, secretMetadataProperty{
				Name:   k,
				Format: detectFormat(credentialData[k]),
			})
		}
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

func bundleCredentials(secretKey string, details map[string][]byte) (map[string][]byte, error) {
	credJSON, err := assembleCredentialJSON(details)
	if err != nil {
		return nil, errors.Wrap(err, errBundleCredentials)
	}
	return map[string][]byte{
		secretKey: credJSON,
	}, nil
}

func assembleCredentialJSON(details map[string][]byte) ([]byte, error) {
	if len(details) == 0 {
		return []byte("{}"), nil
	}
	// If there's a single key whose value is a valid JSON object, use it directly.
	if len(details) == 1 {
		for _, v := range details {
			trimmed := bytes.TrimSpace(v)
			if len(trimmed) > 0 && trimmed[0] == '{' && json.Valid(trimmed) {
				return trimmed, nil
			}
		}
	}
	// Otherwise, build a JSON object from all key-value pairs.
	obj := make(map[string]json.RawMessage, len(details))
	for k, v := range details {
		trimmed := bytes.TrimSpace(v)
		if len(trimmed) > 0 && json.Valid(trimmed) {
			obj[k] = json.RawMessage(trimmed)
		} else {
			quoted, err := json.Marshal(string(v))
			if err != nil {
				return nil, err
			}
			obj[k] = json.RawMessage(quoted)
		}
	}
	return json.Marshal(obj)
}

func processConnectionDetails(cr *v1alpha1.ServiceBinding, details map[string][]byte) (map[string][]byte, error) {
	if cr.Spec.SecretKey != nil {
		return bundleCredentials(*cr.Spec.SecretKey, details)
	}
	return flattenSecretData(details)
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
		cr.Spec.SecretKey,
	)
}
