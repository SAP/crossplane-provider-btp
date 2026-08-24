# DestinationLookUpResult

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Owner** | Pointer to [**Owner**](Owner.md) |  | [optional] 
**DestinationConfiguration** | Pointer to [**Destination**](Destination.md) |  | [optional] 
**Certificates** | Pointer to [**[]Certificate**](Certificate.md) |  | [optional] 
**AuthTokens** | Pointer to [**[]AuthToken**](AuthToken.md) |  | [optional] 

## Methods

### NewDestinationLookUpResult

`func NewDestinationLookUpResult() *DestinationLookUpResult`

NewDestinationLookUpResult instantiates a new DestinationLookUpResult object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewDestinationLookUpResultWithDefaults

`func NewDestinationLookUpResultWithDefaults() *DestinationLookUpResult`

NewDestinationLookUpResultWithDefaults instantiates a new DestinationLookUpResult object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetOwner

`func (o *DestinationLookUpResult) GetOwner() Owner`

GetOwner returns the Owner field if non-nil, zero value otherwise.

### GetOwnerOk

`func (o *DestinationLookUpResult) GetOwnerOk() (*Owner, bool)`

GetOwnerOk returns a tuple with the Owner field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetOwner

`func (o *DestinationLookUpResult) SetOwner(v Owner)`

SetOwner sets Owner field to given value.

### HasOwner

`func (o *DestinationLookUpResult) HasOwner() bool`

HasOwner returns a boolean if a field has been set.

### GetDestinationConfiguration

`func (o *DestinationLookUpResult) GetDestinationConfiguration() Destination`

GetDestinationConfiguration returns the DestinationConfiguration field if non-nil, zero value otherwise.

### GetDestinationConfigurationOk

`func (o *DestinationLookUpResult) GetDestinationConfigurationOk() (*Destination, bool)`

GetDestinationConfigurationOk returns a tuple with the DestinationConfiguration field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetDestinationConfiguration

`func (o *DestinationLookUpResult) SetDestinationConfiguration(v Destination)`

SetDestinationConfiguration sets DestinationConfiguration field to given value.

### HasDestinationConfiguration

`func (o *DestinationLookUpResult) HasDestinationConfiguration() bool`

HasDestinationConfiguration returns a boolean if a field has been set.

### GetCertificates

`func (o *DestinationLookUpResult) GetCertificates() []Certificate`

GetCertificates returns the Certificates field if non-nil, zero value otherwise.

### GetCertificatesOk

`func (o *DestinationLookUpResult) GetCertificatesOk() (*[]Certificate, bool)`

GetCertificatesOk returns a tuple with the Certificates field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCertificates

`func (o *DestinationLookUpResult) SetCertificates(v []Certificate)`

SetCertificates sets Certificates field to given value.

### HasCertificates

`func (o *DestinationLookUpResult) HasCertificates() bool`

HasCertificates returns a boolean if a field has been set.

### GetAuthTokens

`func (o *DestinationLookUpResult) GetAuthTokens() []AuthToken`

GetAuthTokens returns the AuthTokens field if non-nil, zero value otherwise.

### GetAuthTokensOk

`func (o *DestinationLookUpResult) GetAuthTokensOk() (*[]AuthToken, bool)`

GetAuthTokensOk returns a tuple with the AuthTokens field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetAuthTokens

`func (o *DestinationLookUpResult) SetAuthTokens(v []AuthToken)`

SetAuthTokens sets AuthTokens field to given value.

### HasAuthTokens

`func (o *DestinationLookUpResult) HasAuthTokens() bool`

HasAuthTokens returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


