/*
Service Manager

Service Manager provides REST APIs that are responsible for the creation and consumption of service instances in any connected runtime environment.   Use the Service Manager APIs to perform various operations related to your platforms, service brokers, service instances, and service bindings.  Get service plans and service offerings associated with your environment.    #### Platforms   Platforms are OSBAPI-enabled software systems on which applications and services are hosted.   With the Service Manager, you can now register your platform and enable it to consume the SAP BTP services from your native environment.   This registration results in a returned set of credentials that are needed to deploy the Service Manager agent.     #### Service Brokers   Service brokers act as brokers between the Service Manager and a platform’s marketplace to advertise catalogues of service offerings and service plans.  They also receive and process the requests from the marketplace to provision, bind, unbind, and deprovision these offerings and plans.    #### Service Instances   Service instances are instantiations of service plans that make the functionality of those service plans available for consumption.    #### Service Bindings   Service bindings provide access details to existing service instances.  The access details are part of the service bindings' ‘credentials’ property, and typically include access URLs and credentials.    #### Service Plans   Service plans represent sets of capabilities provided by a service offering.  For example, database service offerings provide different plans for different database versions or sizes, while the Service Manager plans offer different data access levels.    #### Service Offerings   Service offerings are advertisements of the services that are supported by a service broker.  For example, software that you can consume in the subaccount.  Service offerings are related to one or more service plans.

API version: 1.0
*/

// Code generated by OpenAPI Generator (https://openapi-generator.tech); DO NOT EDIT.

package openapi

import (
	"encoding/json"
	"time"
)

// checks if the PlatformResponseObject type satisfies the MappedNullable interface at compile time
var _ MappedNullable = &PlatformResponseObject{}

// PlatformResponseObject struct for PlatformResponseObject
type PlatformResponseObject struct {
	// The time the platform was created. <br/>In ISO 8601 format:</br> YYYY-MM-DDThh:mm:ssTZD
	CreatedAt *time.Time `json:"created_at,omitempty"`
	// The description of the platform.
	Description *string `json:"description,omitempty"`
	// The ID of the platform. <br/> You can use this ID to update or to delete the platform.<br/> See the PATCH and DELETE calls for the **Platforms** group.
	Id *string `json:"id,omitempty"`
	// Additional data associated with the resource entity. <br><br>Can be an empty object.
	Labels *map[string][]string `json:"labels,omitempty"`
	LastOperation *OperationResponseObject `json:"last_operation,omitempty"`
	// The name of the platform.
	Name *string `json:"name,omitempty"`
	// Whether the platform is ready for consumption.
	Ready *bool `json:"ready,omitempty"`
	// The type of the platform.<br><br> Possible values: 
	Type *string `json:"type,omitempty"`
	// The last time the platform was updated. <br/>In ISO 8601 format.
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

// NewPlatformResponseObject instantiates a new PlatformResponseObject object
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewPlatformResponseObject() *PlatformResponseObject {
	this := PlatformResponseObject{}
	return &this
}

// NewPlatformResponseObjectWithDefaults instantiates a new PlatformResponseObject object
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewPlatformResponseObjectWithDefaults() *PlatformResponseObject {
	this := PlatformResponseObject{}
	return &this
}

// GetCreatedAt returns the CreatedAt field value if set, zero value otherwise.
func (o *PlatformResponseObject) GetCreatedAt() time.Time {
	if o == nil || IsNil(o.CreatedAt) {
		var ret time.Time
		return ret
	}
	return *o.CreatedAt
}

// GetCreatedAtOk returns a tuple with the CreatedAt field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *PlatformResponseObject) GetCreatedAtOk() (*time.Time, bool) {
	if o == nil || IsNil(o.CreatedAt) {
		return nil, false
	}
	return o.CreatedAt, true
}

// HasCreatedAt returns a boolean if a field has been set.
func (o *PlatformResponseObject) HasCreatedAt() bool {
	if o != nil && !IsNil(o.CreatedAt) {
		return true
	}

	return false
}

// SetCreatedAt gets a reference to the given time.Time and assigns it to the CreatedAt field.
func (o *PlatformResponseObject) SetCreatedAt(v time.Time) {
	o.CreatedAt = &v
}

// GetDescription returns the Description field value if set, zero value otherwise.
func (o *PlatformResponseObject) GetDescription() string {
	if o == nil || IsNil(o.Description) {
		var ret string
		return ret
	}
	return *o.Description
}

// GetDescriptionOk returns a tuple with the Description field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *PlatformResponseObject) GetDescriptionOk() (*string, bool) {
	if o == nil || IsNil(o.Description) {
		return nil, false
	}
	return o.Description, true
}

// HasDescription returns a boolean if a field has been set.
func (o *PlatformResponseObject) HasDescription() bool {
	if o != nil && !IsNil(o.Description) {
		return true
	}

	return false
}

// SetDescription gets a reference to the given string and assigns it to the Description field.
func (o *PlatformResponseObject) SetDescription(v string) {
	o.Description = &v
}

// GetId returns the Id field value if set, zero value otherwise.
func (o *PlatformResponseObject) GetId() string {
	if o == nil || IsNil(o.Id) {
		var ret string
		return ret
	}
	return *o.Id
}

// GetIdOk returns a tuple with the Id field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *PlatformResponseObject) GetIdOk() (*string, bool) {
	if o == nil || IsNil(o.Id) {
		return nil, false
	}
	return o.Id, true
}

// HasId returns a boolean if a field has been set.
func (o *PlatformResponseObject) HasId() bool {
	if o != nil && !IsNil(o.Id) {
		return true
	}

	return false
}

// SetId gets a reference to the given string and assigns it to the Id field.
func (o *PlatformResponseObject) SetId(v string) {
	o.Id = &v
}

// GetLabels returns the Labels field value if set, zero value otherwise.
func (o *PlatformResponseObject) GetLabels() map[string][]string {
	if o == nil || IsNil(o.Labels) {
		var ret map[string][]string
		return ret
	}
	return *o.Labels
}

// GetLabelsOk returns a tuple with the Labels field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *PlatformResponseObject) GetLabelsOk() (*map[string][]string, bool) {
	if o == nil || IsNil(o.Labels) {
		return nil, false
	}
	return o.Labels, true
}

// HasLabels returns a boolean if a field has been set.
func (o *PlatformResponseObject) HasLabels() bool {
	if o != nil && !IsNil(o.Labels) {
		return true
	}

	return false
}

// SetLabels gets a reference to the given map[string][]string and assigns it to the Labels field.
func (o *PlatformResponseObject) SetLabels(v map[string][]string) {
	o.Labels = &v
}

// GetLastOperation returns the LastOperation field value if set, zero value otherwise.
func (o *PlatformResponseObject) GetLastOperation() OperationResponseObject {
	if o == nil || IsNil(o.LastOperation) {
		var ret OperationResponseObject
		return ret
	}
	return *o.LastOperation
}

// GetLastOperationOk returns a tuple with the LastOperation field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *PlatformResponseObject) GetLastOperationOk() (*OperationResponseObject, bool) {
	if o == nil || IsNil(o.LastOperation) {
		return nil, false
	}
	return o.LastOperation, true
}

// HasLastOperation returns a boolean if a field has been set.
func (o *PlatformResponseObject) HasLastOperation() bool {
	if o != nil && !IsNil(o.LastOperation) {
		return true
	}

	return false
}

// SetLastOperation gets a reference to the given OperationResponseObject and assigns it to the LastOperation field.
func (o *PlatformResponseObject) SetLastOperation(v OperationResponseObject) {
	o.LastOperation = &v
}

// GetName returns the Name field value if set, zero value otherwise.
func (o *PlatformResponseObject) GetName() string {
	if o == nil || IsNil(o.Name) {
		var ret string
		return ret
	}
	return *o.Name
}

// GetNameOk returns a tuple with the Name field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *PlatformResponseObject) GetNameOk() (*string, bool) {
	if o == nil || IsNil(o.Name) {
		return nil, false
	}
	return o.Name, true
}

// HasName returns a boolean if a field has been set.
func (o *PlatformResponseObject) HasName() bool {
	if o != nil && !IsNil(o.Name) {
		return true
	}

	return false
}

// SetName gets a reference to the given string and assigns it to the Name field.
func (o *PlatformResponseObject) SetName(v string) {
	o.Name = &v
}

// GetReady returns the Ready field value if set, zero value otherwise.
func (o *PlatformResponseObject) GetReady() bool {
	if o == nil || IsNil(o.Ready) {
		var ret bool
		return ret
	}
	return *o.Ready
}

// GetReadyOk returns a tuple with the Ready field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *PlatformResponseObject) GetReadyOk() (*bool, bool) {
	if o == nil || IsNil(o.Ready) {
		return nil, false
	}
	return o.Ready, true
}

// HasReady returns a boolean if a field has been set.
func (o *PlatformResponseObject) HasReady() bool {
	if o != nil && !IsNil(o.Ready) {
		return true
	}

	return false
}

// SetReady gets a reference to the given bool and assigns it to the Ready field.
func (o *PlatformResponseObject) SetReady(v bool) {
	o.Ready = &v
}

// GetType returns the Type field value if set, zero value otherwise.
func (o *PlatformResponseObject) GetType() string {
	if o == nil || IsNil(o.Type) {
		var ret string
		return ret
	}
	return *o.Type
}

// GetTypeOk returns a tuple with the Type field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *PlatformResponseObject) GetTypeOk() (*string, bool) {
	if o == nil || IsNil(o.Type) {
		return nil, false
	}
	return o.Type, true
}

// HasType returns a boolean if a field has been set.
func (o *PlatformResponseObject) HasType() bool {
	if o != nil && !IsNil(o.Type) {
		return true
	}

	return false
}

// SetType gets a reference to the given string and assigns it to the Type field.
func (o *PlatformResponseObject) SetType(v string) {
	o.Type = &v
}

// GetUpdatedAt returns the UpdatedAt field value if set, zero value otherwise.
func (o *PlatformResponseObject) GetUpdatedAt() time.Time {
	if o == nil || IsNil(o.UpdatedAt) {
		var ret time.Time
		return ret
	}
	return *o.UpdatedAt
}

// GetUpdatedAtOk returns a tuple with the UpdatedAt field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *PlatformResponseObject) GetUpdatedAtOk() (*time.Time, bool) {
	if o == nil || IsNil(o.UpdatedAt) {
		return nil, false
	}
	return o.UpdatedAt, true
}

// HasUpdatedAt returns a boolean if a field has been set.
func (o *PlatformResponseObject) HasUpdatedAt() bool {
	if o != nil && !IsNil(o.UpdatedAt) {
		return true
	}

	return false
}

// SetUpdatedAt gets a reference to the given time.Time and assigns it to the UpdatedAt field.
func (o *PlatformResponseObject) SetUpdatedAt(v time.Time) {
	o.UpdatedAt = &v
}

func (o PlatformResponseObject) MarshalJSON() ([]byte, error) {
	toSerialize,err := o.ToMap()
	if err != nil {
		return []byte{}, err
	}
	return json.Marshal(toSerialize)
}

func (o PlatformResponseObject) ToMap() (map[string]interface{}, error) {
	toSerialize := map[string]interface{}{}
	if !IsNil(o.CreatedAt) {
		toSerialize["created_at"] = o.CreatedAt
	}
	if !IsNil(o.Description) {
		toSerialize["description"] = o.Description
	}
	if !IsNil(o.Id) {
		toSerialize["id"] = o.Id
	}
	if !IsNil(o.Labels) {
		toSerialize["labels"] = o.Labels
	}
	if !IsNil(o.LastOperation) {
		toSerialize["last_operation"] = o.LastOperation
	}
	if !IsNil(o.Name) {
		toSerialize["name"] = o.Name
	}
	if !IsNil(o.Ready) {
		toSerialize["ready"] = o.Ready
	}
	if !IsNil(o.Type) {
		toSerialize["type"] = o.Type
	}
	if !IsNil(o.UpdatedAt) {
		toSerialize["updated_at"] = o.UpdatedAt
	}
	return toSerialize, nil
}

type NullablePlatformResponseObject struct {
	value *PlatformResponseObject
	isSet bool
}

func (v NullablePlatformResponseObject) Get() *PlatformResponseObject {
	return v.value
}

func (v *NullablePlatformResponseObject) Set(val *PlatformResponseObject) {
	v.value = val
	v.isSet = true
}

func (v NullablePlatformResponseObject) IsSet() bool {
	return v.isSet
}

func (v *NullablePlatformResponseObject) Unset() {
	v.value = nil
	v.isSet = false
}

func NewNullablePlatformResponseObject(val *PlatformResponseObject) *NullablePlatformResponseObject {
	return &NullablePlatformResponseObject{value: val, isSet: true}
}

func (v NullablePlatformResponseObject) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.value)
}

func (v *NullablePlatformResponseObject) UnmarshalJSON(src []byte) error {
	v.isSet = true
	return json.Unmarshal(src, &v.value)
}


