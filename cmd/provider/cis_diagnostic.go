package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/sap/crossplane-provider-btp/btp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Opt-in protocol probe: only token exchanges and non-mutating native API reads.
// Tokens and credential values stay in memory and are never logged.
func runCISDiagnostic(kube client.Reader, logger logging.Logger) {
	endpoint := os.Getenv("CIS_DIAGNOSTIC_IAS")
	if endpoint == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	raw, err := os.ReadFile(os.Getenv("CIS_DIAGNOSTIC_TOKEN_FILE"))
	if err != nil {
		logger.Info("CIS diagnostic failed", "stage", "read-assertion")
		return
	}
	jwt := strings.TrimSpace(string(raw))
	logTokenClaims(logger, "projected", jwt)
	params := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {jwt}, "client_id": {os.Getenv("CIS_DIAGNOSTIC_CLIENT_ID")}, "client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"}, "client_assertion": {jwt}, "scope": {"openid email profile"}, "token_format": {"jwt"}}
	if resource := os.Getenv("CIS_DIAGNOSTIC_RESOURCE"); resource != "" {
		params.Set("resource", resource)
	}
	clientParams := url.Values{"grant_type": {"client_credentials"}, "client_id": {os.Getenv("CIS_DIAGNOSTIC_CLIENT_ID")}, "client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"}, "client_assertion": {jwt}, "token_format": {"jwt"}}
	probeToken(ctx, endpoint+"/oauth2/token", clientParams, logger, "ias-client-auth")
	tokens, ok := probeToken(ctx, endpoint+"/oauth2/token", params, logger, "ias-exchange")
	if !ok {
		params.Set("grant_type", "urn:ietf:params:oauth:grant-type:token-exchange")
		params.Del("assertion")
		params.Set("subject_token", jwt)
		params.Set("subject_token_type", "urn:ietf:params:oauth:token-type:jwt")
		tokens, ok = probeToken(ctx, endpoint+"/oauth2/token", params, logger, "ias-token-exchange")
		if !ok {
			return
		}
	}
	secret := &corev1.Secret{}
	if err = kube.Get(ctx, types.NamespacedName{Namespace: os.Getenv("CIS_DIAGNOSTIC_SECRET_NAMESPACE"), Name: os.Getenv("CIS_DIAGNOSTIC_SECRET")}, secret); err != nil {
		logger.Info("CIS diagnostic failed", "stage", "read-binding")
		return
	}
	var binding btp.CISCredential
	if json.Unmarshal(secret.Data[os.Getenv("CIS_DIAGNOSTIC_SECRET_KEY")], &binding) != nil {
		logger.Info("CIS diagnostic failed", "stage", "decode-binding")
		return
	}
	for _, kind := range []string{"access_token", "id_token"} {
		assertion, _ := tokens[kind].(string)
		if len(strings.Split(assertion, ".")) != 3 {
			continue
		}
		logTokenClaims(logger, "ias-"+kind, assertion)
		u := binding.Uaa
		p := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {assertion}, "client_id": {u.Clientid}, "client_secret": {u.Clientsecret}}
		native, ok := probeToken(ctx, u.Url+"/oauth/token", p, logger, "xsuaa-"+kind)
		if !ok {
			continue
		}
		token, _ := native["access_token"].(string)
		logTokenClaims(logger, "xsuaa-"+kind, token)
		path := os.Getenv("CIS_DIAGNOSTIC_API_PATH")
		base := binding.Endpoints.AccountsServiceUrl
		if strings.HasPrefix(path, "/provisioning/") {
			base = binding.Endpoints.ProvisioningServiceUrl
		}
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, e := (&http.Client{Timeout: 30 * time.Second}).Do(req)
		if e != nil {
			logger.Info("CIS diagnostic failed", "stage", "native-read")
			continue
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		logger.Info("CIS diagnostic native read", "tokenKind", kind, "status", resp.StatusCode)
	}
	logger.Info("CIS diagnostic complete")
}

func probeToken(ctx context.Context, endpoint string, params url.Values, logger logging.Logger, stage string) (map[string]interface{}, bool) {
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(params.Encode()))
	if e != nil {
		logger.Info("CIS diagnostic failed", "stage", stage)
		return nil, false
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, e := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if e != nil {
		logger.Info("CIS diagnostic failed", "stage", stage, "errorClass", "transport")
		return nil, false
	}
	defer resp.Body.Close()
	var result map[string]interface{}
	if json.NewDecoder(io.LimitReader(resp.Body, 128*1024)).Decode(&result) != nil {
		logger.Info("CIS diagnostic token result", "stage", stage, "status", resp.StatusCode, "errorClass", "non-json")
		return nil, false
	}
	// Descriptions can contain credentials/assertions: retain only OAuth error code.
	code, _ := result["error"].(string)
	if len(code) > 80 || strings.ContainsAny(code, " .") {
		code = "redacted"
	}
	description, _ := result["error_description"].(string)
	classifications := []string{}
	lower := strings.ToLower(description)
	for _, word := range []string{"audience", "issuer", "subject", "signature", "trust", "client", "assertion", "expired", "grant", "public", "authentication", "unknown", "invalid", "not found", "jwt", "application", "token", "expected", "provider", "signed", "different", "not", "allowed", "matching", "configuration", "external", "identity", "oidc", "configured"} {
		if strings.Contains(lower, word) {
			classifications = append(classifications, word)
		}
	}
	// Preserve diagnostic wording after removing every submitted credential and
	// long encoded string. Never emit token response fields.
	for key, values := range params {
		if key == "assertion" || key == "client_assertion" || key == "client_secret" || key == "subject_token" {
			for _, value := range values {
				if value != "" {
					description = strings.ReplaceAll(description, value, "[credential withheld]")
				}
			}
		}
	}
	description = regexp.MustCompile(`[A-Za-z0-9_-]{40,}(?:\.[A-Za-z0-9_-]+)*`).ReplaceAllString(description, "[encoded value withheld]")
	if len(description) > 600 {
		description = "[long description withheld]"
	}
	logger.Info("CIS diagnostic token result", "stage", stage, "status", resp.StatusCode, "oauthError", code, "errorTerms", classifications, "safeDescription", description)
	return result, resp.StatusCode == 200
}

func logTokenClaims(logger logging.Logger, stage, jwt string) {
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		return
	}
	body, e := base64.RawURLEncoding.DecodeString(parts[1])
	if e != nil {
		return
	}
	var claims map[string]interface{}
	if json.Unmarshal(body, &claims) != nil {
		return
	}
	safe := map[string]interface{}{}
	for _, key := range []string{"iss", "aud", "sub", "mail", "user_name", "user_uuid", "origin", "grant_type", "scope", "ias_apis", "zid", "iat", "exp"} {
		if v, ok := claims[key]; ok {
			safe[key] = v
		}
	}
	data, _ := json.Marshal(safe)
	logger.Info("CIS diagnostic token claims", "stage", stage, "claims", fmt.Sprint(string(data)))
}
