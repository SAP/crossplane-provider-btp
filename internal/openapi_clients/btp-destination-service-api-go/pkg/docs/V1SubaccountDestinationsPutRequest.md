# V1SubaccountDestinationsPutRequest

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Name** | **string** | Name of the destination configuration | 
**Type** | **string** | Type of the destination configuration | 
**PropertyName** | Pointer to **string** | Name of the destination property | [optional] 

## Methods

### NewV1SubaccountDestinationsPutRequest

`func NewV1SubaccountDestinationsPutRequest(name string, type_ string, ) *V1SubaccountDestinationsPutRequest`

NewV1SubaccountDestinationsPutRequest instantiates a new V1SubaccountDestinationsPutRequest object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewV1SubaccountDestinationsPutRequestWithDefaults

`func NewV1SubaccountDestinationsPutRequestWithDefaults() *V1SubaccountDestinationsPutRequest`

NewV1SubaccountDestinationsPutRequestWithDefaults instantiates a new V1SubaccountDestinationsPutRequest object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetName

`func (o *V1SubaccountDestinationsPutRequest) GetName() string`

GetName returns the Name field if non-nil, zero value otherwise.

### GetNameOk

`func (o *V1SubaccountDestinationsPutRequest) GetNameOk() (*string, bool)`

GetNameOk returns a tuple with the Name field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetName

`func (o *V1SubaccountDestinationsPutRequest) SetName(v string)`

SetName sets Name field to given value.


### GetType

`func (o *V1SubaccountDestinationsPutRequest) GetType() string`

GetType returns the Type field if non-nil, zero value otherwise.

### GetTypeOk

`func (o *V1SubaccountDestinationsPutRequest) GetTypeOk() (*string, bool)`

GetTypeOk returns a tuple with the Type field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetType

`func (o *V1SubaccountDestinationsPutRequest) SetType(v string)`

SetType sets Type field to given value.


### GetPropertyName

`func (o *V1SubaccountDestinationsPutRequest) GetPropertyName() string`

GetPropertyName returns the PropertyName field if non-nil, zero value otherwise.

### GetPropertyNameOk

`func (o *V1SubaccountDestinationsPutRequest) GetPropertyNameOk() (*string, bool)`

GetPropertyNameOk returns a tuple with the PropertyName field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetPropertyName

`func (o *V1SubaccountDestinationsPutRequest) SetPropertyName(v string)`

SetPropertyName sets PropertyName field to given value.

### HasPropertyName

`func (o *V1SubaccountDestinationsPutRequest) HasPropertyName() bool`

HasPropertyName returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


