# V1InstanceDestinationsPutRequest

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Name** | **string** | Name of the destination configuration | 
**Type** | **string** | Type of the destination configuration | 
**PropertyName** | Pointer to **string** | Name of the destination property | [optional] 
**Metadata** | Pointer to [**CreateSubscriptionLevelDestinationMetadata**](CreateSubscriptionLevelDestinationMetadata.md) |  | [optional] 

## Methods

### NewV1InstanceDestinationsPutRequest

`func NewV1InstanceDestinationsPutRequest(name string, type_ string, ) *V1InstanceDestinationsPutRequest`

NewV1InstanceDestinationsPutRequest instantiates a new V1InstanceDestinationsPutRequest object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewV1InstanceDestinationsPutRequestWithDefaults

`func NewV1InstanceDestinationsPutRequestWithDefaults() *V1InstanceDestinationsPutRequest`

NewV1InstanceDestinationsPutRequestWithDefaults instantiates a new V1InstanceDestinationsPutRequest object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetName

`func (o *V1InstanceDestinationsPutRequest) GetName() string`

GetName returns the Name field if non-nil, zero value otherwise.

### GetNameOk

`func (o *V1InstanceDestinationsPutRequest) GetNameOk() (*string, bool)`

GetNameOk returns a tuple with the Name field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetName

`func (o *V1InstanceDestinationsPutRequest) SetName(v string)`

SetName sets Name field to given value.


### GetType

`func (o *V1InstanceDestinationsPutRequest) GetType() string`

GetType returns the Type field if non-nil, zero value otherwise.

### GetTypeOk

`func (o *V1InstanceDestinationsPutRequest) GetTypeOk() (*string, bool)`

GetTypeOk returns a tuple with the Type field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetType

`func (o *V1InstanceDestinationsPutRequest) SetType(v string)`

SetType sets Type field to given value.


### GetPropertyName

`func (o *V1InstanceDestinationsPutRequest) GetPropertyName() string`

GetPropertyName returns the PropertyName field if non-nil, zero value otherwise.

### GetPropertyNameOk

`func (o *V1InstanceDestinationsPutRequest) GetPropertyNameOk() (*string, bool)`

GetPropertyNameOk returns a tuple with the PropertyName field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetPropertyName

`func (o *V1InstanceDestinationsPutRequest) SetPropertyName(v string)`

SetPropertyName sets PropertyName field to given value.

### HasPropertyName

`func (o *V1InstanceDestinationsPutRequest) HasPropertyName() bool`

HasPropertyName returns a boolean if a field has been set.

### GetMetadata

`func (o *V1InstanceDestinationsPutRequest) GetMetadata() CreateSubscriptionLevelDestinationMetadata`

GetMetadata returns the Metadata field if non-nil, zero value otherwise.

### GetMetadataOk

`func (o *V1InstanceDestinationsPutRequest) GetMetadataOk() (*CreateSubscriptionLevelDestinationMetadata, bool)`

GetMetadataOk returns a tuple with the Metadata field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetMetadata

`func (o *V1InstanceDestinationsPutRequest) SetMetadata(v CreateSubscriptionLevelDestinationMetadata)`

SetMetadata sets Metadata field to given value.

### HasMetadata

`func (o *V1InstanceDestinationsPutRequest) HasMetadata() bool`

HasMetadata returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


