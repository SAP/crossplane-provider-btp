# Fragment

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**FragmentName** | **string** | Name of the fragment configuration | 
**PropertyName** | Pointer to **string** | Name of the fragment property | [optional] 

## Methods

### NewFragment

`func NewFragment(fragmentName string, ) *Fragment`

NewFragment instantiates a new Fragment object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewFragmentWithDefaults

`func NewFragmentWithDefaults() *Fragment`

NewFragmentWithDefaults instantiates a new Fragment object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetFragmentName

`func (o *Fragment) GetFragmentName() string`

GetFragmentName returns the FragmentName field if non-nil, zero value otherwise.

### GetFragmentNameOk

`func (o *Fragment) GetFragmentNameOk() (*string, bool)`

GetFragmentNameOk returns a tuple with the FragmentName field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetFragmentName

`func (o *Fragment) SetFragmentName(v string)`

SetFragmentName sets FragmentName field to given value.


### GetPropertyName

`func (o *Fragment) GetPropertyName() string`

GetPropertyName returns the PropertyName field if non-nil, zero value otherwise.

### GetPropertyNameOk

`func (o *Fragment) GetPropertyNameOk() (*string, bool)`

GetPropertyNameOk returns a tuple with the PropertyName field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetPropertyName

`func (o *Fragment) SetPropertyName(v string)`

SetPropertyName sets PropertyName field to given value.

### HasPropertyName

`func (o *Fragment) HasPropertyName() bool`

HasPropertyName returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


