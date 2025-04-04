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

// checks if the UpdateServiceInstanceRequestPayload type satisfies the MappedNullable interface at compile time
var _ MappedNullable = &UpdateServiceInstanceRequestPayload{}

// UpdateServiceInstanceRequestPayload struct for UpdateServiceInstanceRequestPayload
type UpdateServiceInstanceRequestPayload struct {
	// The list of labels to update for the resource.
	Labels []Label `json:"labels,omitempty"`
	// The name of the service instance to update.
	Name *string `json:"name,omitempty"`
	// Some services support providing of additional configuration parameters during instance creation.<br>You can update these parameters.<br>For the list of supported configuration parameters, see the documentation of a particular service offering.<br>You can also use the *GET /v1/service_instances/{serviceInstanceID}/parameters* API later to view the parameters defined during this step.
	Parameters *map[string]string `json:"parameters,omitempty"`
	// The ID of the service plan for the service instance to update.
	ServicePlanId *string `json:"service_plan_id,omitempty"`
}

// NewUpdateServiceInstanceRequestPayload instantiates a new UpdateServiceInstanceRequestPayload object
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewUpdateServiceInstanceRequestPayload() *UpdateServiceInstanceRequestPayload {
	this := UpdateServiceInstanceRequestPayload{}
	return &this
}

// NewUpdateServiceInstanceRequestPayloadWithDefaults instantiates a new UpdateServiceInstanceRequestPayload object
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewUpdateServiceInstanceRequestPayloadWithDefaults() *UpdateServiceInstanceRequestPayload {
	this := UpdateServiceInstanceRequestPayload{}
	return &this
}

// GetLabels returns the Labels field value if set, zero value otherwise.
func (o *UpdateServiceInstanceRequestPayload) GetLabels() []Label {
	if o == nil || IsNil(o.Labels) {
		var ret []Label
		return ret
	}
	return o.Labels
}

// GetLabelsOk returns a tuple with the Labels field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *UpdateServiceInstanceRequestPayload) GetLabelsOk() ([]Label, bool) {
	if o == nil || IsNil(o.Labels) {
		return nil, false
	}
	return o.Labels, true
}

// HasLabels returns a boolean if a field has been set.
func (o *UpdateServiceInstanceRequestPayload) HasLabels() bool {
	if o != nil && !IsNil(o.Labels) {
		return true
	}

	return false
}

// SetLabels gets a reference to the given []Label and assigns it to the Labels field.
func (o *UpdateServiceInstanceRequestPayload) SetLabels(v []Label) {
	o.Labels = v
}

// GetName returns the Name field value if set, zero value otherwise.
func (o *UpdateServiceInstanceRequestPayload) GetName() string {
	if o == nil || IsNil(o.Name) {
		var ret string
		return ret
	}
	return *o.Name
}

// GetNameOk returns a tuple with the Name field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *UpdateServiceInstanceRequestPayload) GetNameOk() (*string, bool) {
	if o == nil || IsNil(o.Name) {
		return nil, false
	}
	return o.Name, true
}

// HasName returns a boolean if a field has been set.
func (o *UpdateServiceInstanceRequestPayload) HasName() bool {
	if o != nil && !IsNil(o.Name) {
		return true
	}

	return false
}

// SetName gets a reference to the given string and assigns it to the Name field.
func (o *UpdateServiceInstanceRequestPayload) SetName(v string) {
	o.Name = &v
}

// GetParameters returns the Parameters field value if set, zero value otherwise.
func (o *UpdateServiceInstanceRequestPayload) GetParameters() map[string]string {
	if o == nil || IsNil(o.Parameters) {
		var ret map[string]string
		return ret
	}
	return *o.Parameters
}

// GetParametersOk returns a tuple with the Parameters field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *UpdateServiceInstanceRequestPayload) GetParametersOk() (*map[string]string, bool) {
	if o == nil || IsNil(o.Parameters) {
		return nil, false
	}
	return o.Parameters, true
}

// HasParameters returns a boolean if a field has been set.
func (o *UpdateServiceInstanceRequestPayload) HasParameters() bool {
	if o != nil && !IsNil(o.Parameters) {
		return true
	}

	return false
}

// SetParameters gets a reference to the given map[string]string and assigns it to the Parameters field.
func (o *UpdateServiceInstanceRequestPayload) SetParameters(v map[string]string) {
	o.Parameters = &v
}

// GetServicePlanId returns the ServicePlanId field value if set, zero value otherwise.
func (o *UpdateServiceInstanceRequestPayload) GetServicePlanId() string {
	if o == nil || IsNil(o.ServicePlanId) {
		var ret string
		return ret
	}
	return *o.ServicePlanId
}

// GetServicePlanIdOk returns a tuple with the ServicePlanId field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *UpdateServiceInstanceRequestPayload) GetServicePlanIdOk() (*string, bool) {
	if o == nil || IsNil(o.ServicePlanId) {
		return nil, false
	}
	return o.ServicePlanId, true
}

// HasServicePlanId returns a boolean if a field has been set.
func (o *UpdateServiceInstanceRequestPayload) HasServicePlanId() bool {
	if o != nil && !IsNil(o.ServicePlanId) {
		return true
	}

	return false
}

// SetServicePlanId gets a reference to the given string and assigns it to the ServicePlanId field.
func (o *UpdateServiceInstanceRequestPayload) SetServicePlanId(v string) {
	o.ServicePlanId = &v
}

func (o UpdateServiceInstanceRequestPayload) MarshalJSON() ([]byte, error) {
	toSerialize,err := o.ToMap()
	if err != nil {
		return []byte{}, err
	}
	return json.Marshal(toSerialize)
}

func (o UpdateServiceInstanceRequestPayload) ToMap() (map[string]interface{}, error) {
	toSerialize := map[string]interface{}{}
	if !IsNil(o.Labels) {
		toSerialize["labels"] = o.Labels
	}
	if !IsNil(o.Name) {
		toSerialize["name"] = o.Name
	}
	if !IsNil(o.Parameters) {
		toSerialize["parameters"] = o.Parameters
	}
	if !IsNil(o.ServicePlanId) {
		toSerialize["service_plan_id"] = o.ServicePlanId
	}
	return toSerialize, nil
}

type NullableUpdateServiceInstanceRequestPayload struct {
	value *UpdateServiceInstanceRequestPayload
	isSet bool
}

func (v NullableUpdateServiceInstanceRequestPayload) Get() *UpdateServiceInstanceRequestPayload {
	return v.value
}

func (v *NullableUpdateServiceInstanceRequestPayload) Set(val *UpdateServiceInstanceRequestPayload) {
	v.value = val
	v.isSet = true
}

func (v NullableUpdateServiceInstanceRequestPayload) IsSet() bool {
	return v.isSet
}

func (v *NullableUpdateServiceInstanceRequestPayload) Unset() {
	v.value = nil
	v.isSet = false
}

func NewNullableUpdateServiceInstanceRequestPayload(val *UpdateServiceInstanceRequestPayload) *NullableUpdateServiceInstanceRequestPayload {
	return &NullableUpdateServiceInstanceRequestPayload{value: val, isSet: true}
}

func (v NullableUpdateServiceInstanceRequestPayload) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.value)
}

func (v *NullableUpdateServiceInstanceRequestPayload) UnmarshalJSON(src []byte) error {
	v.isSet = true
	return json.Unmarshal(src, &v.value)
}


