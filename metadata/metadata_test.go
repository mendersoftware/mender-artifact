// Copyright 2016 Mender Software AS
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

package metadata

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateInfo(t *testing.T) {
	var validateTests = []struct {
		in  MetadataInfo
		err error
	}{
		{MetadataInfo{Format: "", Version: 0}, ErrInvalidInfo},
		{MetadataInfo{Format: "", Version: 1}, ErrInvalidInfo},
		{MetadataInfo{Format: "format"}, ErrInvalidInfo},
		{MetadataInfo{}, ErrInvalidInfo},
		{MetadataInfo{Format: "format", Version: 1}, nil},
	}

	for _, tt := range validateTests {
		e := tt.in.Validate()
		assert.Equal(t, e, tt.err)
	}
}

func TestValidateHeaderInfo(t *testing.T) {
	var validateTests = []struct {
		in  MetadataHeaderInfo
		err error
	}{
		{MetadataHeaderInfo{}, ErrInvalidHeaderInfo},
		{MetadataHeaderInfo{Updates: []MetadataUpdateType{}}, ErrInvalidHeaderInfo},
		{MetadataHeaderInfo{Updates: []MetadataUpdateType{{Type: ""}}}, ErrInvalidHeaderInfo},
		{MetadataHeaderInfo{Updates: []MetadataUpdateType{{Type: "update"}, {}}}, ErrInvalidHeaderInfo},
		{MetadataHeaderInfo{Updates: []MetadataUpdateType{{}, {Type: "update"}}}, ErrInvalidHeaderInfo},
		{MetadataHeaderInfo{Updates: []MetadataUpdateType{{Type: "update"}, {Type: ""}}}, ErrInvalidHeaderInfo},
		{MetadataHeaderInfo{Updates: []MetadataUpdateType{{Type: "update"}}}, nil},
		{MetadataHeaderInfo{Updates: []MetadataUpdateType{{Type: "update"}, {Type: "update"}}}, nil},
	}
	for idx, tt := range validateTests {
		e := tt.in.Validate()
		assert.Equal(t, e, tt.err, "failing test: %v (%v)", idx, tt)
	}
}

func TestValidateTypeInfo(t *testing.T) {
	var validateTests = []struct {
		in  MetadataTypeInfo
		err error
	}{
		{MetadataTypeInfo{}, ErrInvalidTypeInfo},
		{MetadataTypeInfo{Rootfs: ""}, ErrInvalidTypeInfo},
		{MetadataTypeInfo{Rootfs: "image-type"}, nil},
	}

	for _, tt := range validateTests {
		e := tt.in.Validate()
		assert.Equal(t, e, tt.err)
	}
}

func TestValidateMetadata(t *testing.T) {
	var validateTests = []struct {
		in  Metadata
		err error
	}{
		{Metadata{}, ErrInvalidMetadata},
		{Metadata{make(map[string]interface{})}, ErrInvalidMetadata},
		{Metadata{map[string]interface{}{}}, ErrInvalidMetadata},
		{Metadata{map[string]interface{}{"": nil}}, ErrInvalidMetadata},
		{Metadata{map[string]interface{}{"key": nil}}, ErrInvalidMetadata},
		{Metadata{map[string]interface{}{"key": "val"}}, ErrInvalidMetadata},
		{Metadata{map[string]interface{}{"DeviceType": "type"}}, ErrInvalidMetadata},
		{Metadata{map[string]interface{}{"DeviceType": nil, "ImageID": "image"}}, ErrInvalidMetadata},
		{Metadata{map[string]interface{}{"DeviceType": "device", "ImageID": "image"}}, nil},
		{Metadata{map[string]interface{}{"DeviceType": "device", "ImageID": "image", "Data": "data"}}, nil},
	}

	for _, tt := range validateTests {
		e := tt.in.Validate()
		assert.Equal(t, e, tt.err, "failing test: %v", tt)
	}
}

func TestValidateFiles(t *testing.T) {
	var validateTests = []struct {
		in  MetadataFiles
		err error
	}{
		{MetadataFiles{}, ErrInvalidFilesInfo},
		{MetadataFiles{Files: []MetadataFile{}}, ErrInvalidFilesInfo},
		{MetadataFiles{Files: []MetadataFile{{File: ""}}}, ErrInvalidFilesInfo},
		{MetadataFiles{Files: []MetadataFile{{File: "file"}}}, nil},
		{MetadataFiles{Files: []MetadataFile{{File: "file"}, {}}}, ErrInvalidFilesInfo},
		{MetadataFiles{Files: []MetadataFile{{File: "file"}, {File: "file_next"}}}, nil},
	}
	for idx, tt := range validateTests {
		e := tt.in.Validate()
		assert.Equal(t, e, tt.err, "failing test: %v (%v)", idx, tt)
	}
}
