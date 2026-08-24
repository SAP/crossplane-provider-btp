# RotateTrustCertificateResponse

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**ActiveCertificate** | Pointer to **string** | X.509 trust certificate in PEM format | [optional] 
**PassiveCertificate** | Pointer to **string** | X.509 trust certificate in PEM format | [optional] 

## Methods

### NewRotateTrustCertificateResponse

`func NewRotateTrustCertificateResponse() *RotateTrustCertificateResponse`

NewRotateTrustCertificateResponse instantiates a new RotateTrustCertificateResponse object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewRotateTrustCertificateResponseWithDefaults

`func NewRotateTrustCertificateResponseWithDefaults() *RotateTrustCertificateResponse`

NewRotateTrustCertificateResponseWithDefaults instantiates a new RotateTrustCertificateResponse object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetActiveCertificate

`func (o *RotateTrustCertificateResponse) GetActiveCertificate() string`

GetActiveCertificate returns the ActiveCertificate field if non-nil, zero value otherwise.

### GetActiveCertificateOk

`func (o *RotateTrustCertificateResponse) GetActiveCertificateOk() (*string, bool)`

GetActiveCertificateOk returns a tuple with the ActiveCertificate field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetActiveCertificate

`func (o *RotateTrustCertificateResponse) SetActiveCertificate(v string)`

SetActiveCertificate sets ActiveCertificate field to given value.

### HasActiveCertificate

`func (o *RotateTrustCertificateResponse) HasActiveCertificate() bool`

HasActiveCertificate returns a boolean if a field has been set.

### GetPassiveCertificate

`func (o *RotateTrustCertificateResponse) GetPassiveCertificate() string`

GetPassiveCertificate returns the PassiveCertificate field if non-nil, zero value otherwise.

### GetPassiveCertificateOk

`func (o *RotateTrustCertificateResponse) GetPassiveCertificateOk() (*string, bool)`

GetPassiveCertificateOk returns a tuple with the PassiveCertificate field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetPassiveCertificate

`func (o *RotateTrustCertificateResponse) SetPassiveCertificate(v string)`

SetPassiveCertificate sets PassiveCertificate field to given value.

### HasPassiveCertificate

`func (o *RotateTrustCertificateResponse) HasPassiveCertificate() bool`

HasPassiveCertificate returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


