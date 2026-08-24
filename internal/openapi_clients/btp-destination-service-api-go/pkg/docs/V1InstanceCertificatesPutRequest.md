# V1InstanceCertificatesPutRequest

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Name** | **string** | Name of the certificate | 
**Type** | Pointer to **string** | Type of the object. Could be null if not present or PEM | [optional] 
**Content** | **string** | Base64 encoded keystore/certificate binary content | 
**Metadata** | Pointer to [**CreateSubscriptionLevelCertificateMetadata**](CreateSubscriptionLevelCertificateMetadata.md) |  | [optional] 
**Attributes** | [**AttributesAttributes**](AttributesAttributes.md) |  | 

## Methods

### NewV1InstanceCertificatesPutRequest

`func NewV1InstanceCertificatesPutRequest(name string, content string, attributes AttributesAttributes, ) *V1InstanceCertificatesPutRequest`

NewV1InstanceCertificatesPutRequest instantiates a new V1InstanceCertificatesPutRequest object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewV1InstanceCertificatesPutRequestWithDefaults

`func NewV1InstanceCertificatesPutRequestWithDefaults() *V1InstanceCertificatesPutRequest`

NewV1InstanceCertificatesPutRequestWithDefaults instantiates a new V1InstanceCertificatesPutRequest object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetName

`func (o *V1InstanceCertificatesPutRequest) GetName() string`

GetName returns the Name field if non-nil, zero value otherwise.

### GetNameOk

`func (o *V1InstanceCertificatesPutRequest) GetNameOk() (*string, bool)`

GetNameOk returns a tuple with the Name field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetName

`func (o *V1InstanceCertificatesPutRequest) SetName(v string)`

SetName sets Name field to given value.


### GetType

`func (o *V1InstanceCertificatesPutRequest) GetType() string`

GetType returns the Type field if non-nil, zero value otherwise.

### GetTypeOk

`func (o *V1InstanceCertificatesPutRequest) GetTypeOk() (*string, bool)`

GetTypeOk returns a tuple with the Type field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetType

`func (o *V1InstanceCertificatesPutRequest) SetType(v string)`

SetType sets Type field to given value.

### HasType

`func (o *V1InstanceCertificatesPutRequest) HasType() bool`

HasType returns a boolean if a field has been set.

### GetContent

`func (o *V1InstanceCertificatesPutRequest) GetContent() string`

GetContent returns the Content field if non-nil, zero value otherwise.

### GetContentOk

`func (o *V1InstanceCertificatesPutRequest) GetContentOk() (*string, bool)`

GetContentOk returns a tuple with the Content field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetContent

`func (o *V1InstanceCertificatesPutRequest) SetContent(v string)`

SetContent sets Content field to given value.


### GetMetadata

`func (o *V1InstanceCertificatesPutRequest) GetMetadata() CreateSubscriptionLevelCertificateMetadata`

GetMetadata returns the Metadata field if non-nil, zero value otherwise.

### GetMetadataOk

`func (o *V1InstanceCertificatesPutRequest) GetMetadataOk() (*CreateSubscriptionLevelCertificateMetadata, bool)`

GetMetadataOk returns a tuple with the Metadata field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetMetadata

`func (o *V1InstanceCertificatesPutRequest) SetMetadata(v CreateSubscriptionLevelCertificateMetadata)`

SetMetadata sets Metadata field to given value.

### HasMetadata

`func (o *V1InstanceCertificatesPutRequest) HasMetadata() bool`

HasMetadata returns a boolean if a field has been set.

### GetAttributes

`func (o *V1InstanceCertificatesPutRequest) GetAttributes() AttributesAttributes`

GetAttributes returns the Attributes field if non-nil, zero value otherwise.

### GetAttributesOk

`func (o *V1InstanceCertificatesPutRequest) GetAttributesOk() (*AttributesAttributes, bool)`

GetAttributesOk returns a tuple with the Attributes field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetAttributes

`func (o *V1InstanceCertificatesPutRequest) SetAttributes(v AttributesAttributes)`

SetAttributes sets Attributes field to given value.



[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


