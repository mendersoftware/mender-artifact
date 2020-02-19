// Copyright 2020 Northern.tech AS
//
//    Licensed under the Apache License, Version 2.0 (the "License");
//    you may not use this file except in compliance with the License.
//    You may obtain a copy of the License at
//
//        http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS,
//    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//    See the License for the specific language governing permissions and
//    limitations under the License.

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAppendStructToMap(t *testing.T) {
	testMap := make(map[string]interface{})
	testStruct := struct {
		V1 string
		V2 int
		V3 map[string]interface{}
		V4 map[int]chan interface{} `,omitempty`
	}{
		V1: "test",
		V2: 123,
		V3: map[string]interface{}{
			"foo": "bar",
			"baz": 123.0,
		},
	}

	// Successfull test
	err := AppendStructToMap(testStruct, testMap)
	assert.NoError(t, err)
	assert.Equal(t, "test", testMap["V1"])
	assert.Equal(t, 123.0, testMap["V2"])
	assert.Equal(t, testStruct.V3, testMap["V3"].(map[string]interface{}))
	assert.Nil(t, testMap["V4"])

	// Conflicting arguments
	err = AppendStructToMap("foo", testMap)
	assert.Error(t, err)

	// Illegal struct argument
	illegalStruct := struct {
		FuncField func() string
	}{
		FuncField: func() string { return "You can't Marshal me!" },
	}
	err = AppendStructToMap(illegalStruct, testMap)
	assert.Error(t, err)
}

func TestStructToMap(t *testing.T) {
	testStruct := struct {
		Foo string
		Bar []string
	}{
		Foo: "føø",
		Bar: []string{"bær", "båz"},
	}

	retMap, err := MarshallStructToMap(testStruct)
	assert.NoError(t, err)
	assert.Equal(t, "føø", retMap["Foo"])
	assert.Equal(t, []interface{}{"bær", "båz"}, retMap["Bar"])

	illegalStruct := struct {
		Chan chan struct{}
	}{
		Chan: make(chan struct{}),
	}

	retMap, err = MarshallStructToMap(illegalStruct)
	assert.Error(t, err)
}
