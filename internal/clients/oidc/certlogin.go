package oidc

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/int128/kubelogin/pkg/infrastructure/clock"
	"github.com/int128/kubelogin/pkg/infrastructure/logger"
	"github.com/int128/kubelogin/pkg/oidc"
	"github.com/int128/kubelogin/pkg/oidc/client"
	"github.com/int128/kubelogin/pkg/pkce"
	"github.com/int128/kubelogin/pkg/tlsclientconfig"
	"github.com/int128/kubelogin/pkg/tlsclientconfig/loader"
	"golang.org/x/sync/errgroup"

	"software.sslmate.com/src/go-pkcs12"
)

const (
	errCouldNotExtractExpiry   = "could not extract 'exp' from tokens"
	errCouldNotExtractIssuedAt = "could not extract 'iat' from tokens"
	errDurationNegative        = "calculated rotation duration negative, is jwt's iat > exp?"
)

type CertLogin struct {
	config CertConfiguration

	oidcInterface client.Interface
	logger        logger.Interface
}

var _ LoginPerformer = &CertLogin{}

type CertConfiguration struct {
	IssuerURL                string
	ClientID                 string
	UserCertificate          []byte
	Password                 string
	Scopes                   []string
	ContainsCertificateChain bool
}

func NewCertLogin(config CertConfiguration, ctx context.Context) (*CertLogin, error) {
	//TODO: make scopes configurable from CR
	cfg, err := configureOIDCProvider(ctx, config.IssuerURL, config.ClientID, []string{"email"})
	if err != nil {
		return nil, err
	}
	return &CertLogin{config: config, oidcInterface: cfg, logger: logger.New()}, nil

}

func configureOIDCProvider(ctx context.Context, issuerURL string, clientID string, scopes []string) (client.Interface, error) {
	loaderLoader := loader.Loader{}
	f := &client.Factory{
		Loader: loaderLoader,
		Clock:  &clock.Real{},
		Logger: logger.New(),
	}
	prov := oidc.Provider{
		IssuerURL:   issuerURL,
		ClientID:    clientID,
		ExtraScopes: scopes,
		PKCEMethod:  oidc.PKCEMethodS256,
	}
	tlsc := tlsclientconfig.Config{
		CACertFilename: []string{},
		CACertData:     []string{},
		SkipTLSVerify:  false,
		Renegotiation:  0,
	}
	cfg, err := f.New(ctx, prov, tlsc)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func (cLogin *CertLogin) DoLogin(ctx context.Context) (*oidc.TokenSet, error) {
	tlsCert, certErr := parseCert(cLogin.config.UserCertificate, cLogin.config.Password, cLogin.config.ContainsCertificateChain)
	if certErr != nil {
		return nil, certErr
	}

	tokens, err := cLogin.doLoginWithCertificate(ctx, cLogin.oidcInterface, tlsCert)
	if err != nil {
		return nil, err
	}

	return tokens, nil
}

func (cLogin *CertLogin) IsExpired(idToken string) bool {
	claims, err := parseClaims(idToken)
	if err != nil {
		return false
	}
	return jwt.NewValidator(jwt.WithIssuedAt()).Validate(claims) != nil
}

func (cLogin *CertLogin) Refresh(ctx context.Context, refreshToken string) (*oidc.TokenSet, error) {
	return cLogin.oidcInterface.Refresh(ctx, refreshToken)
}

func (cLogin *CertLogin) doLoginWithCertificate(ctx context.Context, oidcClient client.Interface, certificate tls.Certificate) (*oidc.TokenSet, error) {
	cLogin.logger.V(1).Infof("starting the authentication code flow")
	state, err := oidc.NewState()
	if err != nil {
		return nil, fmt.Errorf("could not generate a state: %w", err)
	}
	nonce, err := oidc.NewNonce()
	if err != nil {
		return nil, fmt.Errorf("could not generate a nonce: %w", err)
	}
	p, err := pkce.New(oidcClient.NegotiatedPKCEMethod())
	if err != nil {
		return nil, fmt.Errorf("could not generate PKCE parameters: %w", err)
	}
	readyChan := make(chan string, 1)
	//TODO: somehow ensure concurrency isn't an issue here
	in := client.GetTokenByAuthCodeInput{
		BindAddress:            []string{"127.0.0.1:8000"},
		State:                  state,
		Nonce:                  nonce,
		PKCEParams:             p,
		AuthRequestExtraParams: map[string]string{},
	}
	//TODO: do better error handling here, not always wait for timeout, requires more detailed investigation of auth flow
	ctx, cancel := context.WithTimeout(ctx, time.Minute*20)
	defer cancel()
	var out *oidc.TokenSet
	var eg errgroup.Group
	eg.Go(func() error {
		select {
		case url, ok := <-readyChan:
			if !ok {
				return nil
			}
			err = cLogin.callAuthorizeEndpoint(certificate, url)
			if err != nil {
				return err
			}
			return nil
		case <-ctx.Done():
			return fmt.Errorf("context cancelled while waiting for the local server: %w", ctx.Err())
		}
	})
	eg.Go(func() error {
		defer close(readyChan)
		tokenSet, err := oidcClient.GetTokenByAuthCode(ctx, in, readyChan)
		if err != nil {
			return fmt.Errorf("authorization code flow error: %w", err)
		}
		out = tokenSet
		cLogin.logger.V(1).Infof("got a token set by the authorization code flow")
		return nil
	})
	if err := eg.Wait(); err != nil {
		return nil, fmt.Errorf("authentication error: %w", err)
	}
	cLogin.logger.V(1).Infof("finished the authorization code flow")
	return out, nil
}

func (cLogin *CertLogin) callAuthorizeEndpoint(cert tls.Certificate, authorizeUrl string) error {
	// do the authorize call here
	c := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				Certificates: []tls.Certificate{cert},
			},
		},
	}
	_, err := c.Get(authorizeUrl)
	return err
}

func parseCert(data []byte, password string, containsCertificateChain bool) (tls.Certificate, error) {

	// The credential may be provided either as a PKCS#12 keystore (the format
	// SAP IAS hands out) or as a PEM bundle that an operator has already
	// unlocked (e.g. `openssl pkcs12 -in cert.p12 -nodes -out cert.pem`). We
	// auto-detect PEM by the presence of a PEM block header; PKCS#12 is binary
	// DER and never contains it.
	//
	// The PEM path is preferable in FIPS deployments: it performs no PKCS#12
	// key derivation or MAC verification, so no cryptography runs outside Go's
	// standard-library (FIPS-validated) path. The private key must be
	// unencrypted; an encrypted PEM key would reintroduce a key-derivation step
	// and is rejected by tls.X509KeyPair.
	if isPEM(data) {
		return parsePEMCertificate(data)
	}

	privKey, leafCert, caCerts, err := pkcs12.DecodeChain(data, password)
	if err != nil {
		return tls.Certificate{}, err
	}

	// Start with leaf
	certChain := [][]byte{leafCert.Raw}

	if containsCertificateChain {
		for _, ca := range caCerts {
			certChain = append(certChain, ca.Raw)
		}
	}

	pair := tls.Certificate{
		Certificate: certChain,
		PrivateKey:  privKey,
	}
	return pair, nil
}

// isPEM reports whether data looks like a PEM-encoded bundle. PKCS#12 keystores
// are binary DER and never contain a PEM block header, so this is sufficient to
// distinguish the two credential formats.
func isPEM(data []byte) bool {
	return bytes.Contains(data, []byte("-----BEGIN"))
}

// parsePEMCertificate builds a tls.Certificate from a PEM bundle containing the
// certificate (optionally with its chain) and an unencrypted private key, in
// either order. tls.X509KeyPair collects every CERTIFICATE block from the first
// argument and the key block from the second, so a single combined blob can be
// passed as both. For PEM the file content is authoritative: whichever
// certificates it contains are presented, so the containsCertificateChain flag
// (a PKCS#12-only concern) does not apply.
func parsePEMCertificate(data []byte) (tls.Certificate, error) {
	cert, err := tls.X509KeyPair(data, data)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("parsing PEM certificate (the private key must be unencrypted): %w", err)
	}
	return cert, nil
}

func parseClaims(idToken string) (jwt.MapClaims, error) {
	token, _, err := new(jwt.Parser).ParseUnverified(idToken, jwt.MapClaims{})

	if err != nil {
		return nil, err
	}
	return token.Claims.(jwt.MapClaims), nil

}
