package subaccountdestinationgeneratedcertificate

import (
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"strings"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	destclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-destination-service-api-go/pkg"
)

// parseLeaf returns the leaf certificate: the first PEM block of the first
// publicKeys entry.
func parseLeaf(keys []destclient.PublicKey) (*x509.Certificate, error) {
	if len(keys) == 0 {
		return nil, errors.New("publicKeys response has no entries")
	}
	block, _ := pem.Decode([]byte(keys[0].Content))
	if block == nil {
		return nil, errors.New("first publicKeys entry contains no PEM block")
	}
	return x509.ParseCertificate(block.Bytes)
}

// formatDN renders n in RFC 2253 form with one attribute per RDN, CN first.
// pkix.Name.String() would reorder attributes and merge repeated OUs into one
// multi-valued RDN.
func formatDN(n pkix.Name) string {
	seq := make(pkix.RDNSequence, 0, len(n.Names))
	for _, atv := range n.Names {
		seq = append(seq, pkix.RelativeDistinguishedNameSET{atv})
	}
	return seq.String()
}

func observe(c *x509.Certificate) *v1alpha1.GeneratedCertificateObservation {
	fp := sha256.Sum256(c.Raw)
	return &v1alpha1.GeneratedCertificateObservation{
		Subject:           formatDN(c.Subject),
		CommonName:        c.Subject.CommonName,
		Issuer:            formatDN(c.Issuer),
		SerialNumber:      strings.ToUpper(hex.EncodeToString(c.SerialNumber.Bytes())),
		NotBefore:         metav1.NewTime(c.NotBefore.UTC()),
		NotAfter:          metav1.NewTime(c.NotAfter.UTC()),
		SHA256Fingerprint: hex.EncodeToString(fp[:]),
	}
}
