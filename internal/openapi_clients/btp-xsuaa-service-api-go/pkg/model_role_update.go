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

// checks if the RoleUpdate type satisfies the MappedNullable interface at compile time
var _ MappedNullable = &RoleUpdate{}

// RoleUpdate JSON containing the role
type RoleUpdate struct {
	Description *string `json:"description,omitempty"`
	AttributeList []RoleAttribute `json:"attributeList,omitempty"`
}

// NewRoleUpdate instantiates a new RoleUpdate object
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewRoleUpdate() *RoleUpdate {
	this := RoleUpdate{}
	return &this
}

// NewRoleUpdateWithDefaults instantiates a new RoleUpdate object
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewRoleUpdateWithDefaults() *RoleUpdate {
	this := RoleUpdate{}
	return &this
}

// GetDescription returns the Description field value if set, zero value otherwise.
func (o *RoleUpdate) GetDescription() string {
	if o == nil || IsNil(o.Description) {
		var ret string
		return ret
	}
	return *o.Description
}

// GetDescriptionOk returns a tuple with the Description field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *RoleUpdate) GetDescriptionOk() (*string, bool) {
	if o == nil || IsNil(o.Description) {
		return nil, false
	}
	return o.Description, true
}

// HasDescription returns a boolean if a field has been set.
func (o *RoleUpdate) HasDescription() bool {
	if o != nil && !IsNil(o.Description) {
		return true
	}

	return false
}

// SetDescription gets a reference to the given string and assigns it to the Description field.
func (o *RoleUpdate) SetDescription(v string) {
	o.Description = &v
}

// GetAttributeList returns the AttributeList field value if set, zero value otherwise.
func (o *RoleUpdate) GetAttributeList() []RoleAttribute {
	if o == nil || IsNil(o.AttributeList) {
		var ret []RoleAttribute
		return ret
	}
	return o.AttributeList
}

// GetAttributeListOk returns a tuple with the AttributeList field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *RoleUpdate) GetAttributeListOk() ([]RoleAttribute, bool) {
	if o == nil || IsNil(o.AttributeList) {
		return nil, false
	}
	return o.AttributeList, true
}

// HasAttributeList returns a boolean if a field has been set.
func (o *RoleUpdate) HasAttributeList() bool {
	if o != nil && !IsNil(o.AttributeList) {
		return true
	}

	return false
}

// SetAttributeList gets a reference to the given []RoleAttribute and assigns it to the AttributeList field.
func (o *RoleUpdate) SetAttributeList(v []RoleAttribute) {
	o.AttributeList = v
}

func (o RoleUpdate) MarshalJSON() ([]byte, error) {
	toSerialize,err := o.ToMap()
	if err != nil {
		return []byte{}, err
	}
	return json.Marshal(toSerialize)
}

func (o RoleUpdate) ToMap() (map[string]interface{}, error) {
	toSerialize := map[string]interface{}{}
	if !IsNil(o.Description) {
		toSerialize["description"] = o.Description
	}
	if !IsNil(o.AttributeList) {
		toSerialize["attributeList"] = o.AttributeList
	}
	return toSerialize, nil
}

type NullableRoleUpdate struct {
	value *RoleUpdate
	isSet bool
}

func (v NullableRoleUpdate) Get() *RoleUpdate {
	return v.value
}

func (v *NullableRoleUpdate) Set(val *RoleUpdate) {
	v.value = val
	v.isSet = true
}

func (v NullableRoleUpdate) IsSet() bool {
	return v.isSet
}

func (v *NullableRoleUpdate) Unset() {
	v.value = nil
	v.isSet = false
}

func NewNullableRoleUpdate(val *RoleUpdate) *NullableRoleUpdate {
	return &NullableRoleUpdate{value: val, isSet: true}
}

func (v NullableRoleUpdate) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.value)
}

func (v *NullableRoleUpdate) UnmarshalJSON(src []byte) error {
	v.isSet = true
	return json.Unmarshal(src, &v.value)
}


