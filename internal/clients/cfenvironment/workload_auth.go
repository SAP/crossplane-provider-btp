package environments

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/oauth2"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	cf "github.com/cloudfoundry/go-cfclient/v3/client"
	"github.com/cloudfoundry/go-cfclient/v3/config"
	"github.com/sap/crossplane-provider-btp/btp"
)

var workloadCFCache = map[string]*config.Config{}
var workloadCFMu sync.Mutex

func readCFAssertion(file string) (string, string, error) {
	raw, err := os.ReadFile(file)
	if err != nil {
		return "", "", fmt.Errorf("cannot read workload assertion file")
	}
	assertion := strings.TrimSpace(string(raw))
	parts := strings.Split(assertion, ".")
	if len(parts) != 3 {
		return "", "", fmt.Errorf("workload assertion is not a JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", "", fmt.Errorf("cannot decode workload assertion claims")
	}
	var claims struct {
		Iss string      `json:"iss"`
		Sub string      `json:"sub"`
		Aud interface{} `json:"aud"`
		Exp int64       `json:"exp"`
	}
	if json.Unmarshal(payload, &claims) != nil || claims.Iss == "" || claims.Sub == "" || claims.Exp <= time.Now().Unix() {
		return "", "", fmt.Errorf("workload assertion has missing or expired identity claims")
	}
	audience, _ := json.Marshal(claims.Aud)
	return assertion, strings.Join([]string{claims.Iss, claims.Sub, string(audience)}, "\x00"), nil
}

func newWorkloadOrganizationClient(org *btp.CloudFoundryOrg, user *btp.WorkloadIdentityConfiguration) (*organizationClient, error) {
	if org == nil || org.Name == "" || org.Id == "" {
		return nil, fmt.Errorf("missing Cloud Foundry organization metadata")
	}
	assertion, principal, err := readCFAssertion(user.TokenFile)
	if err != nil {
		return nil, err
	}
	key := strings.Join([]string{org.ApiEndpoint, user.TokenFile, user.UserEmail, user.IdentityProvider, principal}, "\x00")
	workloadCFMu.Lock()
	defer workloadCFMu.Unlock()
	cfg := workloadCFCache[key]
	if cfg == nil {
		cfg, err = loginCFWorkload(org.ApiEndpoint, assertion, user.IdentityProvider)
		if err != nil {
			return nil, err
		}
		auth := cfg.HTTPAuthClient()
		auth.Transport = &workloadCFTransport{inner: auth.Transport, api: org.ApiEndpoint, file: user.TokenFile, origin: user.IdentityProvider, principal: principal, created: time.Now()}
		workloadCFCache[key] = cfg
	}
	cli, err := cf.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("cannot create workload Cloud Foundry client")
	}
	return &organizationClient{c: *cli, username: user.UserEmail, organizationName: org.Name, orgGuid: org.Id}, nil
}

func loginCFWorkload(api, assertion, origin string) (*config.Config, error) {
	cfg, err := config.New(api, config.JWTBearerAssertion(assertion), config.Origin(origin), config.HttpClient(&http.Client{Timeout: 30 * time.Second}))
	if err != nil {
		return nil, fmt.Errorf("Cloud Foundry workload assertion login failed")
	}
	var claims struct {
		Iat int64 `json:"iat"`
		Exp int64 `json:"exp"`
	}
	if parts := strings.Split(assertion, "."); len(parts) == 3 {
		if body, e := base64.RawURLEncoding.DecodeString(parts[1]); e == nil && json.Unmarshal(body, &claims) == nil {
			log.Printf("Cloud Foundry workload login succeeded assertion_iat=%d assertion_exp=%d", claims.Iat, claims.Exp)
		}
	}
	return cfg, nil
}

// Serialize the client's mutable refresh transport and rebuild it with the current
// projection when refresh fails, authentication is rejected, or the session ages out.
// Authentication failures are retried once; arbitrary API/network failures are not.
type workloadCFTransport struct {
	mu                           sync.Mutex
	inner                        http.RoundTripper
	api, file, origin, principal string
	created                      time.Time
}

func (t *workloadCFTransport) renew() error {
	assertion, principal, err := readCFAssertion(t.file)
	if err != nil {
		return err
	}
	if principal != t.principal {
		return fmt.Errorf("workload assertion principal changed; reconnect required")
	}
	cfg, err := loginCFWorkload(t.api, assertion, t.origin)
	if err != nil {
		return err
	}
	t.inner = cfg.HTTPAuthClient().Transport
	t.created = time.Now()
	return nil
}

func (t *workloadCFTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if time.Since(t.created) >= 15*time.Minute {
		if err := t.renew(); err != nil {
			return nil, err
		}
	}
	resp, err := t.inner.RoundTrip(req)
	authenticationFailure := resp != nil && resp.StatusCode == http.StatusUnauthorized
	if err != nil {
		message := strings.ToLower(err.Error())
		var retrieve *oauth2.RetrieveError
		authenticationFailure = errors.As(err, &retrieve) || strings.Contains(message, "oauth2:") || strings.Contains(message, "re-authenticating") || strings.Contains(message, "jwt bearer") || strings.Contains(message, "jwt-bearer")
	}
	if !authenticationFailure {
		return resp, err
	}
	if resp != nil && resp.Body != nil {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	if req.Body != nil && req.GetBody == nil {
		return nil, fmt.Errorf("cannot replay Cloud Foundry request after authentication failure")
	}
	if err := t.renew(); err != nil {
		return nil, err
	}
	clone := req.Clone(req.Context())
	if req.GetBody != nil {
		clone.Body, err = req.GetBody()
		if err != nil {
			return nil, err
		}
	}
	resp, err = t.inner.RoundTrip(clone)
	if err != nil {
		return nil, fmt.Errorf("Cloud Foundry request failed after workload reauthentication")
	}
	return resp, nil
}
