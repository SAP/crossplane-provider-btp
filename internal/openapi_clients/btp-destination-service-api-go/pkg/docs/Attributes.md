# Attributes

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Name** | **string** | Name of the certificate | 
**Type** | Pointer to **string** | Type of the object. Could be null if not present or PEM | [optional] 
**Attributes** | [**AttributesAttributes**](AttributesAttributes.md) |  | 

## Methods

### NewAttributes

`func NewAttributes(name string, attributes AttributesAttributes, ) *Attributes`

NewAttributes instantiates a new Attributes object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewAttributesWithDefaults

`func NewAttributesWithDefaults() *Attributes`

NewAttributesWithDefaults instantiates a new Attributes object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetName

`func (o *Attributes) GetName() string`

GetName returns the Name field if non-nil, zero value otherwise.

### GetNameOk

`func (o *Attributes) GetNameOk() (*string, bool)`

GetNameOk returns a tuple with the Name field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetName

`func (o *Attributes) SetName(v string)`

SetName sets Name field to given value.


### GetType

`func (o *Attributes) GetType() string`

GetType returns the Type field if non-nil, zero value otherwise.

### GetTypeOk

`func (o *Attributes) GetTypeOk() (*string, bool)`

GetTypeOk returns a tuple with the Type field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetType

`func (o *Attributes) SetType(v string)`

SetType sets Type field to given value.

### HasType

`func (o *Attributes) HasType() bool`

HasType returns a boolean if a field has been set.

### GetAttributes

`func (o *Attributes) GetAttributes() AttributesAttributes`

GetAttributes returns the Attributes field if non-nil, zero value otherwise.

### GetAttributesOk

`func (o *Attributes) GetAttributesOk() (*AttributesAttributes, bool)`

GetAttributesOk returns a tuple with the Attributes field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetAttributes

`func (o *Attributes) SetAttributes(v AttributesAttributes)`

SetAttributes sets Attributes field to given value.



[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


