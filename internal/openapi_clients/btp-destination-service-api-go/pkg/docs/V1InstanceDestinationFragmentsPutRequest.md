# V1InstanceDestinationFragmentsPutRequest

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**FragmentName** | **string** | Name of the fragment configuration | 
**PropertyName** | Pointer to **string** | Name of the fragment property | [optional] 
**Metadata** | Pointer to [**CreateSubscriptionLevelFragmentMetadata**](CreateSubscriptionLevelFragmentMetadata.md) |  | [optional] 

## Methods

### NewV1InstanceDestinationFragmentsPutRequest

`func NewV1InstanceDestinationFragmentsPutRequest(fragmentName string, ) *V1InstanceDestinationFragmentsPutRequest`

NewV1InstanceDestinationFragmentsPutRequest instantiates a new V1InstanceDestinationFragmentsPutRequest object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewV1InstanceDestinationFragmentsPutRequestWithDefaults

`func NewV1InstanceDestinationFragmentsPutRequestWithDefaults() *V1InstanceDestinationFragmentsPutRequest`

NewV1InstanceDestinationFragmentsPutRequestWithDefaults instantiates a new V1InstanceDestinationFragmentsPutRequest object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetFragmentName

`func (o *V1InstanceDestinationFragmentsPutRequest) GetFragmentName() string`

GetFragmentName returns the FragmentName field if non-nil, zero value otherwise.

### GetFragmentNameOk

`func (o *V1InstanceDestinationFragmentsPutRequest) GetFragmentNameOk() (*string, bool)`

GetFragmentNameOk returns a tuple with the FragmentName field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetFragmentName

`func (o *V1InstanceDestinationFragmentsPutRequest) SetFragmentName(v string)`

SetFragmentName sets FragmentName field to given value.


### GetPropertyName

`func (o *V1InstanceDestinationFragmentsPutRequest) GetPropertyName() string`

GetPropertyName returns the PropertyName field if non-nil, zero value otherwise.

### GetPropertyNameOk

`func (o *V1InstanceDestinationFragmentsPutRequest) GetPropertyNameOk() (*string, bool)`

GetPropertyNameOk returns a tuple with the PropertyName field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetPropertyName

`func (o *V1InstanceDestinationFragmentsPutRequest) SetPropertyName(v string)`

SetPropertyName sets PropertyName field to given value.

### HasPropertyName

`func (o *V1InstanceDestinationFragmentsPutRequest) HasPropertyName() bool`

HasPropertyName returns a boolean if a field has been set.

### GetMetadata

`func (o *V1InstanceDestinationFragmentsPutRequest) GetMetadata() CreateSubscriptionLevelFragmentMetadata`

GetMetadata returns the Metadata field if non-nil, zero value otherwise.

### GetMetadataOk

`func (o *V1InstanceDestinationFragmentsPutRequest) GetMetadataOk() (*CreateSubscriptionLevelFragmentMetadata, bool)`

GetMetadataOk returns a tuple with the Metadata field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetMetadata

`func (o *V1InstanceDestinationFragmentsPutRequest) SetMetadata(v CreateSubscriptionLevelFragmentMetadata)`

SetMetadata sets Metadata field to given value.

### HasMetadata

`func (o *V1InstanceDestinationFragmentsPutRequest) HasMetadata() bool`

HasMetadata returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


