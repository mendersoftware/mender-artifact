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
	"bytes"
	"encoding/json"
	"io"
	"testing"

	"github.com/pkg/errors"
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
					ArtifactName:      []string{"release-2"},
					CompatibleDevices: []string{"vexpress-qemu", "rpi3"},
				},
			},
			err: ""},
		"wrong headerinfo": {
			err: "Artifact validation failed with missing arguments: No updates added, Empty artifact provides"},

		"Empty updates": {
			in:  HeaderInfoV3{Updates: []UpdateType{UpdateType{}}},
			err: "Empty update"},
		"Empty Artifact name": {
			in:  HeaderInfoV3{Updates: []UpdateType{UpdateType{}}, ArtifactProvides: &ArtifactProvides{}},
			err: "Artifact name"},
		"Empty supported update type": {
			in:  HeaderInfoV3{Updates: []UpdateType{UpdateType{}}, ArtifactProvides: &ArtifactProvides{}},
			err: "Supported update type"},
	}
	for name, tt := range tests {
		e := tt.in.Validate()
		if tt.err == "" && e == nil {
			continue
		}
		assert.Contains(t, e.Error(), tt.err, "failing test: %v (%v)", name, tt)
	}
}

func TestHeaderInfoV3(t *testing.T) {
	ut := []UpdateType{UpdateType{Type: "rootfs-image"}}
	provides := &ArtifactProvides{ArtifactName: "release-1", SupportedUpdateTypes: []string{"rootfs-image"}}
	depends := &ArtifactDepends{CompatibleDevices: []string{"vexpress-qemu"}}
	hi := NewHeaderInfoV3(ut, provides, depends)

	assert.Equal(t, hi.GetUpdates()[0].Type, ut[0].Type)
	assert.Equal(t, hi.GetCompatibleDevices(), depends.CompatibleDevices)
	assert.Equal(t, hi.ArtifactDepends, depends)
	assert.Equal(t, hi.ArtifactProvides, provides)
	assert.Equal(t, hi.GetArtifactName(), provides.ArtifactName)
}

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
					ArtifactName:      []string{"release-1"},
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
					      "artifact_name": [
                                                      "release-1"
                                              ],
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
					ArtifactName:      []string{"release-1"},
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
					      "artifact_name": [
		                                      "release-1"
		                              ],
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
					ArtifactName:      []string{"release-1"},
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
					      "artifact_name": [
                                                      "release-1"
                                              ],
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

func TestValidateTypeInfoV3(t *testing.T) {
	var validateTests = map[string]struct {
		in  TypeInfoV3
		err error
	}{
		"Fail validation, update-type missing": {TypeInfoV3{}, ErrValidatingData},
		"Update-type present":                  {TypeInfoV3{Type: "delta"}, nil},
	}

	for _, tt := range validateTests {
		e := tt.in.Validate()
		assert.Equal(t, errors.Cause(e), tt.err)
	}
}

func TestWriteTypeInfoV3(t *testing.T) {
	var validateTests = map[string]struct {
		in  TypeInfoV3
		err error
	}{
		"Update-type present": {TypeInfoV3{Type: "delta"}, nil},
	}

	for _, tt := range validateTests {
		_, err := io.Copy(&tt.in, bytes.NewBuffer([]byte(`{"type":"delta"}`)))
		assert.Nil(t, err)
	}
}

func TestAugmentedHeaderInfoV3(t *testing.T) {
	var validateTests = map[string]struct {
		in  AugmentedHeaderInfoV3
		err error
	}{
		"Fail - Missing update": {AugmentedHeaderInfoV3{}, ErrValidatingData},
		"Update-type present":   {AugmentedHeaderInfoV3{Updates: []UpdateType{UpdateType{Type: "delta"}}}, nil},
	}

	for _, tt := range validateTests {
		err := tt.in.Validate()
		assert.Equal(t, errors.Cause(err), tt.err)
	}
}

func TestWriteAugmentedHeaderInfoV3(t *testing.T) {
	var validateTests = map[string]struct {
		in  AugmentedHeaderInfoV3
		err error
	}{
		"dummy": {AugmentedHeaderInfoV3{}, nil},
	}

	for _, tt := range validateTests {
		_, err := io.Copy(&tt.in, bytes.NewBuffer([]byte(`{"type":"delta"}`)))
		assert.Nil(t, err)
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
				ArtifactDepends: &TypeInfoDepends{
					RootfsChecksum: "4d480539cdb23a4aee6330ff80673a5af92b7793eb1c57c4694532f96383b619",
				},
				ArtifactProvides: &TypeInfoProvides{
					RootfsChecksum: "853jsdfh342789sdflkjsdf987324kljsdf987234kjljsdf987234klsdf987d8",
				},
			},
			expected: `{
				"type": "delta",
				"artifact_depends": {
					"rootfs_image_checksum": "4d480539cdb23a4aee6330ff80673a5af92b7793eb1c57c4694532f96383b619"
				},
				"artifact_provides": {
					"rootfs_image_checksum": "853jsdfh342789sdflkjsdf987324kljsdf987234kjljsdf987234klsdf987d8"
				}
			      }`,
		},
		"empty fields": {
			ti: TypeInfoV3{
				Type: "delta",
			},
			expected: `{
                                 "type": "delta"
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
		{Files{}, ErrValidatingData},
		{Files{[]string{""}}, ErrValidatingData},
		{Files{[]string{}}, ErrValidatingData},
		{Files{[]string{"file"}}, nil},
		{Files{[]string{"file", ""}}, ErrValidatingData},
		{Files{[]string{"file", "file_next"}}, nil},
	}
	for idx, tt := range validateTests {
		e := tt.in.Validate()
		if e != nil && tt.err != nil {
			cause := errors.Cause(e)
			assert.Equal(t, cause, tt.err, "failing test: %v (%v)", idx, tt)
		} else if e != nil && tt.err == nil {
			t.Fatalf("[%d] Failed with error: %q, when no error expected.", idx, e)
		}
	}
}

func TestValideFilesV3(t *testing.T) {
	tests := map[string]struct {
		in  FilesV3
		err error
	}{
		// FilesV3 should validate everything, files or no-files.
		"test1": {FilesV3{&Files{[]string{"foobar"}}}, nil},
		"test2": {FilesV3{&Files{}}, nil},
	}
	for _, tt := range tests {
		assert.Nil(t, tt.in.Validate())
	}
}

func TestHeaderInfo(t *testing.T) {
	hi := NewHeaderInfo("release-1", []UpdateType{UpdateType{Type: "rootfs-image"}}, []string{"vexpress-qemu"})
	assert.Equal(t, hi.GetArtifactName(), "release-1")
	assert.Equal(t, hi.Updates[0].Type, "rootfs-image")
	assert.Equal(t, hi.GetCompatibleDevices()[0], "vexpress-qemu")
}
