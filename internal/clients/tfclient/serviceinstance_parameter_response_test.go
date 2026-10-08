package tfclient

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestParameterResponseCapture(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		valid      bool
	}{
		{"plain", `{"enabled":true,"count":2,"nullable":null}`, true},
		{"wrapped", `{"data":{"enabled":true}}`, true},
		{"empty plain", `{}`, true},
		{"empty wrapped", `{"data":{}}`, true},
		{"null", `null`, false},
		{"array", `[]`, false},
		{"malformed", `{`, false},
		{"null data", `{"data":null}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request, err := http.NewRequest(http.MethodPost, "https://cli.example/command/v2/services/instance?get", strings.NewReader(`{"paramValues":{"parameters":"true","id":"example"}}`))
			if err != nil {
				t.Fatal(err)
			}
			response := &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(tc.body))}
			capture := &parameterReadback{}
			capture.capture(request, response, nil)
			if (capture.parameters != nil) != tc.valid {
				t.Fatalf("valid=%v error=%v", capture.parameters != nil, capture.err)
			}
			if !tc.valid && capture.err == nil {
				t.Fatal("missing retrieval error")
			}
			body, err := io.ReadAll(response.Body)
			if err != nil || string(body) != tc.body {
				t.Fatal("upstream decoder must receive unchanged body")
			}
			if tc.valid {
				var object map[string]any
				if json.Unmarshal(capture.parameters, &object) != nil || object == nil {
					t.Fatal("invalid captured object")
				}
			}
		})
	}
}

func TestParameterCaptureIsRequestScoped(t *testing.T) {
	request, err := http.NewRequest(http.MethodPost, "https://cli.example/command/v2/services/instance?get", bytes.NewBufferString(`{"paramValues":{"parameters":"true"}}`))
	if err != nil {
		t.Fatal(err)
	}
	capture := &parameterReadback{}
	capture.capture(request, nil, errors.New("transport failure"))
	if capture.parameters != nil || capture.err == nil {
		t.Fatal("transport failure must not verify parameters")
	}
	capture.capture(request, &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil)
	if capture.parameters == nil {
		t.Fatal("successful retry must verify an empty object")
	}
	capture.capture(request, nil, errors.New("redundant retry failure"))
	if string(capture.parameters) != "{}" {
		t.Fatal("redundant failure must not discard successful readback")
	}
	other := &parameterReadback{}
	request, err = http.NewRequest(http.MethodPost, "https://cli.example/command/v2/services/instance?get", strings.NewReader(`{"paramValues":{"parameters":"false"}}`))
	if err != nil {
		t.Fatal(err)
	}
	other.capture(request, nil, errors.New("metadata failure"))
	if other.err != nil || other.parameters != nil {
		t.Fatal("metadata requests must not be treated as parameter readback")
	}
}
