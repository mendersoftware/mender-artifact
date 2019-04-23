// Copyright 2019 Northern.tech AS
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
	"io"
	"testing"

	"github.com/mendersoftware/mender-artifact/artifact"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandlerRootfs(t *testing.T) {
	// test if update type is reported correctly
	r := NewRootfsV1("")
	require.Equal(t, "rootfs-image", r.GetUpdateType())

	// test get update files
	r.update = &DataFile{Name: "update.ext4"}
	require.Equal(t, "update.ext4", r.GetUpdateFiles()[0].Name)
	require.Equal(t, 1, r.version)

	r = NewRootfsV2("")
	require.Equal(t, "rootfs-image", r.GetUpdateType())

	// test get update files
	r.update = &DataFile{Name: "update_next.ext4"}
	require.Equal(t, "update_next.ext4", r.GetUpdateFiles()[0].Name)
	require.Equal(t, 2, r.version)

	// test cppy
	n := r.NewInstance()
	require.IsType(t, &Rootfs{}, n)
}

type TestErrWriter bytes.Buffer

func (t *TestErrWriter) Write(b []byte) (n int, err error) {
	return 0, io.ErrUnexpectedEOF
}

func TestRootfsReadHeader(t *testing.T) {
	var r Installer
	r = NewRootfsV1("custom")

	tc := []struct {
		version            int
		rootfs             Installer
		data               string
		name               string
		shouldErr          bool
		errMsg             string
		shouldErrAugmented bool
		errMsgAugmented    string
	}{
		{rootfs: r, data: "invalid", name: "headers/0000/files", shouldErr: true,
			errMsg: "invalid character 'i'", version: 2},
		{rootfs: r, data: `{"files":["update.ext4"]}`,
			name: "headers/0000/files", shouldErr: false, version: 2},
		{rootfs: r, data: `{"files":["update.ext4", "next_update.ext4"]}`,
			name: "headers/0000/files", shouldErr: true, version: 2,
			errMsg: "Rootfs image does not contain exactly one file"},
		{rootfs: r, data: `1212121212121212121212121212`,
			name: "headers/0000/checksums/update.ext4.sum", shouldErr: false, version: 2},
		{rootfs: r, data: "", name: "headers/0000/non-existing", shouldErr: true,
			errMsg: "unsupported file", version: 2},
		{rootfs: r, data: "data", name: "headers/0000/type-info", shouldErr: false, version: 2},
		{rootfs: r, data: "", name: "headers/0000/meta-data", shouldErr: false, version: 2},
		{rootfs: r, data: "", name: "headers/0000/scripts/pre/my_script", shouldErr: false, version: 2},
		{rootfs: r, data: "", name: "headers/0000/scripts/post/my_script", shouldErr: false, version: 2},
		{rootfs: r, data: "", name: "headers/0000/scripts/check/my_script", shouldErr: false, version: 2},
		{rootfs: r, data: "", name: "headers/0000/signatures/update.sig", shouldErr: false, version: 2},
		/////////////////////////
		// Version 3 specifics //
		/////////////////////////
		{rootfs: NewRootfsV3("custom"), data: `{"files":["update.ext4", "next_update.ext4"]}`,
			name: "headers/0000/files", shouldErr: true, version: 3,
			errMsg:             "\"files\" entry found in version 3 artifact",
			shouldErrAugmented: true,
			errMsgAugmented:    "\"files\" entry found in version 3 artifact"},
		{rootfs: NewRootfsV3("custom"), data: "",
			name: "headers/0000/unexpected-file", shouldErr: true, version: 3,
			errMsg:             "unsupported file",
			shouldErrAugmented: true,
			errMsgAugmented:    "unsupported file"},
		{rootfs: NewRootfsV3("custom"), data: "{}",
			name: "headers/0000/type-info", shouldErr: false, version: 3},
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
				// Done for version 1 and 2
				return
			}

			err = r.ReadHeader(bytes.NewReader([]byte(test.data)), test.name, test.version, true)
			if test.shouldErrAugmented {
				require.Error(t, err)
				if test.errMsgAugmented != "" {
					require.Contains(t, errors.Cause(err).Error(), test.errMsgAugmented)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestComposeHeader(t *testing.T) {

	tests := map[string]struct {
		rfs        *Rootfs
		args       ComposeHeaderArgs
		err        error
		verifyFunc func(args ComposeHeaderArgs)
	}{
		"wrong version": {
			rfs:  NewRootfsInstaller(),
			args: ComposeHeaderArgs{Version: -1},
			err:  errors.New("ComposeHeader: rootfs-version 0 not supported"),
		},
		"version 1 - no tar writer": {
			rfs:  NewRootfsV1(""),
			args: ComposeHeaderArgs{},
			err:  errors.New("writer: tar-writer is nil"),
		},
		"version 2 - no tar writer": {
			rfs:  NewRootfsV2(""),
			args: ComposeHeaderArgs{},
			err:  errors.New("writer: tar-writer is nil"),
		},
		"version 3 - no tar writer": {
			rfs:  NewRootfsV3(""),
			args: ComposeHeaderArgs{},
			err:  errors.New("ComposeHeader: Payload: can not tar type-info: arch: Can not write to empty tar-writer"),
		},
		"version 1 - success": { // TODO - should this succeed with no update files?
			rfs: NewRootfsV1(""),
			args: ComposeHeaderArgs{
				TarWriter: tar.NewWriter(bytes.NewBuffer(nil)),
			},
			err: nil,
		},
		"version 3 - success": {
			rfs: NewRootfsV3(""),
			args: ComposeHeaderArgs{
				TarWriter: tar.NewWriter(bytes.NewBuffer(nil)),
			},
			err: errors.New("ComposeHeader: Payload: can not tar type-info: arch: Can not write to empty tar-writer"),
		},
		"version 3 - Test remove augmented provides": {
			rfs: NewAugmentedRootfs(NewRootfsV3(""), ""),
			args: ComposeHeaderArgs{
				TarWriter:  tar.NewWriter(bytes.NewBuffer(nil)),
				TypeInfoV3: &artifact.TypeInfoV3{ArtifactProvides: &artifact.TypeInfoProvides{}},
				Augmented:  true,
			},
			verifyFunc: func(args ComposeHeaderArgs) { assert.Nil(t, args.TypeInfoV3) },
		},
		"error: metadata not empty": {
			rfs: NewRootfsV1(""),
			args: ComposeHeaderArgs{
				TarWriter: tar.NewWriter(bytes.NewBuffer(nil)),
				MetaData:  map[string]interface{}{"foo": "bar"},
			},
			err: errors.New("MetaData not empty in Rootfs.ComposeHeader. This is a bug in the application."),
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

func TestNewAugmentedInstance(t *testing.T) {
	rfs := NewRootfsInstaller()
	inst, err := rfs.NewAugmentedInstance(NewRootfsV2(""))

	assert.NotNil(t, err)
	assert.Nil(t, inst)

	modimg := NewModuleImage("foo-module")
	inst, err = rfs.NewAugmentedInstance(modimg)

	assert.NotNil(t, err)
	assert.Nil(t, inst)

	inst, err = rfs.NewAugmentedInstance(NewRootfsV3(""))

	assert.Nil(t, err)
	assert.NotNil(t, inst)

}

func TestSetAndGetUpdateAugmentFiles(t *testing.T) {

	rfs := NewRootfsV3("")
	files := [](*DataFile){
		&DataFile{
			Name: "baz",
		},
	}
	err := rfs.SetUpdateAugmentFiles(files)
	assert.EqualError(t, err, "Rootfs: Cannot set augmented data file on non-augmented instance.")
	assert.Equal(t, 0, len(rfs.GetUpdateAugmentFiles()))
	assert.Equal(t, 0, len(rfs.GetUpdateAllFiles()))

	// No update files - no augmentation done - no error
	files = nil
	err = rfs.SetUpdateAugmentFiles(files)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(rfs.GetUpdateAugmentFiles()))
	assert.Equal(t, 0, len(rfs.GetUpdateAllFiles()))

	// Can only set one datafile for a rootfs update
	files = [](*DataFile){
		&DataFile{
			Name: "baz",
		},
		&DataFile{
			Name: "foo",
		},
	}
	augrfs := NewAugmentedRootfs(rfs, "")
	err = augrfs.SetUpdateAugmentFiles(files)
	assert.EqualError(t, err, "Rootfs: Must provide exactly one update file")
	assert.Equal(t, 0, len(rfs.GetUpdateAugmentFiles()))
	assert.Equal(t, 0, len(augrfs.GetUpdateAllFiles()))

	// No update files - no augmentation - no error (augmented rootfs)
	files = nil
	err = augrfs.SetUpdateAugmentFiles(files)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(augrfs.GetUpdateAugmentFiles()))
	assert.Equal(t, 0, len(augrfs.GetUpdateAllFiles()))

	// Cannot handle both augmented and non-augmented update-file
	files = [](*DataFile){
		&DataFile{
			Name: "baz",
		},
	}
	rfs = NewRootfsV3("")
	rfs.SetUpdateFiles(files)
	augrfs = NewAugmentedRootfs(rfs, "")
	err = augrfs.SetUpdateAugmentFiles(files)
	assert.NotNil(t, err)
	assert.EqualError(t, err, "Rootfs: Cannot handle both augmented and non-augmented update file")
	assert.Equal(t, 0, len(augrfs.GetUpdateAugmentFiles()))
	assert.Equal(t, 1, len(augrfs.GetUpdateAllFiles()))

	// no error
	rfs = NewRootfsV3("")
	rfs.SetUpdateFiles(nil)
	augrfs = NewAugmentedRootfs(rfs, "")
	err = augrfs.SetUpdateAugmentFiles(files)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(augrfs.GetUpdateAugmentFiles()))
}

func TestSetAndGetUpdateFiles(t *testing.T) {

	// Cannot handle augmented and non-augmented update file
	files := [](*DataFile){
		&DataFile{
			Name: "baz",
		},
	}
	rfs := NewRootfsV3("")
	augrfs := NewAugmentedRootfs(rfs, "")
	err := augrfs.SetUpdateAugmentFiles(files)
	assert.Nil(t, err)
	err = augrfs.SetUpdateFiles(files)
	assert.EqualError(t, err, "Rootfs: Cannot handle both augmented and non-augmented update file")
	assert.Equal(t, 0, len(augrfs.GetUpdateFiles()))
}
