# IdpMetadata

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**IdpMetadata** | Pointer to **string** | SAML IdP metadata, containing the active X.509 certificate and the passive X.509 certificate (if it exists) | [optional] 

## Methods

### NewIdpMetadata

`func NewIdpMetadata() *IdpMetadata`

NewIdpMetadata instantiates a new IdpMetadata object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewIdpMetadataWithDefaults

`func NewIdpMetadataWithDefaults() *IdpMetadata`

NewIdpMetadataWithDefaults instantiates a new IdpMetadata object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetIdpMetadata

`func (o *IdpMetadata) GetIdpMetadata() string`

GetIdpMetadata returns the IdpMetadata field if non-nil, zero value otherwise.

### GetIdpMetadataOk

`func (o *IdpMetadata) GetIdpMetadataOk() (*string, bool)`

GetIdpMetadataOk returns a tuple with the IdpMetadata field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetIdpMetadata

`func (o *IdpMetadata) SetIdpMetadata(v string)`

SetIdpMetadata sets IdpMetadata field to given value.

### HasIdpMetadata

`func (o *IdpMetadata) HasIdpMetadata() bool`

HasIdpMetadata returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


