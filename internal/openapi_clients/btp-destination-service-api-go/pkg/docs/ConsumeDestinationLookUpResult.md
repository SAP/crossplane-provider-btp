# ConsumeDestinationLookUpResult

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**DestinationConfiguration** | Pointer to [**Destination**](Destination.md) |  | [optional] 
**Certificates** | Pointer to [**[]Certificate**](Certificate.md) |  | [optional] 
**AuthTokens** | Pointer to [**[]AuthToken**](AuthToken.md) |  | [optional] 

## Methods

### NewConsumeDestinationLookUpResult

`func NewConsumeDestinationLookUpResult() *ConsumeDestinationLookUpResult`

NewConsumeDestinationLookUpResult instantiates a new ConsumeDestinationLookUpResult object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewConsumeDestinationLookUpResultWithDefaults

`func NewConsumeDestinationLookUpResultWithDefaults() *ConsumeDestinationLookUpResult`

NewConsumeDestinationLookUpResultWithDefaults instantiates a new ConsumeDestinationLookUpResult object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetDestinationConfiguration

`func (o *ConsumeDestinationLookUpResult) GetDestinationConfiguration() Destination`

GetDestinationConfiguration returns the DestinationConfiguration field if non-nil, zero value otherwise.

### GetDestinationConfigurationOk

`func (o *ConsumeDestinationLookUpResult) GetDestinationConfigurationOk() (*Destination, bool)`

GetDestinationConfigurationOk returns a tuple with the DestinationConfiguration field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetDestinationConfiguration

`func (o *ConsumeDestinationLookUpResult) SetDestinationConfiguration(v Destination)`

SetDestinationConfiguration sets DestinationConfiguration field to given value.

### HasDestinationConfiguration

`func (o *ConsumeDestinationLookUpResult) HasDestinationConfiguration() bool`

HasDestinationConfiguration returns a boolean if a field has been set.

### GetCertificates

`func (o *ConsumeDestinationLookUpResult) GetCertificates() []Certificate`

GetCertificates returns the Certificates field if non-nil, zero value otherwise.

### GetCertificatesOk

`func (o *ConsumeDestinationLookUpResult) GetCertificatesOk() (*[]Certificate, bool)`

GetCertificatesOk returns a tuple with the Certificates field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCertificates

`func (o *ConsumeDestinationLookUpResult) SetCertificates(v []Certificate)`

SetCertificates sets Certificates field to given value.

### HasCertificates

`func (o *ConsumeDestinationLookUpResult) HasCertificates() bool`

HasCertificates returns a boolean if a field has been set.

### GetAuthTokens

`func (o *ConsumeDestinationLookUpResult) GetAuthTokens() []AuthToken`

GetAuthTokens returns the AuthTokens field if non-nil, zero value otherwise.

### GetAuthTokensOk

`func (o *ConsumeDestinationLookUpResult) GetAuthTokensOk() (*[]AuthToken, bool)`

GetAuthTokensOk returns a tuple with the AuthTokens field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetAuthTokens

`func (o *ConsumeDestinationLookUpResult) SetAuthTokens(v []AuthToken)`

SetAuthTokens sets AuthTokens field to given value.

### HasAuthTokens

`func (o *ConsumeDestinationLookUpResult) HasAuthTokens() bool`

HasAuthTokens returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


