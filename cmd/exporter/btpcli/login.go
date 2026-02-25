package btpcli

import (
	"context"
	"errors"
	"fmt"
)

// LoginParameters contains parameters for BTP CLI login
type LoginParameters struct {
	UserName               string
	Password               string
	GlobalAccountSubdomain string
	ServerURL              string // optional: custom BTP CLI URL
	IdentityProvider       string // optional: custom IDP
	SSO                    bool   // optional: whether to use single sign-on
}

const DefaultServerURL string = "https://cli.btp.cloud.sap"

// Login authenticates against the BTP CLI server
func (c *BtpCli) Login(ctx context.Context, params *LoginParameters) error {
	args := []string{
		"login",
		"--subdomain", params.GlobalAccountSubdomain,
	}

	if params.SSO {
		args = append(args, "--sso")
	} else {
		args = append(args, "--user", params.UserName, "--password", params.Password)

	}

	cliServerUrl := DefaultServerURL
	if params.ServerURL != "" {
		cliServerUrl = params.ServerURL
	}
	args = append(args, "--url", cliServerUrl)

	if params.IdentityProvider != "" {
		args = append(args, "--idp", params.IdentityProvider)
	}

	output, err := c.Execute(ctx, args...)
	if err != nil {
		return errors.Join(errors.New(string(output)), fmt.Errorf("login failed: %w", err))
	}

	return nil
}
