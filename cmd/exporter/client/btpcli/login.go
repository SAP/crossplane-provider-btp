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
}

const DefaultServerURL string = "https://cli.btp.cloud.sap"

// Login authenticates against the BTP CLI server
func (c *BtpCli) Login(ctx context.Context, params *LoginParameters) error {
	var err error

	if params.UserName == "" {
		err = errors.Join(err, fmt.Errorf("user name is required"))
	}
	if params.Password == "" {
		err = errors.Join(err, fmt.Errorf("password is required"))
	}
	if params.GlobalAccountSubdomain == "" {
		err = errors.Join(err, fmt.Errorf("subdomain is required"))
	}
	if err != nil {
		return fmt.Errorf("invalid login parameters: %w", err)
	}

	args := []string{
		"login",
		"--user", params.UserName,
		"--password", params.Password,
		"--subdomain", params.GlobalAccountSubdomain,
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
