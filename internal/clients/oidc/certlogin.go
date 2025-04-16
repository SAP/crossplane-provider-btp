package oidc

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	jwt "github.com/golang-jwt/jwt/v4"
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
	IssuerURL       string
	ClientID        string
	UserCertificate []byte
	Password        string
	Scopes          []string
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
		UsePKCE:     true,
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
	tlsCert, certErr := parseCert(cLogin.config.UserCertificate, cLogin.config.Password)
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
	return claims.Valid() != nil
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
	p, err := pkce.New(oidcClient.SupportedPKCEMethods())
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

func parseCert(data []byte, password string) (tls.Certificate, error) {

	privKey, pubKey, _, err := pkcs12.DecodeChain(data, password)
	if err != nil {
		return tls.Certificate{}, err
	}

	// Seems to be an IAS created p12, this probably requires improvement to make it robust

	pair := tls.Certificate{
		Certificate: [][]byte{pubKey.Raw},
		PrivateKey:  privKey,
	}
	return pair, nil
}

func parseClaims(idToken string) (jwt.MapClaims, error) {
	token, _, err := new(jwt.Parser).ParseUnverified(idToken, jwt.MapClaims{})

	if err != nil {
		return nil, err
	}
	return token.Claims.(jwt.MapClaims), nil

}
