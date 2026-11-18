package btpcli

import "context"

// ListSubaccountTrustConfigurations retrieves trust configurations in a subaccount.
// Note: the returned list includes trust configurations inherited from the global
// account (these carry ReadOnly=true and share the global origin key), alongside
// trust configurations created natively on the subaccount.
func (c *BtpCli) ListSubaccountTrustConfigurations(ctx context.Context, subaccountID string) ([]TrustConfiguration, error) {
	var response []TrustConfiguration
	if err := c.ExecuteJSON(ctx, &response, "list", "security/trust", "--subaccount", subaccountID); err != nil {
		return nil, err
	}
	return response, nil
}

// ListGlobalaccountTrustConfigurations retrieves trust configurations established at
// the global account level (no --subaccount scope). This is the authoritative source
// for global trust: subaccount lists only show inherited, read-only copies of these.
//
// The built-in platform trust (sap.default, which has no identity provider) is
// excluded: it is platform-wide and not an exportable resource. See IsPlatformTrust.
func (c *BtpCli) ListGlobalaccountTrustConfigurations(ctx context.Context) ([]TrustConfiguration, error) {
	// Target the global account of the active login first. Without this, if the
	// CLI's active target is a subaccount, "list security/trust" would return that
	// subaccount's trust configurations instead of the global account's, and the
	// subaccount trust set would always be empty. Omitting the SUBDOMAIN argument
	// targets the global account of the active login.
	if _, err := c.Execute(ctx, "target", "--global-account"); err != nil {
		return nil, err
	}

	var response []TrustConfiguration
	if err := c.ExecuteJSON(ctx, &response, "list", "security/trust"); err != nil {
		return nil, err
	}
	filtered := response[:0]
	for _, trust := range response {
		if IsPlatformTrust(trust) {
			continue
		}
		filtered = append(filtered, trust)
	}
	return filtered, nil
}

// IsPlatformTrust reports whether a trust configuration is the built-in platform-wide
// trust (e.g. sap.default) rather than a custom identity provider federation. The
// built-in trust has no identity provider; every custom trust sets one. It is not an
// exportable resource and is excluded from both the global and subaccount trust kinds.
func IsPlatformTrust(trust TrustConfiguration) bool {
	return trust.IdentityProvider == ""
}

// GlobalaccountTrustOrigins returns the set of origin keys that are configured at the
// global account level. Both the global and subaccount trust exporters use this to
// ensure a trust is exported exactly once at its true scope: a subaccount trust whose
// origin appears here is an inherited copy of a global trust and must be skipped.
//
// The built-in platform trust is already excluded by
// ListGlobalaccountTrustConfigurations, so it never appears in this set.
func (c *BtpCli) GlobalaccountTrustOrigins(ctx context.Context) (map[string]struct{}, error) {
	trusts, err := c.ListGlobalaccountTrustConfigurations(ctx)
	if err != nil {
		return nil, err
	}
	origins := make(map[string]struct{}, len(trusts))
	for _, trust := range trusts {
		if trust.OriginKey != "" {
			origins[trust.OriginKey] = struct{}{}
		}
	}
	return origins, nil
}

type TrustConfiguration struct {
	Name                         string `json:"name,omitempty"`
	OriginKey                    string `json:"originKey,omitempty"`
	Description                  string `json:"description,omitempty"`
	IdentityProvider             string `json:"identityProvider,omitempty"`
	Domain                       string `json:"domain,omitempty"`
	LinkTextForUserLogon         string `json:"linkTextForUserLogon,omitempty"`
	Status                       string `json:"status,omitempty"`
	AvailableForUserLogon        string `json:"availableForUserLogon,omitempty"`
	CreateShadowUsersDuringLogon string `json:"createShadowUsersDuringLogon,omitempty"`
}
