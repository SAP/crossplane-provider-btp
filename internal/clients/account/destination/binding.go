package destination

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/pkg/errors"
)

// Event reasons emitted by VerifyBinding.
const (
	ReasonBindingSubaccountUnverified    event.Reason = "BindingSubaccountUnverified"
	ReasonExternalNameSubaccountMismatch event.Reason = "ExternalNameSubaccountMismatch"
)

// TokenSubaccounter reports the subaccount of a binding's access token.
type TokenSubaccounter interface {
	TokenSubaccount() (string, error)
}

var _ TokenSubaccounter = (*Session)(nil)

// subaccountFromJWT returns the ext_attr.subaccountid claim of an XSUAA access
// token, or "" if it is absent or the token isn't a decodable JWT. The
// signature isn't verified: the token was just fetched over TLS from the
// binding's token URL, and this guards against misconfiguration, not attacks.
// No fallback to zid: SAP documents that zid and the subaccount may differ.
func subaccountFromJWT(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return ""
	}
	var claims struct {
		ExtAttr struct {
			SubaccountID string `json:"subaccountid"`
		} `json:"ext_attr"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ""
	}
	return strings.TrimSpace(claims.ExtAttr.SubaccountID)
}

func sameID(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

// externalNamePrefix returns <P> of an external name <P>/<name>, or "".
func externalNamePrefix(extName string) string {
	parts := strings.SplitN(extName, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return ""
	}
	return parts[0]
}

// CheckBindingSubaccount compares the subaccount of the binding's token with
// subaccountID. Only a mismatch is an error. A token without the claim, or an
// external-name prefix that differs from the token, yields a warning.
func CheckBindingSubaccount(tokenSubaccount, subaccountID, externalName string) ([]event.Event, error) {
	if strings.TrimSpace(tokenSubaccount) == "" {
		return []event.Event{event.Warning(ReasonBindingSubaccountUnverified, errors.New(
			"cannot verify that the destination service binding belongs to spec.forProvider.subaccountId: its access token has no ext_attr.subaccountid claim"))}, nil
	}
	if s := strings.TrimSpace(subaccountID); s != "" && !sameID(tokenSubaccount, s) {
		return nil, errors.Errorf(
			"destination service binding belongs to subaccount %s, but spec.forProvider.subaccountId is %s: point destinationServiceBindingSecretRef at a binding of subaccount %s",
			tokenSubaccount, s, s)
	}
	if p := externalNamePrefix(externalName); p != "" && !sameID(p, tokenSubaccount) {
		return []event.Event{event.Warning(ReasonExternalNameSubaccountMismatch, errors.Errorf(
			"external-name prefix %s doesn't match the binding's subaccount %s, where the resource actually lives: correct the crossplane.io/external-name annotation",
			p, tokenSubaccount))}, nil
	}
	return nil, nil
}

// VerifyBinding runs CheckBindingSubaccount for mg and records its warnings.
// Deleting a CR that was never created needs no API call, so it skips the
// check and the token fetch. rec may be nil.
func VerifyBinding(s TokenSubaccounter, mg resource.Managed, subaccountID *string, rec event.Recorder) error {
	extName := meta.GetExternalName(mg)
	if meta.WasDeleted(mg) && extName == "" {
		return nil
	}
	tokenSubaccount, err := s.TokenSubaccount()
	if err != nil {
		return err
	}
	var sub string
	if subaccountID != nil {
		sub = *subaccountID
	}
	warnings, err := CheckBindingSubaccount(tokenSubaccount, sub, extName)
	if rec != nil {
		for _, w := range warnings {
			rec.Event(mg, w)
		}
	}
	return err
}
