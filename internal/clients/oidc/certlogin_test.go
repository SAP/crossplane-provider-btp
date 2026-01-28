package oidc

import (
	"crypto/tls"
	"fmt"
	"log"
	"os"
	"path"
	"testing"
	"time"

	"github.com/pkg/errors"

	"github.com/stretchr/testify/assert"

	"github.com/sap/crossplane-provider-btp/internal/testutils"
)

func TestCertLogin_IsExpired(t *testing.T) {

	type args struct {
		idToken string
	}
	const anHourInPast = time.Hour * -1
	const anHourInFuture = time.Hour * 1
	tests := []struct {
		name string
		args args
		want bool
	}{
		{name: "Valid token - no time based claims", args: args{idToken: testutils.JwtToken(testutils.Now)}, want: false},
		{name: "Valid token - issued in the past", args: args{idToken: testutils.JwtToken(testutils.Now, testutils.IssuedAt(anHourInPast))}, want: false},
		{name: "Valid token - nbf in the past", args: args{idToken: testutils.JwtToken(testutils.Now, testutils.NotBefore(anHourInPast))}, want: false},
		{name: "Valid token - expiry in future", args: args{idToken: testutils.JwtToken(testutils.Now, testutils.ExpiresAt(anHourInFuture))}, want: false},
		{name: "Invalid token - valid in future", args: args{idToken: testutils.JwtToken(testutils.Now, testutils.NotBefore(anHourInFuture))}, want: true},
		{name: "Invalid token - issued in future", args: args{idToken: testutils.JwtToken(testutils.Now, testutils.IssuedAt(anHourInFuture))}, want: true},
		{name: "Invalid token - expired an hour ago", args: args{idToken: testutils.JwtToken(testutils.Now, testutils.ExpiresAt(anHourInPast))}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cLogin := CertLogin{}
			assert.Equalf(t, tt.want, cLogin.IsExpired(tt.args.idToken), "IsExpired(%v)", tt.args.idToken)
		})
	}
}

func Test_parseCert(t *testing.T) {

	var tests []certLoginTest
	stores, err := lookupTestKeyStores()
	if err != nil {
		log.Fatal(err)
		return
	}
	for _, store := range stores {

		certificate, err := store.tlsCertificate()
		if err != nil {
			t.Fatal(err)
			return
		}
		containsCertificateChain := false
		if store.name == "openssl-with-certchain" {
			containsCertificateChain = true
		}

		tests = append(tests, certLoginTest{
			name: store.name,
			args: certLoginTestArgs{
				data:                     store.p12,
				password:                 store.p12Password,
				containsCertificateChain: containsCertificateChain,
			},
			want: certificate,
			wantErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				if err != nil {
					t.Errorf("%s", err.Error())
					return false
				}
				return true
			},
		})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseCert(tt.args.data, tt.args.password, tt.args.containsCertificateChain)
			if !tt.wantErr(t, err, fmt.Sprintf("parseCert(%v, %v)", tt.args.data, tt.args.password)) {
				return
			}
			assert.Equalf(t, tt.want.Certificate, got.Certificate, "check certificate")
			assert.Equalf(t, tt.want.PrivateKey, got.PrivateKey, "check private key")
		})
	}
}

func lookupTestKeyStores() ([]testKeyStoreData, error) {
	const keystoreDirectory = "test_keystores"

	var keystores []testKeyStoreData
	entries, err := os.ReadDir(keystoreDirectory)

	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue

		}
		if keystore, err2 := extractKeystoreData(keystoreDirectory, entry); err2 != nil {
			return nil, err2
		} else {
			keystores = append(keystores, keystore)
		}

	}
	return keystores, nil
}

func extractKeystoreData(keystoreDirectory string, entry os.DirEntry) (testKeyStoreData, error) {
	var keystore testKeyStoreData
	keystore.name = entry.Name()

	basePath := path.Join(keystoreDirectory, keystore.name)

	keystorePassPath := path.Join(basePath, "keystore.pass.txt")
	if _, err := os.Stat(keystorePassPath); errors.Is(err, os.ErrNotExist) {
		return keystore, errors.New(fmt.Sprintf("%s not exists", keystorePassPath))
	}

	keyPath := path.Join(basePath, "key.pem")
	if _, err := os.Stat(keyPath); errors.Is(err, os.ErrNotExist) {
		return keystore, errors.New(fmt.Sprintf("%s not exists", keyPath))
	}
	certPath := path.Join(basePath, "cert.pem")
	if _, err := os.Stat(certPath); errors.Is(err, os.ErrNotExist) {
		return keystore, errors.New(fmt.Sprintf("%s not exists", certPath))
	}
	keystorePath := path.Join(basePath, "keystore.p12")
	if _, err := os.Stat(keystorePath); errors.Is(err, os.ErrNotExist) {
		return keystore, errors.New(fmt.Sprintf("%s not exists", keystorePath))
	}

	keystorePassData, err := os.ReadFile(keystorePassPath)
	if err != nil {
		return keystore, err
	}

	keystore.p12Password = string(keystorePassData)

	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		return keystore, err
	}

	keystore.key = keyData
	certData, err := os.ReadFile(certPath)
	if err != nil {
		return keystore, err
	}

	keystore.cert = certData
	keystoreData, err := os.ReadFile(keystorePath)

	if err != nil {
		return keystore, err
	}
	keystore.p12 = keystoreData

	return keystore, nil
}

type testKeyStoreData struct {
	name        string
	key         []byte
	cert        []byte
	p12         []byte
	p12Password string
}

func (t *testKeyStoreData) tlsCertificate() (tls.Certificate, error) {
	return tls.X509KeyPair(t.cert, t.key)
}

type certLoginTest struct {
	name    string
	args    certLoginTestArgs
	want    tls.Certificate
	wantErr assert.ErrorAssertionFunc
}
type certLoginTestArgs struct {
	data                     []byte
	password                 string
	containsCertificateChain bool
}
