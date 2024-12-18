/*
SAP XSUAA REST API

Provides access to RoleTemplates, Roles, RoleCollection etc. using the XSUAA REST API

API version: 1.0.0
*/

// Code generated by OpenAPI Generator (https://openapi-generator.tech); DO NOT EDIT.

package openapi

import (
	"encoding/json"
	"bytes"
	"fmt"
)

// checks if the RoleCollection type satisfies the MappedNullable interface at compile time
var _ MappedNullable = &RoleCollection{}

// RoleCollection Json of the role collections
type RoleCollection struct {
	Name string `json:"name"`
	Description *string `json:"description,omitempty"`
	RoleReferences []RoleReference `json:"roleReferences,omitempty"`
	UserReferences []UserReference `json:"userReferences,omitempty"`
	// Deprecated
	GroupReferences []RoleCollectionAttribute `json:"groupReferences,omitempty"`
	SamlAttributeAssignment []RoleCollectionAttribute `json:"samlAttributeAssignment,omitempty"`
	IsReadOnly *bool `json:"isReadOnly,omitempty"`
}

type _RoleCollection RoleCollection

// NewRoleCollection instantiates a new RoleCollection object
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewRoleCollection(name string) *RoleCollection {
	this := RoleCollection{}
	this.Name = name
	return &this
}

// NewRoleCollectionWithDefaults instantiates a new RoleCollection object
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewRoleCollectionWithDefaults() *RoleCollection {
	this := RoleCollection{}
	return &this
}

// GetName returns the Name field value
func (o *RoleCollection) GetName() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.Name
}

// GetNameOk returns a tuple with the Name field value
// and a boolean to check if the value has been set.
func (o *RoleCollection) GetNameOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.Name, true
}

// SetName sets field value
func (o *RoleCollection) SetName(v string) {
	o.Name = v
}

// GetDescription returns the Description field value if set, zero value otherwise.
func (o *RoleCollection) GetDescription() string {
	if o == nil || IsNil(o.Description) {
		var ret string
		return ret
	}
	return *o.Description
}

// GetDescriptionOk returns a tuple with the Description field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *RoleCollection) GetDescriptionOk() (*string, bool) {
	if o == nil || IsNil(o.Description) {
		return nil, false
	}
	return o.Description, true
}

// HasDescription returns a boolean if a field has been set.
func (o *RoleCollection) HasDescription() bool {
	if o != nil && !IsNil(o.Description) {
		return true
	}

	return false
}

// SetDescription gets a reference to the given string and assigns it to the Description field.
func (o *RoleCollection) SetDescription(v string) {
	o.Description = &v
}

// GetRoleReferences returns the RoleReferences field value if set, zero value otherwise.
func (o *RoleCollection) GetRoleReferences() []RoleReference {
	if o == nil || IsNil(o.RoleReferences) {
		var ret []RoleReference
		return ret
	}
	return o.RoleReferences
}

// GetRoleReferencesOk returns a tuple with the RoleReferences field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *RoleCollection) GetRoleReferencesOk() ([]RoleReference, bool) {
	if o == nil || IsNil(o.RoleReferences) {
		return nil, false
	}
	return o.RoleReferences, true
}

// HasRoleReferences returns a boolean if a field has been set.
func (o *RoleCollection) HasRoleReferences() bool {
	if o != nil && !IsNil(o.RoleReferences) {
		return true
	}

	return false
}

// SetRoleReferences gets a reference to the given []RoleReference and assigns it to the RoleReferences field.
func (o *RoleCollection) SetRoleReferences(v []RoleReference) {
	o.RoleReferences = v
}

// GetUserReferences returns the UserReferences field value if set, zero value otherwise.
func (o *RoleCollection) GetUserReferences() []UserReference {
	if o == nil || IsNil(o.UserReferences) {
		var ret []UserReference
		return ret
	}
	return o.UserReferences
}

// GetUserReferencesOk returns a tuple with the UserReferences field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *RoleCollection) GetUserReferencesOk() ([]UserReference, bool) {
	if o == nil || IsNil(o.UserReferences) {
		return nil, false
	}
	return o.UserReferences, true
}

// HasUserReferences returns a boolean if a field has been set.
func (o *RoleCollection) HasUserReferences() bool {
	if o != nil && !IsNil(o.UserReferences) {
		return true
	}

	return false
}

// SetUserReferences gets a reference to the given []UserReference and assigns it to the UserReferences field.
func (o *RoleCollection) SetUserReferences(v []UserReference) {
	o.UserReferences = v
}

// GetGroupReferences returns the GroupReferences field value if set, zero value otherwise.
// Deprecated
func (o *RoleCollection) GetGroupReferences() []RoleCollectionAttribute {
	if o == nil || IsNil(o.GroupReferences) {
		var ret []RoleCollectionAttribute
		return ret
	}
	return o.GroupReferences
}

// GetGroupReferencesOk returns a tuple with the GroupReferences field value if set, nil otherwise
// and a boolean to check if the value has been set.
// Deprecated
func (o *RoleCollection) GetGroupReferencesOk() ([]RoleCollectionAttribute, bool) {
	if o == nil || IsNil(o.GroupReferences) {
		return nil, false
	}
	return o.GroupReferences, true
}

// HasGroupReferences returns a boolean if a field has been set.
func (o *RoleCollection) HasGroupReferences() bool {
	if o != nil && !IsNil(o.GroupReferences) {
		return true
	}

	return false
}

// SetGroupReferences gets a reference to the given []RoleCollectionAttribute and assigns it to the GroupReferences field.
// Deprecated
func (o *RoleCollection) SetGroupReferences(v []RoleCollectionAttribute) {
	o.GroupReferences = v
}

// GetSamlAttributeAssignment returns the SamlAttributeAssignment field value if set, zero value otherwise.
func (o *RoleCollection) GetSamlAttributeAssignment() []RoleCollectionAttribute {
	if o == nil || IsNil(o.SamlAttributeAssignment) {
		var ret []RoleCollectionAttribute
		return ret
	}
	return o.SamlAttributeAssignment
}

// GetSamlAttributeAssignmentOk returns a tuple with the SamlAttributeAssignment field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *RoleCollection) GetSamlAttributeAssignmentOk() ([]RoleCollectionAttribute, bool) {
	if o == nil || IsNil(o.SamlAttributeAssignment) {
		return nil, false
	}
	return o.SamlAttributeAssignment, true
}

// HasSamlAttributeAssignment returns a boolean if a field has been set.
func (o *RoleCollection) HasSamlAttributeAssignment() bool {
	if o != nil && !IsNil(o.SamlAttributeAssignment) {
		return true
	}

	return false
}

// SetSamlAttributeAssignment gets a reference to the given []RoleCollectionAttribute and assigns it to the SamlAttributeAssignment field.
func (o *RoleCollection) SetSamlAttributeAssignment(v []RoleCollectionAttribute) {
	o.SamlAttributeAssignment = v
}

// GetIsReadOnly returns the IsReadOnly field value if set, zero value otherwise.
func (o *RoleCollection) GetIsReadOnly() bool {
	if o == nil || IsNil(o.IsReadOnly) {
		var ret bool
		return ret
	}
	return *o.IsReadOnly
}

// GetIsReadOnlyOk returns a tuple with the IsReadOnly field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *RoleCollection) GetIsReadOnlyOk() (*bool, bool) {
	if o == nil || IsNil(o.IsReadOnly) {
		return nil, false
	}
	return o.IsReadOnly, true
}

// HasIsReadOnly returns a boolean if a field has been set.
func (o *RoleCollection) HasIsReadOnly() bool {
	if o != nil && !IsNil(o.IsReadOnly) {
		return true
	}

	return false
}

// SetIsReadOnly gets a reference to the given bool and assigns it to the IsReadOnly field.
func (o *RoleCollection) SetIsReadOnly(v bool) {
	o.IsReadOnly = &v
}

func (o RoleCollection) MarshalJSON() ([]byte, error) {
	toSerialize,err := o.ToMap()
	if err != nil {
		return []byte{}, err
	}
	return json.Marshal(toSerialize)
}

func (o RoleCollection) ToMap() (map[string]interface{}, error) {
	toSerialize := map[string]interface{}{}
	toSerialize["name"] = o.Name
	if !IsNil(o.Description) {
		toSerialize["description"] = o.Description
	}
	if !IsNil(o.RoleReferences) {
		toSerialize["roleReferences"] = o.RoleReferences
	}
	if !IsNil(o.UserReferences) {
		toSerialize["userReferences"] = o.UserReferences
	}
	if !IsNil(o.GroupReferences) {
		toSerialize["groupReferences"] = o.GroupReferences
	}
	if !IsNil(o.SamlAttributeAssignment) {
		toSerialize["samlAttributeAssignment"] = o.SamlAttributeAssignment
	}
	if !IsNil(o.IsReadOnly) {
		toSerialize["isReadOnly"] = o.IsReadOnly
	}
	return toSerialize, nil
}

func (o *RoleCollection) UnmarshalJSON(data []byte) (err error) {
	// This validates that all required properties are included in the JSON object
	// by unmarshalling the object into a generic map with string keys and checking
	// that every required field exists as a key in the generic map.
	requiredProperties := []string{
		"name",
	}

	allProperties := make(map[string]interface{})

	err = json.Unmarshal(data, &allProperties)

	if err != nil {
		return err;
	}

	for _, requiredProperty := range(requiredProperties) {
		if _, exists := allProperties[requiredProperty]; !exists {
			return fmt.Errorf("no value given for required property %v", requiredProperty)
		}
	}

	varRoleCollection := _RoleCollection{}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	err = decoder.Decode(&varRoleCollection)

	if err != nil {
		return err
	}

	*o = RoleCollection(varRoleCollection)

	return err
}

type NullableRoleCollection struct {
	value *RoleCollection
	isSet bool
}

func (v NullableRoleCollection) Get() *RoleCollection {
	return v.value
}

func (v *NullableRoleCollection) Set(val *RoleCollection) {
	v.value = val
	v.isSet = true
}

func (v NullableRoleCollection) IsSet() bool {
	return v.isSet
}

func (v *NullableRoleCollection) Unset() {
	v.value = nil
	v.isSet = false
}

func NewNullableRoleCollection(val *RoleCollection) *NullableRoleCollection {
	return &NullableRoleCollection{value: val, isSet: true}
}

func (v NullableRoleCollection) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.value)
}

func (v *NullableRoleCollection) UnmarshalJSON(src []byte) error {
	v.isSet = true
	return json.Unmarshal(src, &v.value)
}


