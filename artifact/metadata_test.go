// Copyright 2018 Northern.tech AS
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
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestValidateHeaderInfoV3(t *testing.T) {
	var tests = map[string]struct {
		in  HeaderInfoV3
		err error
	}{
		"correct headerinfo:": {
			in: HeaderInfoV3{
				Updates: []UpdateType{
					{Type: "rootfs-image"},
					{Type: "delta"}},
				ArtifactProvides: &ArtifactProvides{
					ArtifactName:         "release-2",
					ArtifactGroup:        "group-1",
					SupportedUpdateTypes: []string{"rootfs", "delta"},
				},
				ArtifactDepends: &ArtifactDepends{
					ArtifactName:      "release-2",
					CompatibleDevices: []string{"vexpress-qemu", "rpi3"},
				},
			},
			err: nil},
		"wrong headerinfo": {},
	}
	for name, tt := range tests {
		// TODO - Implement validate!
		e := tt.in.Validate()
		assert.Equal(t, e, tt.err, "failing test: %v (%v)", name, tt)
	}
}

// TODO (?) - currently the marshalling does not match the template in the docs!
// Is artifact-provides supposed to be a list?
func TestMarshalJSONHeaderInfoV3(t *testing.T) {
	tests := map[string]struct {
		hi       HeaderInfoV3
		expected string
	}{
		"one update": {
			hi: HeaderInfoV3{
				Updates: []UpdateType{UpdateType{"rootfs-image"}},
				ArtifactProvides: &ArtifactProvides{
					ArtifactName:         "release-2",
					ArtifactGroup:        "fix",
					SupportedUpdateTypes: []string{"rootfs-image"},
				},
				ArtifactDepends: &ArtifactDepends{
					ArtifactName:      "release-1",
					CompatibleDevices: []string{"vexpress-qemu"},
				},
			},
			expected: `{
					  "updates": [
						  {
							  "type": "rootfs-image"
						  }
					  ],
					  "ArtifactProvides": {
						  "artifact_name": "release-2",
						  "artifact_group": "fix",
						  "update_types_supported": [
							  "rootfs-image"
						  ]
					  },
					  "ArtifactDepends": {
						  "artifact_name": "release-1",
						  "device_type": [
							  "vexpress-qemu"
						  ]
					  }
				  }`,
		},
		"two updates": {
			hi: HeaderInfoV3{
				Updates: []UpdateType{
					UpdateType{"rootfs-image"},
					UpdateType{"delta-image"},
				},
				ArtifactProvides: &ArtifactProvides{
					ArtifactName:         "release-2",
					ArtifactGroup:        "fix",
					SupportedUpdateTypes: []string{"rootfs-image", "delta-update"},
				},
				ArtifactDepends: &ArtifactDepends{
					ArtifactName:      "release-1",
					CompatibleDevices: []string{"vexpress-qemu"},
				},
			},
			expected: `{
				      "updates": [
					      {
						      "type": "rootfs-image"
					      },
					      {
						      "type": "delta-image"
					      }
				      ],
				      "ArtifactProvides": {
					      "artifact_name": "release-2",
					      "artifact_group": "fix",
					      "update_types_supported": [
						      "rootfs-image",
						      "delta-update"
					      ]
				      },
				      "ArtifactDepends": {
					      "artifact_name": "release-1",
					      "device_type": [
						      "vexpress-qemu"
					      ]
				      }
			      }`,
		},
		"two updates, multiple device types": {
			hi: HeaderInfoV3{
				Updates: []UpdateType{
					UpdateType{"rootfs-image"},
					UpdateType{"delta-image"},
				},
				ArtifactProvides: &ArtifactProvides{
					ArtifactName:         "release-2",
					ArtifactGroup:        "fix",
					SupportedUpdateTypes: []string{"rootfs-image", "delta-update"},
				},
				ArtifactDepends: &ArtifactDepends{
					ArtifactName:      "release-1",
					CompatibleDevices: []string{"vexpress-qemu", "beaglebone"},
				},
			},
			expected: `{
				      "updates": [
					      {
						      "type": "rootfs-image"
					      },
					      {
						      "type": "delta-image"
					      }
				      ],
				      "ArtifactProvides": {
					      "artifact_name": "release-2",
					      "artifact_group": "fix",
					      "update_types_supported": [
						      "rootfs-image",
						      "delta-update"
					      ]
				      },
				      "ArtifactDepends": {
					      "artifact_name": "release-1",
					      "device_type": [
						      "vexpress-qemu",
                                                      "beaglebone"
					      ]
				      }
			      }`,
		},
	}
	for name, test := range tests {
		b, err := json.MarshalIndent(test.hi, "", "\t")
		require.Nil(t, err, "failed to marshal json for: %s", name)
		assert.JSONEq(t, test.expected, string(b), "Marshalled json for %s is wrong")
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
