package destination

import (
	"context"
	"encoding/json"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const errLoadSecret = "cannot load destination service binding secret"

// LoadFromSecret reads a Destination Service binding secret referenced by ref
// and returns a normalised JSON blob {"clientid":...,"clientsecret":...,"tokenurl":...,"uri":...}
// ready for ParseCredential. Two secret formats are accepted:
//
//   - Format A (single key): ref.Key is non-empty. The named key must hold a JSON
//     object with clientid, clientsecret, tokenurl, uri. This is the format written
//     by ServiceBinding when spec.secretKey is set (e.g. secretKey: credentials).
//   - Format B (flat keys): ref.Key is empty. The secret has individual keys
//     clientid, clientsecret, tokenurl/token_url, uri/url. This is the format
//     written by ServiceBinding when spec.secretKey is not set.
//
// The secret may also be created manually — the controller does not require it
// to originate from a ServiceBinding CR.
func LoadFromSecret(ctx context.Context, kube client.Client, ref xpv1.SecretKeySelector) ([]byte, error) {
	var secret corev1.Secret
	if err := kube.Get(ctx, types.NamespacedName{
		Namespace: ref.Namespace,
		Name:      ref.Name,
	}, &secret); err != nil {
		return nil, errors.Wrap(err, errLoadSecret)
	}

	if ref.Key != "" {
		data, ok := secret.Data[ref.Key]
		if !ok {
			return nil, errors.Errorf("%s: key %q not found in secret %s/%s", errLoadSecret, ref.Key, ref.Namespace, ref.Name)
		}
		return data, nil
	}

	return assembleCredJSON(secret.Data)
}

// assembleCredJSON builds a normalised JSON credential object from individual
// secret keys (Format B). Accepts "tokenurl" or "token_url" for the token URL
// and "uri" or "url" for the service URI; always marshals as "tokenurl" and "uri".
func assembleCredJSON(data map[string][]byte) ([]byte, error) {
	cred := make(map[string]string, 4)

	for _, k := range []string{"clientid", "clientsecret"} {
		v, ok := data[k]
		if !ok {
			return nil, errors.Errorf("%s: required key %q not found in secret", errLoadSecret, k)
		}
		cred[k] = string(v)
	}

	if v, ok := data["uri"]; ok {
		cred["uri"] = string(v)
	} else if v, ok := data["url"]; ok {
		cred["uri"] = string(v)
	} else {
		return nil, errors.Errorf("%s: required key %q not found in secret", errLoadSecret, "uri")
	}

	if v, ok := data["tokenurl"]; ok {
		cred["tokenurl"] = string(v)
	} else if v, ok := data["token_url"]; ok {
		cred["tokenurl"] = string(v)
	} else {
		return nil, errors.Errorf("%s: required key %q not found in secret", errLoadSecret, "tokenurl")
	}

	return json.Marshal(cred)
}
