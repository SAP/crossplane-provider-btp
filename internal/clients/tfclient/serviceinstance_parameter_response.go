package tfclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type parameterReadbackKey struct{}

// parameterReadback is scoped to one Read call, not shared across resources or
// cached. The upstream facade can issue two parameter GETs for response formats.
type parameterReadback struct {
	parameters []byte
	err        error
}

func (p *parameterReadback) capture(req *http.Request, resp *http.Response, transportErr error) {
	if !isServiceInstanceParameterRead(req) {
		return
	}
	// A successful first response remains authoritative even if the upstream
	// decoder retries it and that redundant request fails.
	if p.parameters != nil {
		return
	}
	if transportErr != nil {
		p.err = fmt.Errorf("parameter retrieval transport failed")
		return
	}
	if resp == nil {
		p.err = fmt.Errorf("parameter retrieval returned no HTTP response")
		return
	}
	status := resp.StatusCode
	if backend := resp.Header.Get(headerCLIBackendStatus); backend != "" && backend != "200" {
		p.err = fmt.Errorf("parameter retrieval backend status %s", backend)
		return
	}
	if status != http.StatusOK {
		p.err = fmt.Errorf("parameter retrieval HTTP status %d", status)
		return
	}
	if resp.Body == nil {
		p.err = fmt.Errorf("parameter retrieval returned no JSON object")
		return
	}
	body, err := io.ReadAll(resp.Body)
	closeErr := resp.Body.Close()
	// Restore the response for the existing Terraform decoder. Never rewrite
	// the body or alter authentication, retry, and session handling.
	resp.Body = io.NopCloser(bytes.NewReader(body))
	if err != nil || closeErr != nil {
		p.err = fmt.Errorf("cannot read parameter response")
		return
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(body, &object) != nil || object == nil {
		p.err = fmt.Errorf("parameter retrieval returned an invalid JSON object")
		return
	}
	// CLI responses may be a plain parameter object or a data envelope.
	if data, ok := object["data"]; ok && len(object) == 1 {
		if json.Unmarshal(data, &object) != nil || object == nil {
			p.err = fmt.Errorf("parameter retrieval returned an invalid data object")
			return
		}
	}
	p.parameters, err = json.Marshal(object)
	if err != nil {
		p.err = fmt.Errorf("cannot encode parameter response")
	}
}

func isServiceInstanceParameterRead(req *http.Request) bool {
	if req.Method != http.MethodPost || !strings.HasSuffix(req.URL.Path, "/services/instance") || !req.URL.Query().Has("get") || req.GetBody == nil {
		return false
	}
	body, err := req.GetBody()
	if err != nil {
		return false
	}
	defer body.Close() //nolint:errcheck
	var command struct {
		Parameters map[string]json.RawMessage `json:"paramValues"`
	}
	if json.NewDecoder(body).Decode(&command) != nil {
		return false
	}
	value := command.Parameters["parameters"]
	return string(value) == `"true"` || string(value) == "true"
}
