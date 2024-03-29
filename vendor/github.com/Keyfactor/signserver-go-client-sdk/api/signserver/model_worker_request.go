/*
Copyright 2022 Keyfactor
Licensed under the Apache License, Version 2.0 (the "License"); you may
not use this file except in compliance with the License.  You may obtain a
copy of the License at http://www.apache.org/licenses/LICENSE-2.0.  Unless
required by applicable law or agreed to in writing, software distributed
under the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES
OR CONDITIONS OF ANY KIND, either express or implied. See the License for
thespecific language governing permissions and limitations under the
License.

SignServer REST Interface

No description provided (generated by Openapi Generator https://github.com/openapitools/openapi-generator)

API version: 1.0
*/

// Code generated by OpenAPI Generator (https://openapi-generator.tech); DO NOT EDIT.

package signserver

import (
	"encoding/json"
)

// checks if the WorkerRequest type satisfies the MappedNullable interface at compile time
var _ MappedNullable = &WorkerRequest{}

// WorkerRequest Represents a worker request.
type WorkerRequest struct {
	// Worker properties list
	Properties           *map[string]string `json:"properties,omitempty"`
	AdditionalProperties map[string]interface{}
}

type _WorkerRequest WorkerRequest

// NewWorkerRequest instantiates a new WorkerRequest object
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewWorkerRequest() *WorkerRequest {
	this := WorkerRequest{}
	return &this
}

// NewWorkerRequestWithDefaults instantiates a new WorkerRequest object
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewWorkerRequestWithDefaults() *WorkerRequest {
	this := WorkerRequest{}
	return &this
}

// GetProperties returns the Properties field value if set, zero value otherwise.
func (o *WorkerRequest) GetProperties() map[string]string {
	if o == nil || isNil(o.Properties) {
		var ret map[string]string
		return ret
	}
	return *o.Properties
}

// GetPropertiesOk returns a tuple with the Properties field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *WorkerRequest) GetPropertiesOk() (*map[string]string, bool) {
	if o == nil || isNil(o.Properties) {
		return nil, false
	}
	return o.Properties, true
}

// HasProperties returns a boolean if a field has been set.
func (o *WorkerRequest) HasProperties() bool {
	if o != nil && !isNil(o.Properties) {
		return true
	}

	return false
}

// SetProperties gets a reference to the given map[string]string and assigns it to the Properties field.
func (o *WorkerRequest) SetProperties(v map[string]string) {
	o.Properties = &v
}

func (o WorkerRequest) MarshalJSON() ([]byte, error) {
	toSerialize, err := o.ToMap()
	if err != nil {
		return []byte{}, err
	}
	return json.Marshal(toSerialize)
}

func (o WorkerRequest) ToMap() (map[string]interface{}, error) {
	toSerialize := map[string]interface{}{}
	if !isNil(o.Properties) {
		toSerialize["properties"] = o.Properties
	}

	for key, value := range o.AdditionalProperties {
		toSerialize[key] = value
	}

	return toSerialize, nil
}

func (o *WorkerRequest) UnmarshalJSON(bytes []byte) (err error) {
	varWorkerRequest := _WorkerRequest{}

	if err = json.Unmarshal(bytes, &varWorkerRequest); err == nil {
		*o = WorkerRequest(varWorkerRequest)
	}

	additionalProperties := make(map[string]interface{})

	if err = json.Unmarshal(bytes, &additionalProperties); err == nil {
		delete(additionalProperties, "properties")
		o.AdditionalProperties = additionalProperties
	}

	return err
}

type NullableWorkerRequest struct {
	value *WorkerRequest
	isSet bool
}

func (v NullableWorkerRequest) Get() *WorkerRequest {
	return v.value
}

func (v *NullableWorkerRequest) Set(val *WorkerRequest) {
	v.value = val
	v.isSet = true
}

func (v NullableWorkerRequest) IsSet() bool {
	return v.isSet
}

func (v *NullableWorkerRequest) Unset() {
	v.value = nil
	v.isSet = false
}

func NewNullableWorkerRequest(val *WorkerRequest) *NullableWorkerRequest {
	return &NullableWorkerRequest{value: val, isSet: true}
}

func (v NullableWorkerRequest) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.value)
}

func (v *NullableWorkerRequest) UnmarshalJSON(src []byte) error {
	v.isSet = true
	return json.Unmarshal(src, &v.value)
}
