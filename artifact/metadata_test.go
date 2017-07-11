// Copyright 2017 Northern.tech AS
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

package artifact

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateInfo(t *testing.T) {
	var validateTests = []struct {
		in  Info
		err error
	}{
		{Info{Format: "", Version: 0}, ErrValidatingData},
		{Info{Format: "", Version: 1}, ErrValidatingData},
		{Info{Format: "format"}, ErrValidatingData},
		{Info{}, ErrValidatingData},
		{Info{Format: "format", Version: 1}, nil},
	}

	for _, tt := range validateTests {
		e := tt.in.Validate()
		assert.Equal(t, e, tt.err)
	}
}

func TestValidateHeaderInfo(t *testing.T) {
	var validateTests = []struct {
		in  HeaderInfo
		err error
	}{
		{HeaderInfo{}, ErrValidatingData},
		{HeaderInfo{Updates: []UpdateType{}}, ErrValidatingData},
		{HeaderInfo{Updates: []UpdateType{{Type: ""}}}, ErrValidatingData},
		{HeaderInfo{Updates: []UpdateType{{Type: "update"}, {}}}, ErrValidatingData},
		{HeaderInfo{Updates: []UpdateType{{}, {Type: "update"}}, CompatibleDevices: []string{""}, ArtifactName: "id"}, ErrValidatingData},
		{HeaderInfo{Updates: []UpdateType{{Type: "update"}, {Type: ""}}, CompatibleDevices: []string{""}, ArtifactName: "id"}, ErrValidatingData},
		{HeaderInfo{Updates: []UpdateType{{Type: "update"}}, CompatibleDevices: []string{""}, ArtifactName: "id"}, nil},
		{HeaderInfo{Updates: []UpdateType{{Type: "update"}}, ArtifactName: "id"}, ErrValidatingData},
		{HeaderInfo{Updates: []UpdateType{{Type: "update"}}, CompatibleDevices: []string{""}, ArtifactName: ""}, ErrValidatingData},
		{HeaderInfo{Updates: []UpdateType{{Type: "update"}, {Type: "update"}}, CompatibleDevices: []string{""}, ArtifactName: "id"}, nil},
	}
	for idx, tt := range validateTests {
		e := tt.in.Validate()
		assert.Equal(t, e, tt.err, "failing test: %v (%v)", idx, tt)
	}
}

func TestValidateTypeInfo(t *testing.T) {
	var validateTests = []struct {
		in  TypeInfo
		err error
	}{
		{TypeInfo{}, ErrValidatingData},
		{TypeInfo{Type: ""}, ErrValidatingData},
		{TypeInfo{Type: "rootfs-image"}, nil},
	}

	for _, tt := range validateTests {
		e := tt.in.Validate()
		assert.Equal(t, e, tt.err)
	}
}

func TestValidateMetadata(t *testing.T) {
	var validateTests = []struct {
		in  string
		err error
	}{
		{``, nil},
		{`{"key": "val"}`, nil},
		{`{"key": "val", "other_key": "other_val"}`, nil},
	}

	for _, tt := range validateTests {
		mtd := new(Metadata)
		l, e := mtd.Write([]byte(tt.in))
		assert.NoError(t, e)
		assert.Equal(t, len(tt.in), l)
		e = mtd.Validate()
		assert.Equal(t, e, tt.err, "failing test: %v", tt)
	}
}

func TestValidateFiles(t *testing.T) {
	var validateTests = []struct {
		in  Files
		err error
	}{
		{Files{}, ErrValidatingData},
		{Files{[]string{""}}, ErrValidatingData},
		{Files{[]string{}}, ErrValidatingData},
		{Files{[]string{"file"}}, nil},
		{Files{[]string{"file", ""}}, ErrValidatingData},
		{Files{[]string{"file", "file_next"}}, nil},
	}
	for idx, tt := range validateTests {
		e := tt.in.Validate()
		assert.Equal(t, e, tt.err, "failing test: %v (%v)", idx, tt)
	}
}
