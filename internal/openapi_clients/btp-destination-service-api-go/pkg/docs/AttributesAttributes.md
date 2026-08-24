# AttributesAttributes

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**CN** | Pointer to **string** | Certificate common name | [optional] 
**CSR** | Pointer to **string** | Certificate signing request (base64 encoded PEM) | [optional] 
**Validity** | Pointer to [**AttributesAttributesValidity**](AttributesAttributesValidity.md) |  | [optional] 
**AutomaticRenew** | Pointer to **bool** | Enable automatic certificate renew when the certificate is close to expiring. | [optional] [default to false]
**Password** | Pointer to **string** | Password which protects the generated private key (in case of pem) or keystore. | [optional] 

## Methods

### NewAttributesAttributes

`func NewAttributesAttributes() *AttributesAttributes`

NewAttributesAttributes instantiates a new AttributesAttributes object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewAttributesAttributesWithDefaults

`func NewAttributesAttributesWithDefaults() *AttributesAttributes`

NewAttributesAttributesWithDefaults instantiates a new AttributesAttributes object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetCN

`func (o *AttributesAttributes) GetCN() string`

GetCN returns the CN field if non-nil, zero value otherwise.

### GetCNOk

`func (o *AttributesAttributes) GetCNOk() (*string, bool)`

GetCNOk returns a tuple with the CN field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCN

`func (o *AttributesAttributes) SetCN(v string)`

SetCN sets CN field to given value.

### HasCN

`func (o *AttributesAttributes) HasCN() bool`

HasCN returns a boolean if a field has been set.

### GetCSR

`func (o *AttributesAttributes) GetCSR() string`

GetCSR returns the CSR field if non-nil, zero value otherwise.

### GetCSROk

`func (o *AttributesAttributes) GetCSROk() (*string, bool)`

GetCSROk returns a tuple with the CSR field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCSR

`func (o *AttributesAttributes) SetCSR(v string)`

SetCSR sets CSR field to given value.

### HasCSR

`func (o *AttributesAttributes) HasCSR() bool`

HasCSR returns a boolean if a field has been set.

### GetValidity

`func (o *AttributesAttributes) GetValidity() AttributesAttributesValidity`

GetValidity returns the Validity field if non-nil, zero value otherwise.

### GetValidityOk

`func (o *AttributesAttributes) GetValidityOk() (*AttributesAttributesValidity, bool)`

GetValidityOk returns a tuple with the Validity field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetValidity

`func (o *AttributesAttributes) SetValidity(v AttributesAttributesValidity)`

SetValidity sets Validity field to given value.

### HasValidity

`func (o *AttributesAttributes) HasValidity() bool`

HasValidity returns a boolean if a field has been set.

### GetAutomaticRenew

`func (o *AttributesAttributes) GetAutomaticRenew() bool`

GetAutomaticRenew returns the AutomaticRenew field if non-nil, zero value otherwise.

### GetAutomaticRenewOk

`func (o *AttributesAttributes) GetAutomaticRenewOk() (*bool, bool)`

GetAutomaticRenewOk returns a tuple with the AutomaticRenew field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetAutomaticRenew

`func (o *AttributesAttributes) SetAutomaticRenew(v bool)`

SetAutomaticRenew sets AutomaticRenew field to given value.

### HasAutomaticRenew

`func (o *AttributesAttributes) HasAutomaticRenew() bool`

HasAutomaticRenew returns a boolean if a field has been set.

### GetPassword

`func (o *AttributesAttributes) GetPassword() string`

GetPassword returns the Password field if non-nil, zero value otherwise.

### GetPasswordOk

`func (o *AttributesAttributes) GetPasswordOk() (*string, bool)`

GetPasswordOk returns a tuple with the Password field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetPassword

`func (o *AttributesAttributes) SetPassword(v string)`

SetPassword sets Password field to given value.

### HasPassword

`func (o *AttributesAttributes) HasPassword() bool`

HasPassword returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


