# BulkDeleteSummaryElement

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Name** | **string** | The name of the configuration | 
**Status** | **string** | The status of the operation for this configuration | 
**Reason** | Pointer to **string** | This field is only present when the status is \&quot;NOT_FOUND\&quot; and represents the reason for this status | [optional] 

## Methods

### NewBulkDeleteSummaryElement

`func NewBulkDeleteSummaryElement(name string, status string, ) *BulkDeleteSummaryElement`

NewBulkDeleteSummaryElement instantiates a new BulkDeleteSummaryElement object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewBulkDeleteSummaryElementWithDefaults

`func NewBulkDeleteSummaryElementWithDefaults() *BulkDeleteSummaryElement`

NewBulkDeleteSummaryElementWithDefaults instantiates a new BulkDeleteSummaryElement object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetName

`func (o *BulkDeleteSummaryElement) GetName() string`

GetName returns the Name field if non-nil, zero value otherwise.

### GetNameOk

`func (o *BulkDeleteSummaryElement) GetNameOk() (*string, bool)`

GetNameOk returns a tuple with the Name field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetName

`func (o *BulkDeleteSummaryElement) SetName(v string)`

SetName sets Name field to given value.


### GetStatus

`func (o *BulkDeleteSummaryElement) GetStatus() string`

GetStatus returns the Status field if non-nil, zero value otherwise.

### GetStatusOk

`func (o *BulkDeleteSummaryElement) GetStatusOk() (*string, bool)`

GetStatusOk returns a tuple with the Status field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetStatus

`func (o *BulkDeleteSummaryElement) SetStatus(v string)`

SetStatus sets Status field to given value.


### GetReason

`func (o *BulkDeleteSummaryElement) GetReason() string`

GetReason returns the Reason field if non-nil, zero value otherwise.

### GetReasonOk

`func (o *BulkDeleteSummaryElement) GetReasonOk() (*string, bool)`

GetReasonOk returns a tuple with the Reason field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetReason

`func (o *BulkDeleteSummaryElement) SetReason(v string)`

SetReason sets Reason field to given value.

### HasReason

`func (o *BulkDeleteSummaryElement) HasReason() bool`

HasReason returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


