package btp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	stdlog "log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

// Every replacement token starts with the current projection. No user password,
// static IAS secret, refresh-token dependency or change to CIS binding ownership.
type workloadCISTokenSource struct {
	mu          sync.Mutex
	credentials *Credentials
	http        *http.Client
	token       *oauth2.Token
	principal   string
	acquired    time.Time
}

func workloadCISHTTPClient(credentials *Credentials) *http.Client {
	source := &workloadCISTokenSource{credentials: credentials, http: &http.Client{Timeout: 30 * time.Second}}
	return &http.Client{Transport: &workloadCISTransport{source: source, inner: &oauth2.Transport{Source: source, Base: http.DefaultTransport}}, Timeout: 60 * time.Second}
}

func (s *workloadCISTokenSource) Token() (*oauth2.Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.token != nil && s.token.Valid() && time.Since(s.acquired) < 15*time.Minute {
		return s.token, nil
	}
	u := s.credentials.UserCredential
	raw, err := os.ReadFile(u.TokenFile)
	if err != nil {
		return nil, fmt.Errorf("cannot read native CIS workload assertion")
	}
	assertion := strings.TrimSpace(string(raw))
	claims, err := workloadJWTClaims(assertion)
	if err != nil {
		return nil, err
	}
	issuer, _ := claims["iss"].(string)
	subject, _ := claims["sub"].(string)
	expires, _ := claims["exp"].(float64)
	if issuer == "" || subject == "" || int64(expires) <= time.Now().Unix() {
		return nil, fmt.Errorf("native CIS workload identity claims missing or expired")
	}
	audience, _ := json.Marshal(claims["aud"])
	principal := strings.Join([]string{issuer, subject, string(audience)}, "\x00")
	if s.principal != "" && s.principal != principal {
		return nil, fmt.Errorf("native CIS workload principal changed; restart required")
	}
	p := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {assertion}, "client_id": {u.IASClientID}, "client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"}, "client_assertion": {assertion}, "resource": {u.IASResource}, "scope": {"openid email profile"}, "token_format": {"jwt"}}
	ias, err := s.exchange(strings.TrimRight(u.IASURL, "/")+"/oauth2/token", p, "IAS")
	if err != nil {
		return nil, err
	}
	iasClaims, err := workloadJWTClaims(ias.AccessToken)
	if err != nil {
		return nil, err
	}
	if iasClaims["mail"] != u.Email || iasClaims["sub"] == "" {
		return nil, fmt.Errorf("IAS user token does not match configured workload email")
	}
	cis := s.credentials.CISCredential
	native, err := s.exchange(strings.TrimRight(cis.Uaa.Url, "/")+"/oauth/token", url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {ias.AccessToken}, "client_id": {cis.Uaa.Clientid}, "client_secret": {cis.Uaa.Clientsecret}}, "CIS")
	if err != nil {
		return nil, err
	}
	nativeClaims, err := workloadJWTClaims(native.AccessToken)
	if err != nil {
		return nil, err
	}
	if nativeClaims["user_name"] != u.Email || nativeClaims["origin"] != u.Idp {
		return nil, fmt.Errorf("CIS user token does not match workload email and origin")
	}
	s.principal = principal
	s.token = native
	s.acquired = time.Now()
	stdlog.Printf("Native CIS workload user login succeeded assertion_iat=%.0f assertion_exp=%.0f", claims["iat"], claims["exp"])
	return native, nil
}

func workloadJWTClaims(jwt string) (map[string]interface{}, error) {
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("workload exchange did not return a JWT")
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("cannot decode workload exchange JWT")
	}
	var claims map[string]interface{}
	if json.Unmarshal(body, &claims) != nil {
		return nil, fmt.Errorf("invalid workload exchange JWT claims")
	}
	return claims, nil
}

func (s *workloadCISTokenSource) exchange(endpoint string, params url.Values, stage string) (*oauth2.Token, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, fmt.Errorf("invalid %s token endpoint", stage)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s workload token request failed", stage)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%s workload token request returned HTTP %d", stage, resp.StatusCode)
	}
	var result struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if json.NewDecoder(resp.Body).Decode(&result) != nil || result.AccessToken == "" || result.ExpiresIn <= 0 {
		return nil, fmt.Errorf("%s workload token response is malformed", stage)
	}
	return &oauth2.Token{AccessToken: result.AccessToken, TokenType: result.TokenType, Expiry: time.Now().Add(time.Duration(result.ExpiresIn) * time.Second)}, nil
}

// Invalidate on backend401. Reconciliation retries with a fresh projection;
// this transport does not replay a potentially mutating API request.
type workloadCISTransport struct {
	source *workloadCISTokenSource
	inner  http.RoundTripper
}

func (t *workloadCISTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.inner.RoundTrip(req)
	if resp != nil && resp.StatusCode == http.StatusUnauthorized {
		t.source.mu.Lock()
		t.source.token = nil
		t.source.mu.Unlock()
	}
	return resp, err
}
