# CreateSubscriptionLevelFragment

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**FragmentName** | **string** | Name of the fragment configuration | 
**Metadata** | Pointer to [**CreateSubscriptionLevelFragmentMetadata**](CreateSubscriptionLevelFragmentMetadata.md) |  | [optional] 
**PropertyName** | Pointer to **string** | Name of the fragment property | [optional] 

## Methods

### NewCreateSubscriptionLevelFragment

`func NewCreateSubscriptionLevelFragment(fragmentName string, ) *CreateSubscriptionLevelFragment`

NewCreateSubscriptionLevelFragment instantiates a new CreateSubscriptionLevelFragment object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewCreateSubscriptionLevelFragmentWithDefaults

`func NewCreateSubscriptionLevelFragmentWithDefaults() *CreateSubscriptionLevelFragment`

NewCreateSubscriptionLevelFragmentWithDefaults instantiates a new CreateSubscriptionLevelFragment object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetFragmentName

`func (o *CreateSubscriptionLevelFragment) GetFragmentName() string`

GetFragmentName returns the FragmentName field if non-nil, zero value otherwise.

### GetFragmentNameOk

`func (o *CreateSubscriptionLevelFragment) GetFragmentNameOk() (*string, bool)`

GetFragmentNameOk returns a tuple with the FragmentName field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetFragmentName

`func (o *CreateSubscriptionLevelFragment) SetFragmentName(v string)`

SetFragmentName sets FragmentName field to given value.


### GetMetadata

`func (o *CreateSubscriptionLevelFragment) GetMetadata() CreateSubscriptionLevelFragmentMetadata`

GetMetadata returns the Metadata field if non-nil, zero value otherwise.

### GetMetadataOk

`func (o *CreateSubscriptionLevelFragment) GetMetadataOk() (*CreateSubscriptionLevelFragmentMetadata, bool)`

GetMetadataOk returns a tuple with the Metadata field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetMetadata

`func (o *CreateSubscriptionLevelFragment) SetMetadata(v CreateSubscriptionLevelFragmentMetadata)`

SetMetadata sets Metadata field to given value.

### HasMetadata

`func (o *CreateSubscriptionLevelFragment) HasMetadata() bool`

HasMetadata returns a boolean if a field has been set.

### GetPropertyName

`func (o *CreateSubscriptionLevelFragment) GetPropertyName() string`

GetPropertyName returns the PropertyName field if non-nil, zero value otherwise.

### GetPropertyNameOk

`func (o *CreateSubscriptionLevelFragment) GetPropertyNameOk() (*string, bool)`

GetPropertyNameOk returns a tuple with the PropertyName field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetPropertyName

`func (o *CreateSubscriptionLevelFragment) SetPropertyName(v string)`

SetPropertyName sets PropertyName field to given value.

### HasPropertyName

`func (o *CreateSubscriptionLevelFragment) HasPropertyName() bool`

HasPropertyName returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


