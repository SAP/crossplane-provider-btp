# AuthToken

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Type** | **string** | Type of the token. \&quot;Basic\&quot;, \&quot;Bearer\&quot; etc. | 
**Value** | **string** | Base64 encoded token binary content | 
**RefreshToken** | Pointer to **string** | Base64 encoded refresh token binary content | [optional] 
**HttpHeader** | [**HttpHeader**](HttpHeader.md) |  | 
**Scope** | Pointer to **string** | The scopes issued with the token. The value of the scope parameter is expressed as a list of space-delimited strings. For example \&quot;read write execute\&quot; | [optional] 

## Methods

### NewAuthToken

`func NewAuthToken(type_ string, value string, httpHeader HttpHeader, ) *AuthToken`

NewAuthToken instantiates a new AuthToken object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewAuthTokenWithDefaults

`func NewAuthTokenWithDefaults() *AuthToken`

NewAuthTokenWithDefaults instantiates a new AuthToken object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetType

`func (o *AuthToken) GetType() string`

GetType returns the Type field if non-nil, zero value otherwise.

### GetTypeOk

`func (o *AuthToken) GetTypeOk() (*string, bool)`

GetTypeOk returns a tuple with the Type field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetType

`func (o *AuthToken) SetType(v string)`

SetType sets Type field to given value.


### GetValue

`func (o *AuthToken) GetValue() string`

GetValue returns the Value field if non-nil, zero value otherwise.

### GetValueOk

`func (o *AuthToken) GetValueOk() (*string, bool)`

GetValueOk returns a tuple with the Value field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetValue

`func (o *AuthToken) SetValue(v string)`

SetValue sets Value field to given value.


### GetRefreshToken

`func (o *AuthToken) GetRefreshToken() string`

GetRefreshToken returns the RefreshToken field if non-nil, zero value otherwise.

### GetRefreshTokenOk

`func (o *AuthToken) GetRefreshTokenOk() (*string, bool)`

GetRefreshTokenOk returns a tuple with the RefreshToken field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRefreshToken

`func (o *AuthToken) SetRefreshToken(v string)`

SetRefreshToken sets RefreshToken field to given value.

### HasRefreshToken

`func (o *AuthToken) HasRefreshToken() bool`

HasRefreshToken returns a boolean if a field has been set.

### GetHttpHeader

`func (o *AuthToken) GetHttpHeader() HttpHeader`

GetHttpHeader returns the HttpHeader field if non-nil, zero value otherwise.

### GetHttpHeaderOk

`func (o *AuthToken) GetHttpHeaderOk() (*HttpHeader, bool)`

GetHttpHeaderOk returns a tuple with the HttpHeader field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetHttpHeader

`func (o *AuthToken) SetHttpHeader(v HttpHeader)`

SetHttpHeader sets HttpHeader field to given value.


### GetScope

`func (o *AuthToken) GetScope() string`

GetScope returns the Scope field if non-nil, zero value otherwise.

### GetScopeOk

`func (o *AuthToken) GetScopeOk() (*string, bool)`

GetScopeOk returns a tuple with the Scope field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetScope

`func (o *AuthToken) SetScope(v string)`

SetScope sets Scope field to given value.

### HasScope

`func (o *AuthToken) HasScope() bool`

HasScope returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


