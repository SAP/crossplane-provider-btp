/*
SAP XSUAA REST API

Provides access to RoleTemplates, Roles, RoleCollection etc. using the XSUAA REST API

API version: 1.0.0
*/

// Code generated by OpenAPI Generator (https://openapi-generator.tech); DO NOT EDIT.

package openapi

import (
	"encoding/json"
)

// checks if the TokenPolicySettingsReq type satisfies the MappedNullable interface at compile time
var _ MappedNullable = &TokenPolicySettingsReq{}

// TokenPolicySettingsReq struct for TokenPolicySettingsReq
type TokenPolicySettingsReq struct {
	AccessTokenValidity *int32 `json:"accessTokenValidity,omitempty"`
	RefreshTokenValidity *int32 `json:"refreshTokenValidity,omitempty"`
	RefreshTokenUnique *bool `json:"refreshTokenUnique,omitempty"`
	ChangeMode *string `json:"changeMode,omitempty"`
	KeyId *string `json:"keyId,omitempty"`
}

// NewTokenPolicySettingsReq instantiates a new TokenPolicySettingsReq object
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewTokenPolicySettingsReq() *TokenPolicySettingsReq {
	this := TokenPolicySettingsReq{}
	return &this
}

// NewTokenPolicySettingsReqWithDefaults instantiates a new TokenPolicySettingsReq object
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewTokenPolicySettingsReqWithDefaults() *TokenPolicySettingsReq {
	this := TokenPolicySettingsReq{}
	return &this
}

// GetAccessTokenValidity returns the AccessTokenValidity field value if set, zero value otherwise.
func (o *TokenPolicySettingsReq) GetAccessTokenValidity() int32 {
	if o == nil || IsNil(o.AccessTokenValidity) {
		var ret int32
		return ret
	}
	return *o.AccessTokenValidity
}

// GetAccessTokenValidityOk returns a tuple with the AccessTokenValidity field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *TokenPolicySettingsReq) GetAccessTokenValidityOk() (*int32, bool) {
	if o == nil || IsNil(o.AccessTokenValidity) {
		return nil, false
	}
	return o.AccessTokenValidity, true
}

// HasAccessTokenValidity returns a boolean if a field has been set.
func (o *TokenPolicySettingsReq) HasAccessTokenValidity() bool {
	if o != nil && !IsNil(o.AccessTokenValidity) {
		return true
	}

	return false
}

// SetAccessTokenValidity gets a reference to the given int32 and assigns it to the AccessTokenValidity field.
func (o *TokenPolicySettingsReq) SetAccessTokenValidity(v int32) {
	o.AccessTokenValidity = &v
}

// GetRefreshTokenValidity returns the RefreshTokenValidity field value if set, zero value otherwise.
func (o *TokenPolicySettingsReq) GetRefreshTokenValidity() int32 {
	if o == nil || IsNil(o.RefreshTokenValidity) {
		var ret int32
		return ret
	}
	return *o.RefreshTokenValidity
}

// GetRefreshTokenValidityOk returns a tuple with the RefreshTokenValidity field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *TokenPolicySettingsReq) GetRefreshTokenValidityOk() (*int32, bool) {
	if o == nil || IsNil(o.RefreshTokenValidity) {
		return nil, false
	}
	return o.RefreshTokenValidity, true
}

// HasRefreshTokenValidity returns a boolean if a field has been set.
func (o *TokenPolicySettingsReq) HasRefreshTokenValidity() bool {
	if o != nil && !IsNil(o.RefreshTokenValidity) {
		return true
	}

	return false
}

// SetRefreshTokenValidity gets a reference to the given int32 and assigns it to the RefreshTokenValidity field.
func (o *TokenPolicySettingsReq) SetRefreshTokenValidity(v int32) {
	o.RefreshTokenValidity = &v
}

// GetRefreshTokenUnique returns the RefreshTokenUnique field value if set, zero value otherwise.
func (o *TokenPolicySettingsReq) GetRefreshTokenUnique() bool {
	if o == nil || IsNil(o.RefreshTokenUnique) {
		var ret bool
		return ret
	}
	return *o.RefreshTokenUnique
}

// GetRefreshTokenUniqueOk returns a tuple with the RefreshTokenUnique field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *TokenPolicySettingsReq) GetRefreshTokenUniqueOk() (*bool, bool) {
	if o == nil || IsNil(o.RefreshTokenUnique) {
		return nil, false
	}
	return o.RefreshTokenUnique, true
}

// HasRefreshTokenUnique returns a boolean if a field has been set.
func (o *TokenPolicySettingsReq) HasRefreshTokenUnique() bool {
	if o != nil && !IsNil(o.RefreshTokenUnique) {
		return true
	}

	return false
}

// SetRefreshTokenUnique gets a reference to the given bool and assigns it to the RefreshTokenUnique field.
func (o *TokenPolicySettingsReq) SetRefreshTokenUnique(v bool) {
	o.RefreshTokenUnique = &v
}

// GetChangeMode returns the ChangeMode field value if set, zero value otherwise.
func (o *TokenPolicySettingsReq) GetChangeMode() string {
	if o == nil || IsNil(o.ChangeMode) {
		var ret string
		return ret
	}
	return *o.ChangeMode
}

// GetChangeModeOk returns a tuple with the ChangeMode field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *TokenPolicySettingsReq) GetChangeModeOk() (*string, bool) {
	if o == nil || IsNil(o.ChangeMode) {
		return nil, false
	}
	return o.ChangeMode, true
}

// HasChangeMode returns a boolean if a field has been set.
func (o *TokenPolicySettingsReq) HasChangeMode() bool {
	if o != nil && !IsNil(o.ChangeMode) {
		return true
	}

	return false
}

// SetChangeMode gets a reference to the given string and assigns it to the ChangeMode field.
func (o *TokenPolicySettingsReq) SetChangeMode(v string) {
	o.ChangeMode = &v
}

// GetKeyId returns the KeyId field value if set, zero value otherwise.
func (o *TokenPolicySettingsReq) GetKeyId() string {
	if o == nil || IsNil(o.KeyId) {
		var ret string
		return ret
	}
	return *o.KeyId
}

// GetKeyIdOk returns a tuple with the KeyId field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *TokenPolicySettingsReq) GetKeyIdOk() (*string, bool) {
	if o == nil || IsNil(o.KeyId) {
		return nil, false
	}
	return o.KeyId, true
}

// HasKeyId returns a boolean if a field has been set.
func (o *TokenPolicySettingsReq) HasKeyId() bool {
	if o != nil && !IsNil(o.KeyId) {
		return true
	}

	return false
}

// SetKeyId gets a reference to the given string and assigns it to the KeyId field.
func (o *TokenPolicySettingsReq) SetKeyId(v string) {
	o.KeyId = &v
}

func (o TokenPolicySettingsReq) MarshalJSON() ([]byte, error) {
	toSerialize,err := o.ToMap()
	if err != nil {
		return []byte{}, err
	}
	return json.Marshal(toSerialize)
}

func (o TokenPolicySettingsReq) ToMap() (map[string]interface{}, error) {
	toSerialize := map[string]interface{}{}
	if !IsNil(o.AccessTokenValidity) {
		toSerialize["accessTokenValidity"] = o.AccessTokenValidity
	}
	if !IsNil(o.RefreshTokenValidity) {
		toSerialize["refreshTokenValidity"] = o.RefreshTokenValidity
	}
	if !IsNil(o.RefreshTokenUnique) {
		toSerialize["refreshTokenUnique"] = o.RefreshTokenUnique
	}
	if !IsNil(o.ChangeMode) {
		toSerialize["changeMode"] = o.ChangeMode
	}
	if !IsNil(o.KeyId) {
		toSerialize["keyId"] = o.KeyId
	}
	return toSerialize, nil
}

type NullableTokenPolicySettingsReq struct {
	value *TokenPolicySettingsReq
	isSet bool
}

func (v NullableTokenPolicySettingsReq) Get() *TokenPolicySettingsReq {
	return v.value
}

func (v *NullableTokenPolicySettingsReq) Set(val *TokenPolicySettingsReq) {
	v.value = val
	v.isSet = true
}

func (v NullableTokenPolicySettingsReq) IsSet() bool {
	return v.isSet
}

func (v *NullableTokenPolicySettingsReq) Unset() {
	v.value = nil
	v.isSet = false
}

func NewNullableTokenPolicySettingsReq(val *TokenPolicySettingsReq) *NullableTokenPolicySettingsReq {
	return &NullableTokenPolicySettingsReq{value: val, isSet: true}
}

func (v NullableTokenPolicySettingsReq) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.value)
}

func (v *NullableTokenPolicySettingsReq) UnmarshalJSON(src []byte) error {
	v.isSet = true
	return json.Unmarshal(src, &v.value)
}


