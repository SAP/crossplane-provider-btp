/*
Provisioning Service

The Provisioning service provides REST APIs that are responsible for the provisioning and deprovisioning of environment instances and tenants in the corresponding region.  Provisioning is performed after validation by the Entitlements service. Use the APIs in this service to manage and create environment instances, such as a Cloud Foundry org, in a subaccount and to retrieve the plans and quota assignments for a subaccount.  See also: * [Authorization](https://help.sap.com/viewer/65de2977205c403bbc107264b8eccf4b/latest/en-US/3670474a58c24ac2b082e76cbbd9dc19.html) * [Rate Limiting](https://help.sap.com/viewer/65de2977205c403bbc107264b8eccf4b/latest/en-US/77b217b3f57a45b987eb7fbc3305ce1e.html) * [Error Response Format](https://help.sap.com/viewer/65de2977205c403bbc107264b8eccf4b/latest/en-US/77fef2fb104b4b1795e2e6cee790e8b8.html) * [Asynchronous Jobs](https://help.sap.com/viewer/65de2977205c403bbc107264b8eccf4b/latest/en-US/0a0a6ab0ad114d72a6611c1c6b21683e.html)

API version: 1.0
*/

// Code generated by OpenAPI Generator (https://openapi-generator.tech); DO NOT EDIT.

package openapi

import (
	"encoding/json"
)

// checks if the BusinessEnvironmentInstancesResponseCollection type satisfies the MappedNullable interface at compile time
var _ MappedNullable = &BusinessEnvironmentInstancesResponseCollection{}

// BusinessEnvironmentInstancesResponseCollection struct for BusinessEnvironmentInstancesResponseCollection
type BusinessEnvironmentInstancesResponseCollection struct {
	// List of all the environment instances
	EnvironmentInstances []BusinessEnvironmentInstanceResponseObject `json:"environmentInstances,omitempty"`
}

// NewBusinessEnvironmentInstancesResponseCollection instantiates a new BusinessEnvironmentInstancesResponseCollection object
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewBusinessEnvironmentInstancesResponseCollection() *BusinessEnvironmentInstancesResponseCollection {
	this := BusinessEnvironmentInstancesResponseCollection{}
	return &this
}

// NewBusinessEnvironmentInstancesResponseCollectionWithDefaults instantiates a new BusinessEnvironmentInstancesResponseCollection object
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewBusinessEnvironmentInstancesResponseCollectionWithDefaults() *BusinessEnvironmentInstancesResponseCollection {
	this := BusinessEnvironmentInstancesResponseCollection{}
	return &this
}

// GetEnvironmentInstances returns the EnvironmentInstances field value if set, zero value otherwise.
func (o *BusinessEnvironmentInstancesResponseCollection) GetEnvironmentInstances() []BusinessEnvironmentInstanceResponseObject {
	if o == nil || IsNil(o.EnvironmentInstances) {
		var ret []BusinessEnvironmentInstanceResponseObject
		return ret
	}
	return o.EnvironmentInstances
}

// GetEnvironmentInstancesOk returns a tuple with the EnvironmentInstances field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *BusinessEnvironmentInstancesResponseCollection) GetEnvironmentInstancesOk() ([]BusinessEnvironmentInstanceResponseObject, bool) {
	if o == nil || IsNil(o.EnvironmentInstances) {
		return nil, false
	}
	return o.EnvironmentInstances, true
}

// HasEnvironmentInstances returns a boolean if a field has been set.
func (o *BusinessEnvironmentInstancesResponseCollection) HasEnvironmentInstances() bool {
	if o != nil && !IsNil(o.EnvironmentInstances) {
		return true
	}

	return false
}

// SetEnvironmentInstances gets a reference to the given []BusinessEnvironmentInstanceResponseObject and assigns it to the EnvironmentInstances field.
func (o *BusinessEnvironmentInstancesResponseCollection) SetEnvironmentInstances(v []BusinessEnvironmentInstanceResponseObject) {
	o.EnvironmentInstances = v
}

func (o BusinessEnvironmentInstancesResponseCollection) MarshalJSON() ([]byte, error) {
	toSerialize,err := o.ToMap()
	if err != nil {
		return []byte{}, err
	}
	return json.Marshal(toSerialize)
}

func (o BusinessEnvironmentInstancesResponseCollection) ToMap() (map[string]interface{}, error) {
	toSerialize := map[string]interface{}{}
	if !IsNil(o.EnvironmentInstances) {
		toSerialize["environmentInstances"] = o.EnvironmentInstances
	}
	return toSerialize, nil
}

type NullableBusinessEnvironmentInstancesResponseCollection struct {
	value *BusinessEnvironmentInstancesResponseCollection
	isSet bool
}

func (v NullableBusinessEnvironmentInstancesResponseCollection) Get() *BusinessEnvironmentInstancesResponseCollection {
	return v.value
}

func (v *NullableBusinessEnvironmentInstancesResponseCollection) Set(val *BusinessEnvironmentInstancesResponseCollection) {
	v.value = val
	v.isSet = true
}

func (v NullableBusinessEnvironmentInstancesResponseCollection) IsSet() bool {
	return v.isSet
}

func (v *NullableBusinessEnvironmentInstancesResponseCollection) Unset() {
	v.value = nil
	v.isSet = false
}

func NewNullableBusinessEnvironmentInstancesResponseCollection(val *BusinessEnvironmentInstancesResponseCollection) *NullableBusinessEnvironmentInstancesResponseCollection {
	return &NullableBusinessEnvironmentInstancesResponseCollection{value: val, isSet: true}
}

func (v NullableBusinessEnvironmentInstancesResponseCollection) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.value)
}

func (v *NullableBusinessEnvironmentInstancesResponseCollection) UnmarshalJSON(src []byte) error {
	v.isSet = true
	return json.Unmarshal(src, &v.value)
}


