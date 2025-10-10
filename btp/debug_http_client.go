package btp

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/crossplane/crossplane-runtime/pkg/logging"

	"golang.org/x/oauth2"
)

var (
	log logging.Logger
	// the debug flag for the debug client.
	Debug bool
)

// SetLogger sets the logger for the debug client.
func SetLogger(logger logging.Logger) {
	log = logger
}

// NewBackgroundContextWithDebugPrintHTTPClient creates a new context with a HTTP client that logs the request and response in the RoundTrip.
func NewBackgroundContextWithDebugPrintHTTPClient(opts ...Option) context.Context {
	if Debug {
		return context.WithValue(context.Background(), oauth2.HTTPClient, DebugPrintHTTPClient(opts...))
	} else {
		return context.Background()
	}
}

// AddDebugPrintHTTPClientToContext adds a HTTP client that logs the request and response in the RoundTrip to the context with the oauth2.HTTPClient key.
func AddDebugPrintHTTPClientToContext(ctx context.Context, opts ...Option) context.Context {
	if Debug {
		return context.WithValue(ctx, oauth2.HTTPClient, DebugPrintHTTPClient(opts...))
	} else {
		return ctx
	}
}

type Option func(*debugHttpClient)

// DebugHttpClient wraps a http.Client to allow passing a http.Client as an option to DebugPrintHTTPClient
type debugHttpClient struct {
	client *http.Client
}

// WithHttpClient sets the http.Client to use for the debug client. For debugging, the Transport RoundTripper is wrapped with the RoundTripDebugger that calls the original RoundTripper and logs the request and response.
func WithHttpClient(client *http.Client) Option {
	return func(d *debugHttpClient) {
		d.client = client
	}
}

// DebugPrintHTTPClient returns a new http.Client that logs the request and response in the RoundTrip.
// The debug client uses the default http.Client if no client is set with the WithHttpClient option, otherwise the set client is used and the Transport RoundTripper wrapped.
func DebugPrintHTTPClient(opts ...Option) *http.Client {
	debugClient := &debugHttpClient{
		client: &http.Client{},
	}

	for _, applyOpt := range opts {
		applyOpt(debugClient)
	}

	//Set Own RoundTripper Interceptor with RoundTripper from client in case it is set

	if debugClient.client.Transport == nil {
		// If no Transport is set, we wrap the default transport in our RoundTripDebugger.
		debugClient.client.Transport = &RoundTripDebugger{base: http.DefaultTransport}
	} else {
		// otherwise, we wrap the existing Transport in our RoundTripDebugger.
		debugClient.client.Transport = &RoundTripDebugger{base: debugClient.client.Transport}
	}

	return debugClient.client
}

type RoundTripDebugger struct {
	base http.RoundTripper
}

// RoundTrip logs the request and response and calls the base RoundTripper for the any underlying RoundTrip method of the http.Client that is wrapped.
func (r *RoundTripDebugger) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := r.base.RoundTrip(req)
	// err will be returned after logging response
	log.Debug("HTTP Request", constructLogMessage(req, resp)...)
	return resp, err
}

func constructLogMessage(req *http.Request, resp *http.Response) []any {
	reqLog := constructRequestLogMessage(req)
	respLog := constructResponseLogMessage(resp)
	return append(reqLog, respLog...)
}

func constructRequestLogMessage(req *http.Request) []any {
	if req == nil {
		return []any{}
	}
	redactedHeader := redactSensitiveHeaders(req.Header)
	proto := req.Proto
	method := req.Method
	host := req.URL.Host
	path := req.URL.Path
	parameters := req.URL.Query().Encode()

	var bodyCopy io.ReadCloser
	var err error
	bodyCopy, req.Body, err = drainBody(req.Body)
	if err != nil {
		bodyCopy = io.NopCloser(strings.NewReader(fmt.Sprintf("Error reading body: %v", err)))
	}
	bodyBytes, _ := io.ReadAll(bodyCopy)
	bodyRedacted := string(redactJwtTokensFromBody(redactSensitiveBodyBasedOnKeywords(bodyBytes)))
	return []any{"type", "REQUEST", "proto", proto, "host", host, "method", method, "path", path, "parameters", parameters, "headers", fmt.Sprintf("%v", redactedHeader), "body", bodyRedacted}
}

func constructResponseLogMessage(resp *http.Response) []any {
	if resp == nil {
		return []any{}
	}
	redactedHeader := redactSensitiveHeaders(resp.Header)
	status := resp.Status
	proto := resp.Proto

	var bodyCopy io.ReadCloser
	var err error
	bodyCopy, resp.Body, err = drainBody(resp.Body)
	if err != nil {
		bodyCopy = io.NopCloser(strings.NewReader(fmt.Sprintf("Error reading body: %v", err)))
	}
	bodyBytes, _ := io.ReadAll(bodyCopy)
	bodyRedacted := string(redactJwtTokensFromBody(redactSensitiveBodyBasedOnKeywords(bodyBytes)))
	return []any{"type", "RESPONSE", "proto", proto, "status", status, "headers", fmt.Sprintf("%v", redactedHeader), "body", bodyRedacted}
}

// redactSensitiveBodyBasedOnKeywords redacts the whole body if certain keywords are present.
func redactSensitiveBodyBasedOnKeywords(body []byte) []byte {
	keywords := []string{"password", "secret", "token", "credential"}
	for _, keyword := range keywords {
		if bytes.Contains(bytes.ToLower(body), []byte(keyword)) {
			return []byte("<BODY REDACTED>")
		}
	}
	return body
}

// redactJwtTokensFromBody redacts JWT tokens from the body
// JWT tokens are 3 base64 encoded parts, seperated by ".": <base64ecoded>.<base64ecoded>.<base64ecoded>
// The whole token is replaced with "<REDACTED>".
func redactJwtTokensFromBody(body []byte) []byte {
	jwtPattern := regexp.MustCompile(`[A-Za-z0-9_-]{2,}(?:\.[A-Za-z0-9_-]{2,}){2}`)
	return jwtPattern.ReplaceAll([]byte(body), []byte("<REDACTED>"))
}

// redactSensitiveHeaders returns a redacted copy of the header.
func redactSensitiveHeaders(header http.Header) http.Header {
	filteredHeader := make(http.Header)
	for key, values := range header {
		if key == "Authorization" {
			filteredHeader[key] = []string{"<REDACTED>"}
		} else {
			filteredHeader[key] = values
		}
	}
	return filteredHeader
}

// drainBody reads all of b to memory and then returns two equivalent
// ReadClosers yielding the same bytes.
//
// It returns an error if the initial slurp of all bytes fails. It does not attempt
// to make the returned ReadClosers have identical error-matching behavior.
// Source: https://cs.opensource.google/go/go/+/refs/tags/go1.22.2:src/net/http/httputil/dump.go
func drainBody(b io.ReadCloser) (r1, r2 io.ReadCloser, err error) {
	if b == nil || b == http.NoBody {
		// No copying needed. Preserve the magic sentinel meaning of NoBody.
		return http.NoBody, http.NoBody, nil
	}
	var buf bytes.Buffer
	if _, err = buf.ReadFrom(b); err != nil {
		return nil, b, err
	}
	if err = b.Close(); err != nil {
		return nil, b, err
	}
	return io.NopCloser(&buf), io.NopCloser(bytes.NewReader(buf.Bytes())), nil
}
