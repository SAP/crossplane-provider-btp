# CreateSubscriptionLevelCertificate

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Name** | **string** | Name of the certificate/keystore | 
**Type** | Pointer to **string** | Type of the object. Could be null if not present | [optional] 
**Content** | **string** | Base64 encoded keystore/certificate binary content | 
**Metadata** | Pointer to [**CreateSubscriptionLevelCertificateMetadata**](CreateSubscriptionLevelCertificateMetadata.md) |  | [optional] 

## Methods

### NewCreateSubscriptionLevelCertificate

`func NewCreateSubscriptionLevelCertificate(name string, content string, ) *CreateSubscriptionLevelCertificate`

NewCreateSubscriptionLevelCertificate instantiates a new CreateSubscriptionLevelCertificate object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewCreateSubscriptionLevelCertificateWithDefaults

`func NewCreateSubscriptionLevelCertificateWithDefaults() *CreateSubscriptionLevelCertificate`

NewCreateSubscriptionLevelCertificateWithDefaults instantiates a new CreateSubscriptionLevelCertificate object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetName

`func (o *CreateSubscriptionLevelCertificate) GetName() string`

GetName returns the Name field if non-nil, zero value otherwise.

### GetNameOk

`func (o *CreateSubscriptionLevelCertificate) GetNameOk() (*string, bool)`

GetNameOk returns a tuple with the Name field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetName

`func (o *CreateSubscriptionLevelCertificate) SetName(v string)`

SetName sets Name field to given value.


### GetType

`func (o *CreateSubscriptionLevelCertificate) GetType() string`

GetType returns the Type field if non-nil, zero value otherwise.

### GetTypeOk

`func (o *CreateSubscriptionLevelCertificate) GetTypeOk() (*string, bool)`

GetTypeOk returns a tuple with the Type field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetType

`func (o *CreateSubscriptionLevelCertificate) SetType(v string)`

SetType sets Type field to given value.

### HasType

`func (o *CreateSubscriptionLevelCertificate) HasType() bool`

HasType returns a boolean if a field has been set.

### GetContent

`func (o *CreateSubscriptionLevelCertificate) GetContent() string`

GetContent returns the Content field if non-nil, zero value otherwise.

### GetContentOk

`func (o *CreateSubscriptionLevelCertificate) GetContentOk() (*string, bool)`

GetContentOk returns a tuple with the Content field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetContent

`func (o *CreateSubscriptionLevelCertificate) SetContent(v string)`

SetContent sets Content field to given value.


### GetMetadata

`func (o *CreateSubscriptionLevelCertificate) GetMetadata() CreateSubscriptionLevelCertificateMetadata`

GetMetadata returns the Metadata field if non-nil, zero value otherwise.

### GetMetadataOk

`func (o *CreateSubscriptionLevelCertificate) GetMetadataOk() (*CreateSubscriptionLevelCertificateMetadata, bool)`

GetMetadataOk returns a tuple with the Metadata field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetMetadata

`func (o *CreateSubscriptionLevelCertificate) SetMetadata(v CreateSubscriptionLevelCertificateMetadata)`

SetMetadata sets Metadata field to given value.

### HasMetadata

`func (o *CreateSubscriptionLevelCertificate) HasMetadata() bool`

HasMetadata returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


