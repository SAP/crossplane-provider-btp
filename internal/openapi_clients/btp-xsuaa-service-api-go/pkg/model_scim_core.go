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

// checks if the ScimCore type satisfies the MappedNullable interface at compile time
var _ MappedNullable = &ScimCore{}

// ScimCore struct for ScimCore
type ScimCore struct {
	Id *string `json:"id,omitempty"`
	ExternalId *string `json:"externalId,omitempty"`
	Meta *ScimMeta `json:"meta,omitempty"`
	Schemas []string `json:"schemas,omitempty"`
}

// NewScimCore instantiates a new ScimCore object
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewScimCore() *ScimCore {
	this := ScimCore{}
	return &this
}

// NewScimCoreWithDefaults instantiates a new ScimCore object
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewScimCoreWithDefaults() *ScimCore {
	this := ScimCore{}
	return &this
}

// GetId returns the Id field value if set, zero value otherwise.
func (o *ScimCore) GetId() string {
	if o == nil || IsNil(o.Id) {
		var ret string
		return ret
	}
	return *o.Id
}

// GetIdOk returns a tuple with the Id field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *ScimCore) GetIdOk() (*string, bool) {
	if o == nil || IsNil(o.Id) {
		return nil, false
	}
	return o.Id, true
}

// HasId returns a boolean if a field has been set.
func (o *ScimCore) HasId() bool {
	if o != nil && !IsNil(o.Id) {
		return true
	}

	return false
}

// SetId gets a reference to the given string and assigns it to the Id field.
func (o *ScimCore) SetId(v string) {
	o.Id = &v
}

// GetExternalId returns the ExternalId field value if set, zero value otherwise.
func (o *ScimCore) GetExternalId() string {
	if o == nil || IsNil(o.ExternalId) {
		var ret string
		return ret
	}
	return *o.ExternalId
}

// GetExternalIdOk returns a tuple with the ExternalId field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *ScimCore) GetExternalIdOk() (*string, bool) {
	if o == nil || IsNil(o.ExternalId) {
		return nil, false
	}
	return o.ExternalId, true
}

// HasExternalId returns a boolean if a field has been set.
func (o *ScimCore) HasExternalId() bool {
	if o != nil && !IsNil(o.ExternalId) {
		return true
	}

	return false
}

// SetExternalId gets a reference to the given string and assigns it to the ExternalId field.
func (o *ScimCore) SetExternalId(v string) {
	o.ExternalId = &v
}

// GetMeta returns the Meta field value if set, zero value otherwise.
func (o *ScimCore) GetMeta() ScimMeta {
	if o == nil || IsNil(o.Meta) {
		var ret ScimMeta
		return ret
	}
	return *o.Meta
}

// GetMetaOk returns a tuple with the Meta field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *ScimCore) GetMetaOk() (*ScimMeta, bool) {
	if o == nil || IsNil(o.Meta) {
		return nil, false
	}
	return o.Meta, true
}

// HasMeta returns a boolean if a field has been set.
func (o *ScimCore) HasMeta() bool {
	if o != nil && !IsNil(o.Meta) {
		return true
	}

	return false
}

// SetMeta gets a reference to the given ScimMeta and assigns it to the Meta field.
func (o *ScimCore) SetMeta(v ScimMeta) {
	o.Meta = &v
}

// GetSchemas returns the Schemas field value if set, zero value otherwise.
func (o *ScimCore) GetSchemas() []string {
	if o == nil || IsNil(o.Schemas) {
		var ret []string
		return ret
	}
	return o.Schemas
}

// GetSchemasOk returns a tuple with the Schemas field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *ScimCore) GetSchemasOk() ([]string, bool) {
	if o == nil || IsNil(o.Schemas) {
		return nil, false
	}
	return o.Schemas, true
}

// HasSchemas returns a boolean if a field has been set.
func (o *ScimCore) HasSchemas() bool {
	if o != nil && !IsNil(o.Schemas) {
		return true
	}

	return false
}

// SetSchemas gets a reference to the given []string and assigns it to the Schemas field.
func (o *ScimCore) SetSchemas(v []string) {
	o.Schemas = v
}

func (o ScimCore) MarshalJSON() ([]byte, error) {
	toSerialize,err := o.ToMap()
	if err != nil {
		return []byte{}, err
	}
	return json.Marshal(toSerialize)
}

func (o ScimCore) ToMap() (map[string]interface{}, error) {
	toSerialize := map[string]interface{}{}
	if !IsNil(o.Id) {
		toSerialize["id"] = o.Id
	}
	if !IsNil(o.ExternalId) {
		toSerialize["externalId"] = o.ExternalId
	}
	if !IsNil(o.Meta) {
		toSerialize["meta"] = o.Meta
	}
	if !IsNil(o.Schemas) {
		toSerialize["schemas"] = o.Schemas
	}
	return toSerialize, nil
}

type NullableScimCore struct {
	value *ScimCore
	isSet bool
}

func (v NullableScimCore) Get() *ScimCore {
	return v.value
}

func (v *NullableScimCore) Set(val *ScimCore) {
	v.value = val
	v.isSet = true
}

func (v NullableScimCore) IsSet() bool {
	return v.isSet
}

func (v *NullableScimCore) Unset() {
	v.value = nil
	v.isSet = false
}

func NewNullableScimCore(val *ScimCore) *NullableScimCore {
	return &NullableScimCore{value: val, isSet: true}
}

func (v NullableScimCore) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.value)
}

func (v *NullableScimCore) UnmarshalJSON(src []byte) error {
	v.isSet = true
	return json.Unmarshal(src, &v.value)
}


