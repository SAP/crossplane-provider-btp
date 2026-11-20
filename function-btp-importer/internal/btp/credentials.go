// Package btp provides HTTP clients for the SAP BTP Service Manager,
// Accounts Service, and Provisioning Service APIs.
package btp

import (
	"encoding/base64"
	"encoding/json"

	"github.com/crossplane/function-sdk-go/errors"
)

// Sentinel errors for missing required CIS credential fields.
// Callers can use errors.Is to distinguish which field failed validation.
var (
	ErrMissingClientID           = errors.New("CIS credentials missing required field: uaa.clientid")
	ErrMissingClientSecret       = errors.New("CIS credentials missing required field: uaa.clientsecret")
	ErrMissingUAAURL             = errors.New("CIS credentials missing required field: uaa.url")
	ErrMissingAccountsServiceURL = errors.New("CIS credentials missing required field: endpoints.accounts_service_url")
	ErrMissingSubaccountID       = errors.New("CIS credentials missing required field: uaa.subaccountid")
)

// CISCredentials holds the fields extracted from the CIS provider secret's
// base64-encoded JSON credentials blob.
type CISCredentials struct {
	UAA struct {
		ClientID     string `json:"clientid"`
		ClientSecret string `json:"clientsecret"`
		URL          string `json:"url"`
		SubaccountID string `json:"subaccountid"`
	} `json:"uaa"`
	Endpoints struct {
		AccountsServiceURL     string `json:"accounts_service_url"`     //nolint:tagliatelle // BTP API uses snake_case
		ProvisioningServiceURL string `json:"provisioning_service_url"` //nolint:tagliatelle // BTP API uses snake_case
	} `json:"endpoints"`
}

// ParseCISCredentials base64-decodes and JSON-unmarshals the credentials string
// from the CIS secret's data field, then validates all required fields.
// provisioning_service_url is optional — all other fields are required.
func ParseCISCredentials(encoded string) (*CISCredentials, error) {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errors.Wrap(err, "cannot base64-decode CIS credentials")
	}

	var creds CISCredentials
	if err := json.Unmarshal(decoded, &creds); err != nil {
		return nil, errors.Wrap(err, "cannot parse CIS credentials JSON")
	}

	if creds.UAA.ClientID == "" {
		return nil, ErrMissingClientID
	}
	if creds.UAA.ClientSecret == "" {
		return nil, ErrMissingClientSecret
	}
	if creds.UAA.URL == "" {
		return nil, ErrMissingUAAURL
	}
	if creds.Endpoints.AccountsServiceURL == "" {
		return nil, ErrMissingAccountsServiceURL
	}
	if creds.UAA.SubaccountID == "" {
		return nil, ErrMissingSubaccountID
	}

	return &creds, nil
}
