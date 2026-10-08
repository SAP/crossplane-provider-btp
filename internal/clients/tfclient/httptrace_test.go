package tfclient

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type countedBody struct {
	io.Reader
	reads, closes int
}

func (b *countedBody) Read(p []byte) (int, error) { b.reads++; return b.Reader.Read(p) }
func (b *countedBody) Close() error               { b.closes++; return nil }

func TestHTTPDiagnosticsDoNotConsumeSuccessfulBody(t *testing.T) {
	body := &countedBody{Reader: strings.NewReader("original response")}
	transport := &cliTransport{base: rtFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: body}, nil
	})}
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "https://example.com/command?get=", nil)
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	if body.reads != 0 || body.closes != 0 {
		t.Fatal("diagnostics consumed or closed the body")
	}
	got, err := io.ReadAll(resp.Body)
	if err != nil || string(got) != "original response" {
		t.Fatal("response changed", err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if body.closes != 1 {
		t.Fatal("close not forwarded")
	}
}
