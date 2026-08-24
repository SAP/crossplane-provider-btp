# PublicKey

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Alias** | Pointer to **string** | Alias of a certificate from the keystore. This attribute will not be present when the input is not a keystore. | [optional] 
**Content** | **string** | Base64 encoded certificate chain in PEM format | 
**SubjectDN** | Pointer to **string** | The subject DN of the chain&#39;s leaf certificate | [optional] 

## Methods

### NewPublicKey

`func NewPublicKey(content string, ) *PublicKey`

NewPublicKey instantiates a new PublicKey object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewPublicKeyWithDefaults

`func NewPublicKeyWithDefaults() *PublicKey`

NewPublicKeyWithDefaults instantiates a new PublicKey object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetAlias

`func (o *PublicKey) GetAlias() string`

GetAlias returns the Alias field if non-nil, zero value otherwise.

### GetAliasOk

`func (o *PublicKey) GetAliasOk() (*string, bool)`

GetAliasOk returns a tuple with the Alias field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetAlias

`func (o *PublicKey) SetAlias(v string)`

SetAlias sets Alias field to given value.

### HasAlias

`func (o *PublicKey) HasAlias() bool`

HasAlias returns a boolean if a field has been set.

### GetContent

`func (o *PublicKey) GetContent() string`

GetContent returns the Content field if non-nil, zero value otherwise.

### GetContentOk

`func (o *PublicKey) GetContentOk() (*string, bool)`

GetContentOk returns a tuple with the Content field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetContent

`func (o *PublicKey) SetContent(v string)`

SetContent sets Content field to given value.


### GetSubjectDN

`func (o *PublicKey) GetSubjectDN() string`

GetSubjectDN returns the SubjectDN field if non-nil, zero value otherwise.

### GetSubjectDNOk

`func (o *PublicKey) GetSubjectDNOk() (*string, bool)`

GetSubjectDNOk returns a tuple with the SubjectDN field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSubjectDN

`func (o *PublicKey) SetSubjectDN(v string)`

SetSubjectDN sets SubjectDN field to given value.

### HasSubjectDN

`func (o *PublicKey) HasSubjectDN() bool`

HasSubjectDN returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


