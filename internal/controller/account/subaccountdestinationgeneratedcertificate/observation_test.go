package subaccountdestinationgeneratedcertificate

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"

	destclient "github.com/sap/crossplane-provider-btp/internal/openapi_clients/btp-destination-service-api-go/pkg"
)

var (
	oidC  = asn1.ObjectIdentifier{2, 5, 4, 6}
	oidO  = asn1.ObjectIdentifier{2, 5, 4, 10}
	oidOU = asn1.ObjectIdentifier{2, 5, 4, 11}
	oidL  = asn1.ObjectIdentifier{2, 5, 4, 7}
	oidCN = asn1.ObjectIdentifier{2, 5, 4, 3}
)

// issueSubject is the subject from issue #1086, in certificate (ASN.1) order.
func issueSubject(cn string) pkix.Name {
	return pkix.Name{ExtraNames: []pkix.AttributeTypeAndValue{
		{Type: oidC, Value: "DE"},
		{Type: oidO, Value: "SAP SE"},
		{Type: oidOU, Value: "SAP Cloud Platform Clients"},
		{Type: oidOU, Value: "dest-sub-id"},
		{Type: oidL, Value: "destination-configuration.cfapps.eu10.hana.ondemand.com/sub-id"},
		{Type: oidCN, Value: cn},
	}}
}

// issue returns a PEM-encoded certificate, its parsed form and its key. A nil
// parent makes it self-signed.
func issue(t *testing.T, subject pkix.Name, serial *big.Int, notAfter time.Time, parent *x509.Certificate, parentKey *ecdsa.PrivateKey) (string, *x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: serial, Subject: subject, NotBefore: notAfter.Add(-24 * time.Hour), NotAfter: notAfter}
	if parent == nil {
		tmpl.IsCA, tmpl.BasicConstraintsValid = true, true
		parent, parentKey = tmpl, key
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, &key.PublicKey, parentKey)
	if err != nil {
		t.Fatal(err)
	}
	c, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), c, key
}

func selfSigned(t *testing.T, subject pkix.Name, serial *big.Int, notAfter time.Time) (string, *x509.Certificate) {
	t.Helper()
	p, c, _ := issue(t, subject, serial, notAfter, nil, nil)
	return p, c
}

func TestFormatDN_IssueSample(t *testing.T) {
	_, c := selfSigned(t, issueSubject("my-client"), big.NewInt(1), time.Now().Add(time.Hour))
	want := "CN=my-client,L=destination-configuration.cfapps.eu10.hana.ondemand.com/sub-id,OU=dest-sub-id,OU=SAP Cloud Platform Clients,O=SAP SE,C=DE"
	if got := formatDN(c.Subject); got != want {
		t.Errorf("formatDN =\n %s\nwant\n %s", got, want)
	}
}

func TestFormatDN_EscapesSpecialCharacters(t *testing.T) {
	_, c := selfSigned(t, issueSubject("a,b+c"), big.NewInt(1), time.Now().Add(time.Hour))
	if got := formatDN(c.Subject); !strings.HasPrefix(got, `CN=a\,b\+c,`) {
		t.Errorf("formatDN = %s", got)
	}
}

// The leaf is the first block of the first entry, followed by its CA.
func TestParseLeafAndObserve(t *testing.T) {
	notAfter := time.Date(2027, 1, 8, 7, 17, 39, 0, time.UTC)
	caSubject := pkix.Name{CommonName: "SAP Cloud Platform Client CA", Organization: []string{"SAP SE"}, Country: []string{"DE"}}
	caPEM, ca, caKey := issue(t, caSubject, big.NewInt(7), notAfter.AddDate(10, 0, 0), nil, nil)
	leafPEM, leaf, _ := issue(t, issueSubject("my-client"), big.NewInt(0x0A1B), notAfter, ca, caKey)

	got, err := parseLeaf([]destclient.PublicKey{{Content: leafPEM + caPEM}, {Content: caPEM}})
	if err != nil {
		t.Fatal(err)
	}
	obs := observe(got)
	fp := sha256.Sum256(leaf.Raw)
	if obs.Subject != formatDN(leaf.Subject) || obs.Issuer != formatDN(ca.Subject) || obs.CommonName != "my-client" {
		t.Errorf("subject/issuer/CN = %q / %q / %q", obs.Subject, obs.Issuer, obs.CommonName)
	}
	if obs.SerialNumber != "0A1B" {
		t.Errorf("serial = %s, want 0A1B (leading zero kept)", obs.SerialNumber)
	}
	if !obs.NotBefore.Time.Equal(leaf.NotBefore) || !obs.NotAfter.Time.Equal(notAfter) {
		t.Errorf("validity = %s .. %s", obs.NotBefore, obs.NotAfter)
	}
	if obs.SHA256Fingerprint != hex.EncodeToString(fp[:]) {
		t.Errorf("fingerprint = %s", obs.SHA256Fingerprint)
	}
}

func TestParseLeaf_Errors(t *testing.T) {
	for name, keys := range map[string][]destclient.PublicKey{
		"empty":   nil,
		"no PEM":  {{Content: "not pem"}},
		"bad DER": {{Content: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("x")}))}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseLeaf(keys); err == nil {
				t.Fatal("want error")
			}
		})
	}
}
