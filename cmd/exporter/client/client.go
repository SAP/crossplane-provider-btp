package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/client/btpcli"
)

type Client struct {
	// The CIS client is used to retrieve Subaccounts and Entitlements.
	// TODO: migrate to BTP CLI.
	CisClient *btp.Client

	// The BTP CLI client is used to retrieve other resources.
	BtpCli *btpcli.BtpCli

	// cache for BTP resources
	cache ObjectCache
}

func NewLoggedInClient(ctx context.Context, cisSecret []byte, userSecret []byte) (*Client, error) {
	client := Client{}
	var resultingErr error

	// Get the CIS client.
	cisClient, err := btp.ServiceClientFromSecret(cisSecret, userSecret)
	if err != nil {
		resultingErr = errors.Join(resultingErr, fmt.Errorf("failed to create BTP CLI: %w", err))
	} else {
		client.CisClient = &cisClient
	}

	// Get the BTP CLI client.
	cliParams, err := cliLoginParametersFromSecret(cisSecret, userSecret)
	if err != nil {
		resultingErr = errors.Join(resultingErr, fmt.Errorf("failed to get BTP CLI login parameters: %w", err))
	} else {
		slog.InfoContext(ctx, "BTP CLI login parameters:", "user", cliParams.UserName, "subdomain", cliParams.GlobalAccountSubdomain)
		client.BtpCli = btpcli.NewClient("")
		err = client.BtpCli.Login(ctx, cliParams)
		if err != nil {
			resultingErr = errors.Join(resultingErr, fmt.Errorf("failed to login to BTP CLI: %w", err))
		}
	}

	if resultingErr != nil {
		return nil, resultingErr
	}

	return &client, nil
}

// cliLoginParametersFromSecret extracts BTP CLI login parameters from the given CIS and user secrets,
// which are the CIS_CENTRAL_BINDING and BTP_TECHNICAL_USER environment variables, respectively, described under:
// https://github.com/SAP/crossplane-provider-btp
// TODO: migrate to environment variables used by Terraform Provider / Exporter for SAP BTP.
func cliLoginParametersFromSecret(cisSecret []byte, userSecret []byte) (*btpcli.LoginParameters, error) {
	var cisCredential btp.CISCredential
	if err := json.Unmarshal(cisSecret, &cisCredential); err != nil {
		return nil, fmt.Errorf("cannot parce CIS secret: %w", err)
	}

	var userCredential btp.UserCredential
	if err := json.Unmarshal(userSecret, &userCredential); err != nil {
		return nil, fmt.Errorf("cannot parce user secret: %w", err)
	}

	return &btpcli.LoginParameters{
		UserName:               userCredential.Username,
		Password:               userCredential.Password,
		GlobalAccountSubdomain: cisCredential.Uaa.Tenantid,
	}, nil
}
