# V1SubaccountCertificatesGet200ResponseInner

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Name** | **string** | The &#x60;&#x60;&#x60;Name&#x60;&#x60;&#x60; field of the configuration. | 
**Type** | Pointer to **string** | Type of the object. Could be null if not present | [optional] 
**Content** | **string** | Base64 encoded keystore/certificate binary content | 

## Methods

### NewV1SubaccountCertificatesGet200ResponseInner

`func NewV1SubaccountCertificatesGet200ResponseInner(name string, content string, ) *V1SubaccountCertificatesGet200ResponseInner`

NewV1SubaccountCertificatesGet200ResponseInner instantiates a new V1SubaccountCertificatesGet200ResponseInner object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewV1SubaccountCertificatesGet200ResponseInnerWithDefaults

`func NewV1SubaccountCertificatesGet200ResponseInnerWithDefaults() *V1SubaccountCertificatesGet200ResponseInner`

NewV1SubaccountCertificatesGet200ResponseInnerWithDefaults instantiates a new V1SubaccountCertificatesGet200ResponseInner object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetName

`func (o *V1SubaccountCertificatesGet200ResponseInner) GetName() string`

GetName returns the Name field if non-nil, zero value otherwise.

### GetNameOk

`func (o *V1SubaccountCertificatesGet200ResponseInner) GetNameOk() (*string, bool)`

GetNameOk returns a tuple with the Name field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetName

`func (o *V1SubaccountCertificatesGet200ResponseInner) SetName(v string)`

SetName sets Name field to given value.


### GetType

`func (o *V1SubaccountCertificatesGet200ResponseInner) GetType() string`

GetType returns the Type field if non-nil, zero value otherwise.

### GetTypeOk

`func (o *V1SubaccountCertificatesGet200ResponseInner) GetTypeOk() (*string, bool)`

GetTypeOk returns a tuple with the Type field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetType

`func (o *V1SubaccountCertificatesGet200ResponseInner) SetType(v string)`

SetType sets Type field to given value.

### HasType

`func (o *V1SubaccountCertificatesGet200ResponseInner) HasType() bool`

HasType returns a boolean if a field has been set.

### GetContent

`func (o *V1SubaccountCertificatesGet200ResponseInner) GetContent() string`

GetContent returns the Content field if non-nil, zero value otherwise.

### GetContentOk

`func (o *V1SubaccountCertificatesGet200ResponseInner) GetContentOk() (*string, bool)`

GetContentOk returns a tuple with the Content field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetContent

`func (o *V1SubaccountCertificatesGet200ResponseInner) SetContent(v string)`

SetContent sets Content field to given value.



[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


