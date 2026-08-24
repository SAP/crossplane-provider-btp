package destination

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// testServer creates a fake HTTP server returning the given status, body and ETag.
func testServer(t *testing.T, statusCode int, body string, etag string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if etag != "" {
			w.Header().Set("ETag", etag)
		}
		w.WriteHeader(statusCode)
		_, _ = io.WriteString(w, body)
	}))
}

func newTestClient(t *testing.T, srv *httptest.Server) *destinationClient {
	t.Helper()
	return &destinationClient{httpClient: srv.Client(), baseURL: srv.URL}
}

func TestParseCredential(t *testing.T) {
	raw := `{"clientid":"my-id","clientsecret":"my-secret","tokenurl":"https://token.example.com","uri":"https://api.example.com"}`
	cred, err := ParseCredential([]byte(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cred.ClientID != "my-id" {
		t.Errorf("ClientID = %q, want %q", cred.ClientID, "my-id")
	}
	if cred.URI != "https://api.example.com" {
		t.Errorf("URI = %q, want %q", cred.URI, "https://api.example.com")
	}
}

func TestGet_Success(t *testing.T) {
	body := `{"Name":"my-dest","Type":"HTTP","URL":"https://example.com","Authentication":"NoAuthentication"}`
	srv := testServer(t, http.StatusOK, body, "etag-abc")
	defer srv.Close()
	c := newTestClient(t, srv)

	props, etag, err := c.Get(context.Background(), "my-dest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if props["Name"] != "my-dest" {
		t.Errorf("Name = %q, want %q", props["Name"], "my-dest")
	}
	if props["URL"] != "https://example.com" {
		t.Errorf("URL = %q, want %q", props["URL"], "https://example.com")
	}
	if etag != "etag-abc" {
		t.Errorf("etag = %q, want %q", etag, "etag-abc")
	}
}

func TestGet_NotFound(t *testing.T) {
	srv := testServer(t, http.StatusNotFound, `{"message":"not found"}`, "")
	defer srv.Close()
	c := newTestClient(t, srv)

	_, _, err := c.Get(context.Background(), "missing")
	if !IsNotFound(err) {
		t.Fatalf("IsNotFound = false, want true; err = %v", err)
	}
}

func TestGet_ServerError(t *testing.T) {
	srv := testServer(t, http.StatusInternalServerError, `{"message":"boom"}`, "")
	defer srv.Close()
	c := newTestClient(t, srv)

	_, _, err := c.Get(context.Background(), "dest")
	if err == nil {
		t.Fatal("expected error on 500, got nil")
	}
	if IsNotFound(err) || IsConflict(err) {
		t.Errorf("unexpected error type: %v", err)
	}
}

func TestCreate_Success(t *testing.T) {
	srv := testServer(t, http.StatusCreated, "", "")
	defer srv.Close()
	c := newTestClient(t, srv)

	err := c.Create(context.Background(), map[string]any{"Name": "my-dest", "Type": "HTTP"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreate_Conflict(t *testing.T) {
	srv := testServer(t, http.StatusConflict, `{"message":"already exists"}`, "")
	defer srv.Close()
	c := newTestClient(t, srv)

	err := c.Create(context.Background(), map[string]any{"Name": "my-dest", "Type": "HTTP"})
	if !IsConflict(err) {
		t.Fatalf("IsConflict = false, want true; err = %v", err)
	}
}

func TestUpdate_Success(t *testing.T) {
	srv := testServer(t, http.StatusOK, "", "")
	defer srv.Close()
	c := newTestClient(t, srv)

	err := c.Update(context.Background(), map[string]any{"Name": "my-dest", "Type": "HTTP"}, "etag-abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdate_PassesETag(t *testing.T) {
	var gotETag string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotETag = r.Header.Get("If-Match")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c := newTestClient(t, srv)

	_ = c.Update(context.Background(), map[string]any{"Name": "d", "Type": "HTTP"}, "my-etag")
	if gotETag != "my-etag" {
		t.Errorf("If-Match header = %q, want %q", gotETag, "my-etag")
	}
}

func TestDelete_Success(t *testing.T) {
	srv := testServer(t, http.StatusOK, "", "")
	defer srv.Close()
	c := newTestClient(t, srv)

	err := c.Delete(context.Background(), "my-dest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDelete_NotFoundIsIgnored(t *testing.T) {
	srv := testServer(t, http.StatusNotFound, "", "")
	defer srv.Close()
	c := newTestClient(t, srv)

	err := c.Delete(context.Background(), "missing")
	if err != nil {
		t.Fatalf("expected nil on 404 delete, got: %v", err)
	}
}

// Silence unused import during TDD red phase.
var _ = json.Marshal
