# BulkResponse

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Name** | **string** | Configuration&#39;s name | 
**Status** | **int32** | Status code | 
**Etag** | Pointer to **string** | Current server-side ETag value of the resource. | [optional] 
**Cause** | Pointer to **string** | Cause error description | [optional] 

## Methods

### NewBulkResponse

`func NewBulkResponse(name string, status int32, ) *BulkResponse`

NewBulkResponse instantiates a new BulkResponse object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewBulkResponseWithDefaults

`func NewBulkResponseWithDefaults() *BulkResponse`

NewBulkResponseWithDefaults instantiates a new BulkResponse object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetName

`func (o *BulkResponse) GetName() string`

GetName returns the Name field if non-nil, zero value otherwise.

### GetNameOk

`func (o *BulkResponse) GetNameOk() (*string, bool)`

GetNameOk returns a tuple with the Name field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetName

`func (o *BulkResponse) SetName(v string)`

SetName sets Name field to given value.


### GetStatus

`func (o *BulkResponse) GetStatus() int32`

GetStatus returns the Status field if non-nil, zero value otherwise.

### GetStatusOk

`func (o *BulkResponse) GetStatusOk() (*int32, bool)`

GetStatusOk returns a tuple with the Status field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetStatus

`func (o *BulkResponse) SetStatus(v int32)`

SetStatus sets Status field to given value.


### GetEtag

`func (o *BulkResponse) GetEtag() string`

GetEtag returns the Etag field if non-nil, zero value otherwise.

### GetEtagOk

`func (o *BulkResponse) GetEtagOk() (*string, bool)`

GetEtagOk returns a tuple with the Etag field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetEtag

`func (o *BulkResponse) SetEtag(v string)`

SetEtag sets Etag field to given value.

### HasEtag

`func (o *BulkResponse) HasEtag() bool`

HasEtag returns a boolean if a field has been set.

### GetCause

`func (o *BulkResponse) GetCause() string`

GetCause returns the Cause field if non-nil, zero value otherwise.

### GetCauseOk

`func (o *BulkResponse) GetCauseOk() (*string, bool)`

GetCauseOk returns a tuple with the Cause field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCause

`func (o *BulkResponse) SetCause(v string)`

SetCause sets Cause field to given value.

### HasCause

`func (o *BulkResponse) HasCause() bool`

HasCause returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


