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
	"github.com/pkg/errors"
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
		if e != nil {
			e = errors.Cause(e)
		}
		assert.Equal(t, e, tt.err)
	}
}

func TestValidateHeaderInfo(t *testing.T) {
	var validateTests = []struct {
		in  HeaderInfo
		err string
	}{
		{HeaderInfo{},
			"Artifact validation failed with missing arguments: No updates added, No compatible devices listed, No artifact name"},
		{HeaderInfo{Updates: []UpdateType{}},
			"Artifact validation failed with missing arguments: No updates added, No compatible devices listed, No artifact name"},
		{HeaderInfo{Updates: []UpdateType{{Type: ""}}},
			"Artifact validation failed with missing arguments: No compatible devices listed, No artifact name, Empty update"},
		{HeaderInfo{Updates: []UpdateType{{Type: "update"}, {}}},
			"Artifact validation failed with missing arguments: No compatible devices listed, No artifact name, Empty update"},
		{HeaderInfo{Updates: []UpdateType{{}, {Type: "update"}}, CompatibleDevices: []string{""}, ArtifactName: "id"},
			"Artifact validation failed with missing argument: Empty update"},
		{HeaderInfo{Updates: []UpdateType{{Type: "update"}, {Type: ""}}, CompatibleDevices: []string{""}, ArtifactName: "id"},
			"Artifact validation failed with missing argument: Empty update"},
		{HeaderInfo{Updates: []UpdateType{{Type: "update"}}, CompatibleDevices: []string{""}, ArtifactName: "id"},
			""},
		{HeaderInfo{Updates: []UpdateType{{Type: "update"}}, ArtifactName: "id"},
			"Artifact validation failed with missing argument: No compatible devices listed"},
		{HeaderInfo{Updates: []UpdateType{{Type: "update"}}, CompatibleDevices: []string{""}, ArtifactName: ""},
			"Artifact validation failed with missing argument: No artifact name"},
		{HeaderInfo{Updates: []UpdateType{{Type: "update"}, {Type: "update"}}, CompatibleDevices: []string{""}, ArtifactName: "id"},
			""},
	}
	for idx, tt := range validateTests {
		e := tt.in.Validate()
		if tt.err == "" && e == nil {
			continue
		}
		assert.EqualError(t, e, tt.err, "failing test: %v (%v)", idx, tt)
	}
}

func TestValidateHeaderInfoV3(t *testing.T) {
	var tests = map[string]struct {
		in  HeaderInfoV3
		err string
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
			err: ""},
		"wrong headerinfo": {
			err: "Artifact validation failed with missing arguments: No updates added, Empty artifact provides"},
	}
	for name, tt := range tests {
		e := tt.in.Validate()
		if tt.err == "" && e == nil {
			continue
		}
		assert.Equal(t, tt.err, e.Error(), "failing test: %v (%v)", name, tt)
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
				      "artifact_provides": {
					      "artifact_name": "release-2",
					      "artifact_group": "fix",
					      "update_types_supported": [
						      "rootfs-image"
					      ]
				      },
				      "artifact_depends": {
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
				      "artifact_provides": {
					      "artifact_name": "release-2",
					      "artifact_group": "fix",
					      "update_types_supported": [
						      "rootfs-image",
						      "delta-update"
					      ]
				      },
				      "artifact_depends": {
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
				      "artifact_provides": {
					      "artifact_name": "release-2",
					      "artifact_group": "fix",
					      "update_types_supported": [
						      "rootfs-image",
						      "delta-update"
					      ]
				      },
				      "artifact_depends": {
					      "artifact_name": "release-1",
					      "device_type": [
						      "vexpress-qemu",
                                                      "beaglebone"
					      ]
				      }
			      }`,
		},
		"No artifact depends": {
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
				      "artifact_provides": {
					      "artifact_name": "release-2",
					      "artifact_group": "fix",
					      "update_types_supported": [
						      "rootfs-image",
						      "delta-update"
					      ]
				      },
                                      "artifact_depends": null
			      }`,
		},
	}
	for name, test := range tests {
		b, err := json.MarshalIndent(test.hi, "", "\t")
		require.Nil(t, err, "failed to marshal json for: %s", name)
		require.JSONEq(t, test.expected, string(b), "Marshalled json for %q is wrong", name)
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
		assert.Equal(t, errors.Cause(e), tt.err)
	}
}

func TestMarshalJSONTypeInfoV3(t *testing.T) {
	tests := map[string]struct {
		ti       TypeInfoV3
		expected string
	}{
		"delta": {
			ti: TypeInfoV3{
				Type: "delta",
				ArtifactDepends: []TypeInfoDepends{
					{RootfsChecksum: "4d480539cdb23a4aee6330ff80673a5af92b7793eb1c57c4694532f96383b619"}},
				ArtifactProvides: []TypeInfoProvides{
					{RootfsChecksum: "4d480539cdb23a4aee6330ff80673a5af92b7793eb1c57c4694532f96383b619"}},
			},
			expected: `{
				      "type": "delta",
				      "artifact_depends": [
					      {
						      "rootfs_image_checksum": "4d480539cdb23a4aee6330ff80673a5af92b7793eb1c57c4694532f96383b619"
					      }
				      ],
				      "artifact_provides": [
					      {
						      "rootfs_image_checksum": "4d480539cdb23a4aee6330ff80673a5af92b7793eb1c57c4694532f96383b619"
					      }
				      ]
			      }`,
		},
	}

	for name, test := range tests {
		b, err := json.MarshalIndent(test.ti, "", "\t")
		require.Nil(t, err)
		require.JSONEq(t, test.expected, string(b), "%q failed!", name)
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
		{Files{[]string{"file"}}, nil},
		{Files{[]string{"file", "file_next"}}, nil},
	}
	for idx, tt := range validateTests {
		e := tt.in.Validate()
		assert.Equal(t, e, tt.err, "failing test: %v (%v)", idx, tt)
	}
}
