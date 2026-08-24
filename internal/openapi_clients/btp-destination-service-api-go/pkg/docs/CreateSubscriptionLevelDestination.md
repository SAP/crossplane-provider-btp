# CreateSubscriptionLevelDestination

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Name** | **string** | Name of the destination configuration | 
**Type** | **string** | Type of the destination configuration | 
**Metadata** | Pointer to [**CreateSubscriptionLevelDestinationMetadata**](CreateSubscriptionLevelDestinationMetadata.md) |  | [optional] 
**PropertyName** | Pointer to **string** | Name of the destination property | [optional] 

## Methods

### NewCreateSubscriptionLevelDestination

`func NewCreateSubscriptionLevelDestination(name string, type_ string, ) *CreateSubscriptionLevelDestination`

NewCreateSubscriptionLevelDestination instantiates a new CreateSubscriptionLevelDestination object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewCreateSubscriptionLevelDestinationWithDefaults

`func NewCreateSubscriptionLevelDestinationWithDefaults() *CreateSubscriptionLevelDestination`

NewCreateSubscriptionLevelDestinationWithDefaults instantiates a new CreateSubscriptionLevelDestination object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetName

`func (o *CreateSubscriptionLevelDestination) GetName() string`

GetName returns the Name field if non-nil, zero value otherwise.

### GetNameOk

`func (o *CreateSubscriptionLevelDestination) GetNameOk() (*string, bool)`

GetNameOk returns a tuple with the Name field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetName

`func (o *CreateSubscriptionLevelDestination) SetName(v string)`

SetName sets Name field to given value.


### GetType

`func (o *CreateSubscriptionLevelDestination) GetType() string`

GetType returns the Type field if non-nil, zero value otherwise.

### GetTypeOk

`func (o *CreateSubscriptionLevelDestination) GetTypeOk() (*string, bool)`

GetTypeOk returns a tuple with the Type field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetType

`func (o *CreateSubscriptionLevelDestination) SetType(v string)`

SetType sets Type field to given value.


### GetMetadata

`func (o *CreateSubscriptionLevelDestination) GetMetadata() CreateSubscriptionLevelDestinationMetadata`

GetMetadata returns the Metadata field if non-nil, zero value otherwise.

### GetMetadataOk

`func (o *CreateSubscriptionLevelDestination) GetMetadataOk() (*CreateSubscriptionLevelDestinationMetadata, bool)`

GetMetadataOk returns a tuple with the Metadata field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetMetadata

`func (o *CreateSubscriptionLevelDestination) SetMetadata(v CreateSubscriptionLevelDestinationMetadata)`

SetMetadata sets Metadata field to given value.

### HasMetadata

`func (o *CreateSubscriptionLevelDestination) HasMetadata() bool`

HasMetadata returns a boolean if a field has been set.

### GetPropertyName

`func (o *CreateSubscriptionLevelDestination) GetPropertyName() string`

GetPropertyName returns the PropertyName field if non-nil, zero value otherwise.

### GetPropertyNameOk

`func (o *CreateSubscriptionLevelDestination) GetPropertyNameOk() (*string, bool)`

GetPropertyNameOk returns a tuple with the PropertyName field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetPropertyName

`func (o *CreateSubscriptionLevelDestination) SetPropertyName(v string)`

SetPropertyName sets PropertyName field to given value.

### HasPropertyName

`func (o *CreateSubscriptionLevelDestination) HasPropertyName() bool`

HasPropertyName returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


