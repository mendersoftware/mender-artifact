// Copyright 2022 Northern.tech AS
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

package handlers

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"testing"

	"github.com/mendersoftware/mender-artifact/artifact"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBootstrapArtifact(t *testing.T) {
	expected := &BootstrapArtifact{version: 3}
	result := NewBootstrapArtifact()
	assert.Equal(t, expected, result)
}

func TestGetVersion(t *testing.T) {
	expectedVersion := 3
	bootstrapArtifact := NewBootstrapArtifact()
	version := bootstrapArtifact.GetVersion()
	assert.Equal(t, expectedVersion, version)
}

func TestGetUpdateType(t *testing.T) {
	var expected *string
	bootstrapArtifact := NewBootstrapArtifact()
	result := bootstrapArtifact.GetUpdateType()
	assert.Equal(t, expected, result)
}

func TestGetUpdateOriginalType(t *testing.T) {
	var expected string
	bootstrapArtifact := NewBootstrapArtifact()
	result := bootstrapArtifact.GetUpdateOriginalType()
	assert.Equal(t, expected, result)
}

func TestGetUpdateDepends(t *testing.T) {
	type testCase struct {
		typeInfo      *artifact.TypeInfoV3
		expected      artifact.TypeInfoDepends
		expectedError error
	}
	testCases := []testCase{
		{
			typeInfo:      nil,
			expected:      nil,
			expectedError: nil,
		},
		{
			typeInfo: &artifact.TypeInfoV3{
				ArtifactDepends: artifact.TypeInfoDepends{
					"depends": "value",
				},
			},
			expected: artifact.TypeInfoDepends{
				"depends": "value",
			},
			expectedError: nil,
		},
	}

	for n, tc := range testCases {
		t.Run(fmt.Sprintf("test case %d", n), func(t *testing.T) {
			bootstrapArtifact := NewBootstrapArtifact()
			bootstrapArtifact.typeInfoV3 = tc.typeInfo
			result, err := bootstrapArtifact.GetUpdateDepends()
			assert.Equal(t, tc.expected, result)
			assert.Equal(t, tc.expectedError, err)
		})
	}
}

func TestGetUpdateProvides(t *testing.T) {
	type testCase struct {
		typeInfo      *artifact.TypeInfoV3
		expected      artifact.TypeInfoProvides
		expectedError error
	}
	testCases := []testCase{
		{
			typeInfo:      nil,
			expected:      nil,
			expectedError: nil,
		},
		{
			typeInfo: &artifact.TypeInfoV3{
				ArtifactProvides: artifact.TypeInfoProvides{
					"provides": "value",
				},
			},
			expected: artifact.TypeInfoProvides{
				"provides": "value",
			},
			expectedError: nil,
		},
	}

	for n, tc := range testCases {
		t.Run(fmt.Sprintf("test case %d", n), func(t *testing.T) {
			bootstrapArtifact := NewBootstrapArtifact()
			bootstrapArtifact.typeInfoV3 = tc.typeInfo
			result, err := bootstrapArtifact.GetUpdateProvides()
			assert.Equal(t, tc.expected, result)
			assert.Equal(t, tc.expectedError, err)
		})
	}
}

func TestGetUpdateMetaData(t *testing.T) {
	var expected map[string]interface{}
	bootstrapArtifact := NewBootstrapArtifact()
	result, err := bootstrapArtifact.GetUpdateMetaData()
	assert.Equal(t, expected, result)
	assert.Nil(t, err)
}

func TestGetUpdateClearsProvides(t *testing.T) {
	type testCase struct {
		typeInfo *artifact.TypeInfoV3
		expected []string
	}
	testCases := []testCase{
		{
			typeInfo: nil,
			expected: nil,
		},
		{
			typeInfo: &artifact.TypeInfoV3{
				ClearsArtifactProvides: []string{
					"clears", "value",
				},
			},
			expected: []string{
				"clears", "value",
			},
		},
	}

	for n, tc := range testCases {
		t.Run(fmt.Sprintf("test case %d", n), func(t *testing.T) {
			bootstrapArtifact := NewBootstrapArtifact()
			bootstrapArtifact.typeInfoV3 = tc.typeInfo
			result := bootstrapArtifact.GetUpdateClearsProvides()
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestGetUpdateOriginalDepends(t *testing.T) {
	var expected artifact.TypeInfoDepends
	bootstrapArtifact := NewBootstrapArtifact()
	result := bootstrapArtifact.GetUpdateOriginalDepends()
	assert.Equal(t, expected, result)
}

func TestGetUpdateOriginalProvides(t *testing.T) {
	var expected artifact.TypeInfoProvides
	bootstrapArtifact := NewBootstrapArtifact()
	result := bootstrapArtifact.GetUpdateOriginalProvides()
	assert.Equal(t, expected, result)
}

func TestGetUpdateOriginalMetaData(t *testing.T) {
	var expected map[string]interface{}
	bootstrapArtifact := NewBootstrapArtifact()
	result := bootstrapArtifact.GetUpdateOriginalMetaData()
	assert.Equal(t, expected, result)
}

func TestGetUpdateOriginalClearsProvides(t *testing.T) {
	var expected []string
	bootstrapArtifact := NewBootstrapArtifact()
	result := bootstrapArtifact.GetUpdateOriginalClearsProvides()
	assert.Equal(t, expected, result)
}

func TestGetUpdateAugmentDepends(t *testing.T) {
	var expected artifact.TypeInfoDepends
	bootstrapArtifact := NewBootstrapArtifact()
	result := bootstrapArtifact.GetUpdateAugmentDepends()
	assert.Equal(t, expected, result)
}

func TestGetUpdateAugmentProvides(t *testing.T) {
	var expected artifact.TypeInfoProvides
	bootstrapArtifact := NewBootstrapArtifact()
	result := bootstrapArtifact.GetUpdateAugmentProvides()
	assert.Equal(t, expected, result)
}

func TestGetUpdateAugmentMetaData(t *testing.T) {
	var expected map[string]interface{}
	bootstrapArtifact := NewBootstrapArtifact()
	result := bootstrapArtifact.GetUpdateAugmentMetaData()
	assert.Equal(t, expected, result)
}

func TestGetUpdateAugmentClearsProvides(t *testing.T) {
	var expected []string
	bootstrapArtifact := NewBootstrapArtifact()
	result := bootstrapArtifact.GetUpdateAugmentClearsProvides()
	assert.Equal(t, expected, result)
}

func TestGetUpdateOriginalTypeInfoWriter(t *testing.T) {
	var expected io.Writer
	bootstrapArtifact := NewBootstrapArtifact()
	result := bootstrapArtifact.GetUpdateOriginalTypeInfoWriter()
	assert.Equal(t, expected, result)
}

func TestGetUpdateAugmentTypeInfoWriter(t *testing.T) {
	var expected io.Writer
	bootstrapArtifact := NewBootstrapArtifact()
	result := bootstrapArtifact.GetUpdateAugmentTypeInfoWriter()
	assert.Equal(t, expected, result)
}

func TestSetUpdateFiles(t *testing.T) {
	bootstrapArtifact := NewBootstrapArtifact()
	expected := [](*DataFile){
		&DataFile{Name: "test_name", Size: 1},
	}
	files := [](*DataFile){
		&DataFile{Name: "test_name", Size: 1},
	}
	err := bootstrapArtifact.SetUpdateFiles(files)
	assert.Equal(t, expected, bootstrapArtifact.files)
	assert.Nil(t, err)
}

func TestComposeHeaderBootstrap(t *testing.T) {

	tests := map[string]struct {
		rfs        *BootstrapArtifact
		args       ComposeHeaderArgs
		err        error
		verifyFunc func(args ComposeHeaderArgs)
	}{
		"version 3 - no tar writer": {
			rfs:  NewBootstrapArtifact(),
			args: ComposeHeaderArgs{},
			err:  errors.New("ComposeHeader: Payload: can not tar type-info: arch: Can not write to empty tar-writer"),
		},
		"version 3 - success": {
			rfs: NewBootstrapArtifact(),
			args: ComposeHeaderArgs{
				TarWriter: tar.NewWriter(bytes.NewBuffer(nil)),
			},
			err: errors.New("ComposeHeader: Payload: can not tar type-info: arch: Can not write to empty tar-writer"),
		},
		"error: metadata not empty": {
			rfs: NewBootstrapArtifact(),
			args: ComposeHeaderArgs{
				TarWriter: tar.NewWriter(bytes.NewBuffer(nil)),
				MetaData:  map[string]interface{}{"foo": "bar"},
			},
			err: errors.New("MetaData not empty in Rootfs.ComposeHeader. This is a bug in the application."),
		},
		"error: metadata not marshallable": {
			rfs: NewBootstrapArtifact(),
			args: ComposeHeaderArgs{
				TarWriter: tar.NewWriter(bytes.NewBuffer(nil)),
				MetaData:  map[string]interface{}{"asd": make(chan int)},
			},
			err: errors.New("MetaData field unmarshalable. This is a bug in the application: json: unsupported type: chan int"),
		},
	}

	for name, test := range tests {

		err := test.rfs.ComposeHeader(&test.args)
		if err != nil {
			if test.err == nil {
				t.Errorf("Test %s failed with an unexpected error: %v", name, err)
				continue
			}
			assert.EqualError(t, err, test.err.Error())
			if test.verifyFunc != nil {
				test.verifyFunc(test.args)
			}
		}
	}
}

func TestBootstrapReadHeader(t *testing.T) {
	r := NewBootstrapArtifact()

	tc := []struct {
		version   int
		rootfs    Installer
		data      string
		name      string
		shouldErr bool
		errMsg    string
	}{
		{rootfs: r, data: "", name: "headers/0000/non-existing", shouldErr: true,
			errMsg: "unsupported file", version: 3},
		{rootfs: r, data: "data", name: "headers/0000/type-info", shouldErr: true, version: 3,
			errMsg: "invalid character 'd' looking for beginning of value"},
		{rootfs: r, data: "data", name: "headers/0000/type-info", shouldErr: true, version: 2,
			errMsg: "version 2 not supported"},
		{rootfs: r, data: "{\"type\":null}", name: "headers/0000/type-info", shouldErr: false, version: 3},
	}

	for _, test := range tc {
		t.Run("", func(t *testing.T) {

			buf := bytes.NewBuffer(nil)

			tw := tar.NewWriter(buf)
			err := tw.WriteHeader(&tar.Header{
				Name: "not-needed",
				Size: int64(len(test.data)),
			})
			require.NoError(t, err)
			_, err = tw.Write([]byte(test.data))
			require.NoError(t, err)
			err = tw.Close()
			require.NoError(t, err)

			tr := tar.NewReader(buf)
			_, err = tr.Next()
			require.NoError(t, err)

			err = r.ReadHeader(tr, test.name, test.version, false)
			if test.shouldErr {
				require.Error(t, err)
				if test.errMsg != "" {
					require.Contains(t, errors.Cause(err).Error(), test.errMsg)
				}
			} else {
				require.NoError(t, err)
			}

			if test.version < 3 {
				// Done for 2
				return
			}

			// err = r.ReadHeader(bytes.NewReader([]byte(test.data)), test.name, test.version, true)
			// if test.shouldErrAugmented {
			// 	require.Error(t, err)
			// 	if test.errMsgAugmented != "" {
			// 		require.Contains(t, errors.Cause(err).Error(), test.errMsgAugmented)
			// 	}
			// } else {
			// 	require.NoError(t, err)
			// }
		})
	}
}

func TestNewInstance(t *testing.T) {
	bootstrapArtifact := NewBootstrapArtifact()
	expected := &BootstrapArtifact{
		version: 3,
	}
	result := bootstrapArtifact.NewInstance()
	assert.Equal(t, expected, result)
}

func TestNewAugmentedInstanceBootstrap(t *testing.T) {
	bootstrapArtifact := NewBootstrapArtifact()
	input := NewModuleImage("foo-module")
	result, err := bootstrapArtifact.NewAugmentedInstance(input)
	assert.Equal(t, nil, result)
	assert.Nil(t, err)
}

func TestNewUpdateStorer(t *testing.T) {
	bootstrapArtifact := NewBootstrapArtifact()
	updateType := "type"
	payloadNum := 0
	expected := devNullUpdateStorer{}
	result, err := bootstrapArtifact.NewUpdateStorer(&updateType, payloadNum)
	assert.Equal(t, &expected, result)
	assert.Nil(t, err)
}

func TestGetUpdateAllFiles(t *testing.T) {
	bootstrapArtifact := NewBootstrapArtifact()
	var expected [](*DataFile)
	result := bootstrapArtifact.GetUpdateAllFiles()
	assert.Equal(t, expected, result)
}

func TestGetUpdateAugmentFiles(t *testing.T) {
	bootstrapArtifact := NewBootstrapArtifact()
	var expected [](*DataFile)
	result := bootstrapArtifact.GetUpdateAugmentFiles()
	assert.Equal(t, expected, result)
}

func TestGetUpdateFiles(t *testing.T) {
	bootstrapArtifact := NewBootstrapArtifact()
	var expected [](*DataFile)
	result := bootstrapArtifact.GetUpdateFiles()
	assert.Equal(t, expected, result)
}

func TestSetUpdateAugmentFiles(t *testing.T) {
	bootstrapArtifact := NewBootstrapArtifact()
	files := [](*DataFile){&DataFile{Name: "name"}}
	err := bootstrapArtifact.SetUpdateAugmentFiles(files)
	assert.Nil(t, err)
}
