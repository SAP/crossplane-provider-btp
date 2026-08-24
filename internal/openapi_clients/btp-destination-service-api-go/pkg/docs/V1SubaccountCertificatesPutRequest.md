# V1SubaccountCertificatesPutRequest

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Name** | **string** | Name of the certificate | 
**Type** | Pointer to **string** | Type of the object. Could be null if not present or PEM | [optional] 
**Content** | **string** | Base64 encoded keystore/certificate binary content | 
**Attributes** | [**AttributesAttributes**](AttributesAttributes.md) |  | 

## Methods

### NewV1SubaccountCertificatesPutRequest

`func NewV1SubaccountCertificatesPutRequest(name string, content string, attributes AttributesAttributes, ) *V1SubaccountCertificatesPutRequest`

NewV1SubaccountCertificatesPutRequest instantiates a new V1SubaccountCertificatesPutRequest object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewV1SubaccountCertificatesPutRequestWithDefaults

`func NewV1SubaccountCertificatesPutRequestWithDefaults() *V1SubaccountCertificatesPutRequest`

NewV1SubaccountCertificatesPutRequestWithDefaults instantiates a new V1SubaccountCertificatesPutRequest object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetName

`func (o *V1SubaccountCertificatesPutRequest) GetName() string`

GetName returns the Name field if non-nil, zero value otherwise.

### GetNameOk

`func (o *V1SubaccountCertificatesPutRequest) GetNameOk() (*string, bool)`

GetNameOk returns a tuple with the Name field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetName

`func (o *V1SubaccountCertificatesPutRequest) SetName(v string)`

SetName sets Name field to given value.


### GetType

`func (o *V1SubaccountCertificatesPutRequest) GetType() string`

GetType returns the Type field if non-nil, zero value otherwise.

### GetTypeOk

`func (o *V1SubaccountCertificatesPutRequest) GetTypeOk() (*string, bool)`

GetTypeOk returns a tuple with the Type field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetType

`func (o *V1SubaccountCertificatesPutRequest) SetType(v string)`

SetType sets Type field to given value.

### HasType

`func (o *V1SubaccountCertificatesPutRequest) HasType() bool`

HasType returns a boolean if a field has been set.

### GetContent

`func (o *V1SubaccountCertificatesPutRequest) GetContent() string`

GetContent returns the Content field if non-nil, zero value otherwise.

### GetContentOk

`func (o *V1SubaccountCertificatesPutRequest) GetContentOk() (*string, bool)`

GetContentOk returns a tuple with the Content field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetContent

`func (o *V1SubaccountCertificatesPutRequest) SetContent(v string)`

SetContent sets Content field to given value.


### GetAttributes

`func (o *V1SubaccountCertificatesPutRequest) GetAttributes() AttributesAttributes`

GetAttributes returns the Attributes field if non-nil, zero value otherwise.

### GetAttributesOk

`func (o *V1SubaccountCertificatesPutRequest) GetAttributesOk() (*AttributesAttributes, bool)`

GetAttributesOk returns a tuple with the Attributes field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetAttributes

`func (o *V1SubaccountCertificatesPutRequest) SetAttributes(v AttributesAttributes)`

SetAttributes sets Attributes field to given value.



[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


