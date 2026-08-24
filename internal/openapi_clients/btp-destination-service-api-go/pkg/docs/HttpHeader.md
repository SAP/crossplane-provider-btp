# HttpHeader

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Key** | **string** | Key of the header. \&quot;Authorization\&quot;, \&quot;Cookie\&quot; etc. | 
**Value** | **string** | The appropriate authorization scheme (token type) and token using the correct separator | 

## Methods

### NewHttpHeader

`func NewHttpHeader(key string, value string, ) *HttpHeader`

NewHttpHeader instantiates a new HttpHeader object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewHttpHeaderWithDefaults

`func NewHttpHeaderWithDefaults() *HttpHeader`

NewHttpHeaderWithDefaults instantiates a new HttpHeader object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetKey

`func (o *HttpHeader) GetKey() string`

GetKey returns the Key field if non-nil, zero value otherwise.

### GetKeyOk

`func (o *HttpHeader) GetKeyOk() (*string, bool)`

GetKeyOk returns a tuple with the Key field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetKey

`func (o *HttpHeader) SetKey(v string)`

SetKey sets Key field to given value.


### GetValue

`func (o *HttpHeader) GetValue() string`

GetValue returns the Value field if non-nil, zero value otherwise.

### GetValueOk

`func (o *HttpHeader) GetValueOk() (*string, bool)`

GetValueOk returns a tuple with the Value field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetValue

`func (o *HttpHeader) SetValue(v string)`

SetValue sets Value field to given value.



[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


