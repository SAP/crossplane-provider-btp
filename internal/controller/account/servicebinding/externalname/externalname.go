package externalname

import (
	"fmt"
	"strings"
)

type EncodedExternalName struct {
	SubAccountID      string
	ServiceInstanceID string
}

func NewEncodedExternalName(subaccountID, serviceInstanceID *string) *EncodedExternalName {
	if subaccountID == nil || serviceInstanceID == nil || *subaccountID == "" || *serviceInstanceID == "" {
		return nil
	}
	return &EncodedExternalName{
		SubAccountID:      *subaccountID,
		ServiceInstanceID: *serviceInstanceID,
	}
}

func ParseEncodedExternalName(externalName string) *EncodedExternalName {
	parts := strings.SplitN(externalName, ",", 2)
	if len(parts) != 2 {
		return nil
	}
	if parts[0] == "" || parts[1] == "" {
		return nil
	}
	return &EncodedExternalName{
		SubAccountID:      parts[0],
		ServiceInstanceID: parts[1],
	}
}

func (een *EncodedExternalName) String() string {
	return fmt.Sprintf("%s,%s", een.SubAccountID, een.ServiceInstanceID)
}
