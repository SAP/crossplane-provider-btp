package externalname_test

import (
	"testing"

	"github.com/sap/crossplane-provider-btp/internal/controller/account/servicebinding/externalname"
)

func TestParseEncodedExternalName(t *testing.T) {
	een := externalname.ParseEncodedExternalName("subaid,serinstid")
	if een == nil {
		t.Error("cannot parse valid encoded external name")
	} else {
		if een.ServiceInstanceID != "serinstid" {
			t.Errorf("invalid serviceInstanceID field value (%s) for parsed external name: subaid,serinstid", een.ServiceInstanceID)
		}
		if een.SubAccountID != "subaid" {
			t.Errorf("invalid subaccoundID field value (%s) for parsed external name: subaid,serinstid", een.SubAccountID)
		}
	}

	een = externalname.ParseEncodedExternalName("subaid")
	if een != nil {
		t.Errorf("invalid encoded external name (subaid) parsing succeeded")
	}
	een = externalname.ParseEncodedExternalName("")
	if een != nil {
		t.Errorf("invalid encoded external name (empty string) parsing succeeded")
	}
	een = externalname.ParseEncodedExternalName(",serinstid")
	if een != nil {
		t.Errorf("invalid encoded external name (,serinstid) parsing succeeded")
	}
	een = externalname.ParseEncodedExternalName("subaid,")
	if een != nil {
		t.Errorf("invalid encoded external name (subaid,) parsing succeeded")
	}
}
