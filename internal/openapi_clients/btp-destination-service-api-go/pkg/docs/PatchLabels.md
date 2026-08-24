# PatchLabels

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Action** | **string** | The action to be performed on the labels | 
**Labels** | [**[]Label**](Label.md) |  | 

## Methods

### NewPatchLabels

`func NewPatchLabels(action string, labels []Label, ) *PatchLabels`

NewPatchLabels instantiates a new PatchLabels object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewPatchLabelsWithDefaults

`func NewPatchLabelsWithDefaults() *PatchLabels`

NewPatchLabelsWithDefaults instantiates a new PatchLabels object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetAction

`func (o *PatchLabels) GetAction() string`

GetAction returns the Action field if non-nil, zero value otherwise.

### GetActionOk

`func (o *PatchLabels) GetActionOk() (*string, bool)`

GetActionOk returns a tuple with the Action field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetAction

`func (o *PatchLabels) SetAction(v string)`

SetAction sets Action field to given value.


### GetLabels

`func (o *PatchLabels) GetLabels() []Label`

GetLabels returns the Labels field if non-nil, zero value otherwise.

### GetLabelsOk

`func (o *PatchLabels) GetLabelsOk() (*[]Label, bool)`

GetLabelsOk returns a tuple with the Labels field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetLabels

`func (o *PatchLabels) SetLabels(v []Label)`

SetLabels sets Labels field to given value.



[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


