/*
Service Manager

Service Manager provides REST APIs that are responsible for the creation and consumption of service instances in any connected runtime environment.   Use the Service Manager APIs to perform various operations related to your platforms, service brokers, service instances, and service bindings.  Get service plans and service offerings associated with your environment.    #### Platforms   Platforms are OSBAPI-enabled software systems on which applications and services are hosted.   With the Service Manager, you can now register your platform and enable it to consume the SAP BTP services from your native environment.   This registration results in a returned set of credentials that are needed to deploy the Service Manager agent.     #### Service Brokers   Service brokers act as brokers between the Service Manager and a platform’s marketplace to advertise catalogues of service offerings and service plans.  They also receive and process the requests from the marketplace to provision, bind, unbind, and deprovision these offerings and plans.    #### Service Instances   Service instances are instantiations of service plans that make the functionality of those service plans available for consumption.    #### Service Bindings   Service bindings provide access details to existing service instances.  The access details are part of the service bindings' ‘credentials’ property, and typically include access URLs and credentials.    #### Service Plans   Service plans represent sets of capabilities provided by a service offering.  For example, database service offerings provide different plans for different database versions or sizes, while the Service Manager plans offer different data access levels.    #### Service Offerings   Service offerings are advertisements of the services that are supported by a service broker.  For example, software that you can consume in the subaccount.  Service offerings are related to one or more service plans.

API version: 1.0
*/

// Code generated by OpenAPI Generator (https://openapi-generator.tech); DO NOT EDIT.

package openapi

import (
	"encoding/json"
)

// checks if the TransitiveResource type satisfies the MappedNullable interface at compile time
var _ MappedNullable = &TransitiveResource{}

// TransitiveResource struct for TransitiveResource
type TransitiveResource struct {
	// The minimum criteria required to use the resource in the context of the platform.
	Criteria *string `json:"criteria,omitempty"`
	// The ID of the resource.
	Id *string `json:"id,omitempty"`
	// The type of the operation associated with the resource.
	OperationType *string `json:"operation_type,omitempty"`
	// The type of the resource.
	Type *string `json:"type,omitempty"`
}

// NewTransitiveResource instantiates a new TransitiveResource object
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewTransitiveResource() *TransitiveResource {
	this := TransitiveResource{}
	return &this
}

// NewTransitiveResourceWithDefaults instantiates a new TransitiveResource object
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewTransitiveResourceWithDefaults() *TransitiveResource {
	this := TransitiveResource{}
	return &this
}

// GetCriteria returns the Criteria field value if set, zero value otherwise.
func (o *TransitiveResource) GetCriteria() string {
	if o == nil || IsNil(o.Criteria) {
		var ret string
		return ret
	}
	return *o.Criteria
}

// GetCriteriaOk returns a tuple with the Criteria field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *TransitiveResource) GetCriteriaOk() (*string, bool) {
	if o == nil || IsNil(o.Criteria) {
		return nil, false
	}
	return o.Criteria, true
}

// HasCriteria returns a boolean if a field has been set.
func (o *TransitiveResource) HasCriteria() bool {
	if o != nil && !IsNil(o.Criteria) {
		return true
	}

	return false
}

// SetCriteria gets a reference to the given string and assigns it to the Criteria field.
func (o *TransitiveResource) SetCriteria(v string) {
	o.Criteria = &v
}

// GetId returns the Id field value if set, zero value otherwise.
func (o *TransitiveResource) GetId() string {
	if o == nil || IsNil(o.Id) {
		var ret string
		return ret
	}
	return *o.Id
}

// GetIdOk returns a tuple with the Id field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *TransitiveResource) GetIdOk() (*string, bool) {
	if o == nil || IsNil(o.Id) {
		return nil, false
	}
	return o.Id, true
}

// HasId returns a boolean if a field has been set.
func (o *TransitiveResource) HasId() bool {
	if o != nil && !IsNil(o.Id) {
		return true
	}

	return false
}

// SetId gets a reference to the given string and assigns it to the Id field.
func (o *TransitiveResource) SetId(v string) {
	o.Id = &v
}

// GetOperationType returns the OperationType field value if set, zero value otherwise.
func (o *TransitiveResource) GetOperationType() string {
	if o == nil || IsNil(o.OperationType) {
		var ret string
		return ret
	}
	return *o.OperationType
}

// GetOperationTypeOk returns a tuple with the OperationType field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *TransitiveResource) GetOperationTypeOk() (*string, bool) {
	if o == nil || IsNil(o.OperationType) {
		return nil, false
	}
	return o.OperationType, true
}

// HasOperationType returns a boolean if a field has been set.
func (o *TransitiveResource) HasOperationType() bool {
	if o != nil && !IsNil(o.OperationType) {
		return true
	}

	return false
}

// SetOperationType gets a reference to the given string and assigns it to the OperationType field.
func (o *TransitiveResource) SetOperationType(v string) {
	o.OperationType = &v
}

// GetType returns the Type field value if set, zero value otherwise.
func (o *TransitiveResource) GetType() string {
	if o == nil || IsNil(o.Type) {
		var ret string
		return ret
	}
	return *o.Type
}

// GetTypeOk returns a tuple with the Type field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *TransitiveResource) GetTypeOk() (*string, bool) {
	if o == nil || IsNil(o.Type) {
		return nil, false
	}
	return o.Type, true
}

// HasType returns a boolean if a field has been set.
func (o *TransitiveResource) HasType() bool {
	if o != nil && !IsNil(o.Type) {
		return true
	}

	return false
}

// SetType gets a reference to the given string and assigns it to the Type field.
func (o *TransitiveResource) SetType(v string) {
	o.Type = &v
}

func (o TransitiveResource) MarshalJSON() ([]byte, error) {
	toSerialize,err := o.ToMap()
	if err != nil {
		return []byte{}, err
	}
	return json.Marshal(toSerialize)
}

func (o TransitiveResource) ToMap() (map[string]interface{}, error) {
	toSerialize := map[string]interface{}{}
	if !IsNil(o.Criteria) {
		toSerialize["criteria"] = o.Criteria
	}
	if !IsNil(o.Id) {
		toSerialize["id"] = o.Id
	}
	if !IsNil(o.OperationType) {
		toSerialize["operation_type"] = o.OperationType
	}
	if !IsNil(o.Type) {
		toSerialize["type"] = o.Type
	}
	return toSerialize, nil
}

type NullableTransitiveResource struct {
	value *TransitiveResource
	isSet bool
}

func (v NullableTransitiveResource) Get() *TransitiveResource {
	return v.value
}

func (v *NullableTransitiveResource) Set(val *TransitiveResource) {
	v.value = val
	v.isSet = true
}

func (v NullableTransitiveResource) IsSet() bool {
	return v.isSet
}

func (v *NullableTransitiveResource) Unset() {
	v.value = nil
	v.isSet = false
}

func NewNullableTransitiveResource(val *TransitiveResource) *NullableTransitiveResource {
	return &NullableTransitiveResource{value: val, isSet: true}
}

func (v NullableTransitiveResource) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.value)
}

func (v *NullableTransitiveResource) UnmarshalJSON(src []byte) error {
	v.isSet = true
	return json.Unmarshal(src, &v.value)
}


